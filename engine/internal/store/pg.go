package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lib/pq"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
)

type PG struct {
	db *sql.DB
}

// schemaDDL creates all tables used by the engine if they don't already exist.
// This makes a fresh DATABASE_URL (local dev, CI, a new Render Postgres instance)
// self-provisioning instead of relying on manual/undocumented DDL.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS proofs (
	id text PRIMARY KEY,
	agent text NOT NULL,
	proof_hash text NOT NULL,
	input_hash text NOT NULL,
	output_hash text NOT NULL,
	model_hash text NOT NULL,
	merkle_root text NOT NULL,
	merkle_path text[] NOT NULL DEFAULT '{}',
	leaf_index integer NOT NULL DEFAULT 0,
	created_at bigint NOT NULL,
	valid boolean NOT NULL DEFAULT true,
	revoked boolean NOT NULL DEFAULT false,
	use_case text NOT NULL DEFAULT '',
	public_key text NOT NULL DEFAULT '',
	deploy_hash text NOT NULL DEFAULT '',
	generation_ms bigint NOT NULL DEFAULT 0,
	mode text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS kyc_whitelist (
	user_id text PRIMARY KEY,
	proof_id text NOT NULL,
	whitelisted boolean NOT NULL DEFAULT true,
	granted_at bigint NOT NULL
);

CREATE TABLE IF NOT EXISTS model_registry (
	model_id text PRIMARY KEY,
	model_hash text NOT NULL,
	verifier_contract text NOT NULL,
	metadata jsonb NOT NULL DEFAULT '{}',
	registered_at bigint NOT NULL,
	deploy_hash text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS aggregation_batches (
	batch_id text PRIMARY KEY,
	max_proofs integer NOT NULL DEFAULT 10,
	proof_hashes text[] NOT NULL DEFAULT '{}',
	merkle_root text NOT NULL DEFAULT '',
	status text NOT NULL DEFAULT 'open',
	created_at bigint NOT NULL,
	finalized_at bigint NOT NULL DEFAULT 0,
	aggregate_proof_hash text NOT NULL DEFAULT '',
	individual_proof_hashes text[] NOT NULL DEFAULT '{}',
	proof_count integer NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS api_keys (
	id text PRIMARY KEY,
	key_hash text NOT NULL UNIQUE,
	wallet_addr text NOT NULL,
	scope text NOT NULL,
	created_at bigint NOT NULL,
	revoked boolean NOT NULL DEFAULT false,
	revoked_at bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS api_keys_wallet_idx ON api_keys(wallet_addr);

CREATE TABLE IF NOT EXISTS wallet_challenges (
	nonce text PRIMARY KEY,
	wallet_addr text NOT NULL,
	created_at bigint NOT NULL,
	expires_at bigint NOT NULL,
	consumed_at bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS wallet_challenges_wallet_idx ON wallet_challenges(wallet_addr);
`

// Open connects to PostgreSQL using DATABASE_URL. Returns nil,nil if unset.
func Open() (*PG, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, nil
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("pg open: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pg ping: %w", err)
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		return nil, fmt.Errorf("pg schema init: %w", err)
	}
	return &PG{db: db}, nil
}

func (s *PG) Close() {
	if s != nil && s.db != nil {
		_ = s.db.Close()
	}
}

// Load reads all proofs from the database into the engine.
func (s *PG) Load(eng *prover.ProofEngine) (int, error) {
	rows, err := s.db.Query(`SELECT id, agent, proof_hash, input_hash, output_hash, model_hash,
		merkle_root, merkle_path, leaf_index, created_at, valid, revoked, use_case,
		COALESCE(public_key,''), COALESCE(deploy_hash,''), generation_ms, mode
		FROM proofs ORDER BY created_at`)
	if err != nil {
		return 0, fmt.Errorf("pg load query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var p prover.Proof
		var path pq.StringArray
		err := rows.Scan(&p.ID, &p.Agent, &p.PH, &p.IH, &p.OH, &p.MH,
			&p.Root, &path, &p.Idx, &p.TS, &p.Valid, &p.Revoked, &p.UseCase,
			&p.PubKey, &p.Deploy, &p.GenMs, &p.Mode)
		if err != nil {
			return count, fmt.Errorf("pg load scan: %w", err)
		}
		p.Path = []string(path)
		eng.Restore(&p)
		count++
	}
	return count, rows.Err()
}

// Save inserts a new proof or updates on conflict.
func (s *PG) Save(p *prover.Proof) error {
	_, err := s.db.Exec(`INSERT INTO proofs
		(id, agent, proof_hash, input_hash, output_hash, model_hash,
		 merkle_root, merkle_path, leaf_index, created_at, valid, revoked,
		 use_case, public_key, deploy_hash, generation_ms, mode)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO UPDATE SET
			valid=EXCLUDED.valid, revoked=EXCLUDED.revoked, deploy_hash=EXCLUDED.deploy_hash`,
		p.ID, p.Agent, p.PH, p.IH, p.OH, p.MH,
		p.Root, pq.StringArray(p.Path), p.Idx, p.TS, p.Valid, p.Revoked,
		p.UseCase, p.PubKey, p.Deploy, p.GenMs, p.Mode)
	return err
}

// Update persists changes to valid/revoked/deploy_hash.
func (s *PG) Update(p *prover.Proof) error {
	_, err := s.db.Exec(`UPDATE proofs SET valid=$1, revoked=$2, deploy_hash=$3 WHERE id=$4`,
		p.Valid, p.Revoked, p.Deploy, p.ID)
	return err
}

// SaveKYC persists a KYC whitelist entry.
func (s *PG) SaveKYC(user, proofID string, ts int64) error {
	_, err := s.db.Exec(`INSERT INTO kyc_whitelist (user_id, proof_id, whitelisted, granted_at)
		VALUES ($1,$2,TRUE,$3) ON CONFLICT (user_id) DO UPDATE SET proof_id=EXCLUDED.proof_id, granted_at=EXCLUDED.granted_at`,
		user, proofID, ts)
	return err
}

// KYCEntry is a persisted KYC whitelist row.
type KYCEntry struct {
	User    string
	ProofID string
}

// LoadKYC reads all currently-whitelisted users from the database. Used to
// rehydrate the in-memory whitelist on server start - previously this table
// was write-only (SaveKYC was called but nothing ever read it back), so every
// restart silently lost every KYC grant even though the row was sitting in
// Postgres the whole time.
func (s *PG) LoadKYC() ([]KYCEntry, error) {
	rows, err := s.db.Query(`SELECT user_id, proof_id FROM kyc_whitelist WHERE whitelisted = TRUE`)
	if err != nil {
		return nil, fmt.Errorf("pg load kyc query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []KYCEntry
	for rows.Next() {
		var e KYCEntry
		if err := rows.Scan(&e.User, &e.ProofID); err != nil {
			return out, fmt.Errorf("pg load kyc scan: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ModelRegistryEntry is the persisted record for an on-chain-bound AI model.
// (Defined here, not in package inference, so store has no import-cycle back
// to its own caller; inference.ModelRegistryEntry is converted to/from this.)
type ModelRegistryEntry struct {
	ModelID          string
	ModelHash        string
	VerifierContract string
	Metadata         map[string]string
	RegisteredAt     int64
	DeployHash       string
}

// SaveProof is a context-aware alias for Save, for callers that pass a ctx
// (the underlying database/sql call itself is not yet context-cancellable
// here; kept as a thin wrapper so call sites can pass ctx uniformly).
func (s *PG) SaveProof(ctx context.Context, p *prover.Proof) error {
	return s.Save(p)
}

// SaveModelRegistryEntry inserts or updates a model registry row.
func (s *PG) SaveModelRegistryEntry(ctx context.Context, e *ModelRegistryEntry) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_registry
		(model_id, model_hash, verifier_contract, metadata, registered_at, deploy_hash)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (model_id) DO UPDATE SET
			model_hash=EXCLUDED.model_hash, verifier_contract=EXCLUDED.verifier_contract,
			metadata=EXCLUDED.metadata, deploy_hash=EXCLUDED.deploy_hash`,
		e.ModelID, e.ModelHash, e.VerifierContract, meta, e.RegisteredAt, e.DeployHash)
	return err
}

// GetModelRegistryEntry looks up a model registry row by model ID.
func (s *PG) GetModelRegistryEntry(ctx context.Context, modelID string) (*ModelRegistryEntry, error) {
	var e ModelRegistryEntry
	var meta []byte
	err := s.db.QueryRowContext(ctx, `SELECT model_id, model_hash, verifier_contract, metadata, registered_at, deploy_hash
		FROM model_registry WHERE model_id=$1`, modelID).
		Scan(&e.ModelID, &e.ModelHash, &e.VerifierContract, &meta, &e.RegisteredAt, &e.DeployHash)
	if err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &e.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &e, nil
}

// AggBatchRow is a persisted aggregation batch row.
type AggBatchRow struct {
	BatchID              string
	MaxProofs            int
	ProofHashes          []string
	MerkleRoot           string
	Status               string
	CreatedAt            int64
	FinalizedAt          int64
	AggregateProofHash   string
	IndividualProofHashes []string
	ProofCount           int
}

// SaveAggBatch persists an aggregation batch row.
func (s *PG) SaveAggBatch(ctx context.Context, b *AggBatchRow) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO aggregation_batches
		(batch_id, max_proofs, proof_hashes, merkle_root, status,
		 created_at, finalized_at, aggregate_proof_hash, individual_proof_hashes, proof_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (batch_id) DO UPDATE SET
			proof_hashes=EXCLUDED.proof_hashes, merkle_root=EXCLUDED.merkle_root,
			status=EXCLUDED.status, finalized_at=EXCLUDED.finalized_at,
			aggregate_proof_hash=EXCLUDED.aggregate_proof_hash,
			individual_proof_hashes=EXCLUDED.individual_proof_hashes,
			proof_count=EXCLUDED.proof_count`,
		b.BatchID, b.MaxProofs, pq.StringArray(b.ProofHashes), b.MerkleRoot, b.Status,
		b.CreatedAt, b.FinalizedAt, b.AggregateProofHash,
		pq.StringArray(b.IndividualProofHashes), b.ProofCount)
	return err
}

// LoadAggBatches reads all aggregation batches from the database.
func (s *PG) LoadAggBatches() ([]AggBatchRow, error) {
	rows, err := s.db.Query(`SELECT batch_id, max_proofs, proof_hashes, merkle_root, status,
		created_at, finalized_at, aggregate_proof_hash, individual_proof_hashes, proof_count
		FROM aggregation_batches ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("pg load agg batches: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AggBatchRow
	for rows.Next() {
		var b AggBatchRow
		var ph, iph pq.StringArray
		if err := rows.Scan(&b.BatchID, &b.MaxProofs, &ph, &b.MerkleRoot, &b.Status,
			&b.CreatedAt, &b.FinalizedAt, &b.AggregateProofHash, &iph, &b.ProofCount); err != nil {
			return out, fmt.Errorf("pg load agg scan: %w", err)
		}
		b.ProofHashes = []string(ph)
		b.IndividualProofHashes = []string(iph)
		out = append(out, b)
	}
	return out, rows.Err()
}

// --- Per-wallet API keys ---
//
// APIKeyRecord is the persisted row for a per-wallet API key. Only
// sha256(plaintext) ever lands here (KeyHash); plaintext is returned
// exactly once by POST /admin/keys/issue and then discarded.
type APIKeyRecord struct {
	ID        string
	KeyHash   string
	Wallet    string
	Scope     string
	CreatedAt int64
	Revoked   bool
	RevokedAt int64
}

// InsertAPIKey persists a new API key record. Uniqueness on key_hash
// makes duplicate-plaintext (astronomically unlikely with 256-bit
// entropy) a hard error rather than silent overwrite.
func (s *PG) InsertAPIKey(ctx context.Context, rec *APIKeyRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_keys
		(id, key_hash, wallet_addr, scope, created_at, revoked, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rec.ID, rec.KeyHash, rec.Wallet, rec.Scope, rec.CreatedAt, rec.Revoked, rec.RevokedAt)
	if err != nil {
		return fmt.Errorf("pg insert api_key: %w", err)
	}
	return nil
}

// LookupAPIKeyByHash fetches the record for a given sha256(key).
// Returns sql.ErrNoRows if the key is unknown.
func (s *PG) LookupAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	var r APIKeyRecord
	err := s.db.QueryRowContext(ctx, `SELECT id, key_hash, wallet_addr, scope,
		created_at, revoked, revoked_at FROM api_keys WHERE key_hash=$1`, keyHash).
		Scan(&r.ID, &r.KeyHash, &r.Wallet, &r.Scope, &r.CreatedAt, &r.Revoked, &r.RevokedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("pg lookup api_key: %w", err)
	}
	return &r, nil
}

// RevokeAPIKey marks the key with the given id as revoked. Returns
// an error containing "not found" if no row was affected — the caller
// (adminRevokeKey) turns that into a 404.
func (s *PG) RevokeAPIKey(ctx context.Context, id string, revokedAt int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked=TRUE, revoked_at=$2
		WHERE id=$1 AND revoked=FALSE`, id, revokedAt)
	if err != nil {
		return fmt.Errorf("pg revoke api_key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("pg revoke rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("api key not found or already revoked: %s", id)
	}
	return nil
}

// --- Wallet challenges (nonces for signed issuance) ---
//
// WalletChallengeRecord persists a short-lived nonce a client must
// echo back with a wallet signature to prove control of the wallet.
// See engine/internal/api/admin_keys.go for the full flow.
type WalletChallengeRecord struct {
	Nonce      string
	Wallet     string
	CreatedAt  int64
	ExpiresAt  int64
	ConsumedAt int64
}

// InsertWalletChallenge persists a fresh challenge. The Nonce column
// is PRIMARY KEY so the astronomically unlikely event of duplicate
// nonces is a hard error rather than silent overwrite.
func (s *PG) InsertWalletChallenge(ctx context.Context, rec *WalletChallengeRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO wallet_challenges
		(nonce, wallet_addr, created_at, expires_at, consumed_at)
		VALUES ($1, $2, $3, $4, $5)`,
		rec.Nonce, rec.Wallet, rec.CreatedAt, rec.ExpiresAt, rec.ConsumedAt)
	if err != nil {
		return fmt.Errorf("pg insert wallet_challenge: %w", err)
	}
	return nil
}

// LookupWalletChallenge fetches a challenge by its nonce. Returns
// sql.ErrNoRows if unknown.
func (s *PG) LookupWalletChallenge(ctx context.Context, nonce string) (*WalletChallengeRecord, error) {
	var r WalletChallengeRecord
	err := s.db.QueryRowContext(ctx, `SELECT nonce, wallet_addr, created_at, expires_at, consumed_at
		FROM wallet_challenges WHERE nonce=$1`, nonce).
		Scan(&r.Nonce, &r.Wallet, &r.CreatedAt, &r.ExpiresAt, &r.ConsumedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("pg lookup wallet_challenge: %w", err)
	}
	return &r, nil
}

// MarkWalletChallengeConsumed atomically flips consumed_at from 0 to
// the supplied timestamp. If the row is already consumed the UPDATE
// affects zero rows and this returns an error — the caller relies on
// that to detect nonce replay races.
func (s *PG) MarkWalletChallengeConsumed(ctx context.Context, nonce string, consumedAt int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE wallet_challenges SET consumed_at=$2
		WHERE nonce=$1 AND consumed_at=0`, nonce, consumedAt)
	if err != nil {
		return fmt.Errorf("pg consume wallet_challenge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("pg consume rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("wallet challenge already consumed or missing: %s", nonce)
	}
	return nil
}
