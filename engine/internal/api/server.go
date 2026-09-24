package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/aggregator"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/siwe"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api/tenant"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/config"
	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto/keystore"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/decision/attest"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/hasher"
	hitlsvc "github.com/anna-stolbovskaja/CasperProver/engine/internal/hitl"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/inference"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/kyc"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/obs"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/observability"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/quorum"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/receipts"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/store"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/submitter"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/verifier"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/zkverifier/gnarkzk"
	"github.com/anna-stolbovskaja/CasperProver/engine/pkg/phase2"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
)

// contractHashes holds on-chain contract hashes, configurable via env vars
// so redeployed contracts don't require a code change.
type contractHashes struct {
	ProofRegistry string
	VerifierGate  string
	DefiMock      string
	StakeSlashing string
	// The remaining 5 of the 9 live contracts. These are manifest-only
	// (deploy-out/onchain.json), no CONTRACT_* env override exists for
	// them and Preflight() does not require one — see the comment there.
	ProofAggregation string
	ModelRegistry    string
	ProofOfInference string
	Governance       string
	// ZkVerifier was compiled since the hackathon deadline but only deployed
	// to testnet on 2026-07-27 (see docs/UNDEPLOYED_CONTRACTS.md history).
	// It is a verdict-recorder, not a pairing verifier — see its source doc
	// comment for the exact trust model.
	ZkVerifier string
}

type Server struct {
	eng       *prover.ProofEngine
	ver       *verifier.LocalVerifier
	kyc       *kyc.DemoKYC
	db        *store.PG
	sub       *submitter.CasperSubmitter
	inf       *inference.InferenceService
	zk        *zkverifier.Groth16Verifier
	realZK    *gnarkzk.Setup    // legacy PreimageCircuit-only setup, kept for backwards compat
	zkReg     *gnarkzk.Registry // v1 circuit registry (persistent keys via CP_ZK_KEYS_DIR)
	keyRing   *pqcrypto.KeyRing // deprecated shim: same object as keystore.Ring() when the backend is memory. Do not add new call sites; new code should route through `keystore`.
	keystore  keystore.Keystore // PQ signing backend. Default memory; opt-in file/remote via CP_KEYSTORE_KIND.
	contracts contractHashes
	port      int
	log       *slog.Logger
	start     time.Time
	apiKey    string
	strict    bool           // fail closed for requested on-chain operations
	scopeReg  *ScopeRegistry // optional per-key scope allowlist; nil = disabled

	obsRegistry *obs.Registry // populated on Start() for /metrics exposition

	// Optional tenant store (BA / backlog 10.1 + 10.2). Non-nil when
	// TENANTS_FILE is set at boot. When nil, all requests are
	// attributed to the synthetic _default tenant and existing
	// behaviour is preserved bit-for-bit.
	tenants *tenant.Store

	aggMu      sync.Mutex
	aggBatches map[string]*aggBatch

	siwe *siwe.Store
	// Multi-provider judge for /inference/judge. Set via SetJudge; nil = 503.
	judge    JudgeService
	hitlSink hitl.Sink // optional HITL delivery sink; set via SetHITLSink
	webhooks *webhookStore
	scopes   *scopeRegistry

	// A2A / HITL pipeline (Pack AQ). Nil when disabled.
	decisionPool   *attest.ProviderPool
	decisionRouter *attest.Router
	decisionJudge  *attest.Judge
	hitlService    *hitlsvc.Service

	// Provenance-lineage receipts (Pack AR). Nil when disabled.
	receipts    *receipts.Service
	receiptSink io.Closer // JSONLSink close-handle when configured

	// BLS12-381 threshold quorum (Pack AS). Nil when disabled.
	quorumRegistry *quorum.Registry

	// Observability (Pack AS-obs). Always non-nil after New*.
	metrics    *observability.Registry
	httpMetric *observability.HTTPMetrics
}

// ctxTenantKey is the context.WithValue key for the resolved tenant
// on a request. Handlers that want the tenant call tenantFromCtx(r).
type ctxTenantKey struct{}

func tenantFromCtx(r *http.Request) *tenant.Tenant {
	if r == nil {
		return nil
	}
	if v := r.Context().Value(ctxTenantKey{}); v != nil {
		if t, ok := v.(*tenant.Tenant); ok {
			return t
		}
	}
	return nil
}

// aggBatch tracks per-batch state for the /aggregation/* endpoints.
// Batches are persisted to Postgres (aggregation_batches table) and
// rehydrated on server startup. The aggregation uses hash-chain
// verification via internal/aggregator.
type aggBatch struct {
	ID          string
	MerkleRoot  string
	MaxProofs   int
	ProofHashes []string
	Status      string // "open" | "finalized"
	CreatedAt   int64
	FinalizedAt int64

	// Pack is populated on finalize by running the batch's proof hashes
	// through the real internal/aggregator (still a conceptual/hash-based
	// simulation of STARK aggregation, not real STARK math - see that
	// package's doc comment - but genuinely wired in and genuinely
	// verifiable, not just bookkeeping).
	Pack *aggregator.STARKPack
}

// manifestHashOrEnv returns env override first, then the canonical manifest,
// then an empty string. A hardcoded fallback here would silently outlive a
// redeploy (Gate 1.5 forbids it).
func manifestHashOrEnv(envKey, manifestKey string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	h, err := config.ContractHash(manifestKey)
	if err != nil {
		slog.Warn("onchain manifest not readable; contract hash will be empty until redeploy manifest is provisioned", "env_key", envKey, "manifest_key", manifestKey, "err", err)
		return ""
	}
	return h
}

// New constructs the API Server from environment configuration.
//
// It returns an error when CP_STRICT=1 is set but the deployment is missing
// a fail-loud prerequisite. Right now that means:
//
//   - API_KEY is empty. Under strict mode we refuse to boot instead of
//     tolerating anonymous writes -- silent "auth off" was the failure
//     mode CP_AGENT_SPEC v2 called out ("startup fails or prominently
//     degrades if API_KEY missing").
//
// Additional strict-mode preconditions may be added here in follow-up
// PRs (see docs/STRICT_MODE_ROLLOUT.md in AE402 for the sibling doc).
func New(eng *prover.ProofEngine, port int, db *store.PG) (*Server, error) {
	contracts := contractHashes{
		ProofRegistry:    manifestHashOrEnv("CONTRACT_PROOF_REGISTRY", "proof_registry"),
		VerifierGate:     manifestHashOrEnv("CONTRACT_VERIFIER_GATE", "verifier_gate"),
		DefiMock:         manifestHashOrEnv("CONTRACT_DEFI_MOCK", "defi_mock"),
		StakeSlashing:    manifestHashOrEnv("CONTRACT_STAKE_SLASHING", "stake_slashing"),
		ProofAggregation: manifestHashOrEnv("CONTRACT_PROOF_AGGREGATION", "proof_aggregation"),
		ModelRegistry:    manifestHashOrEnv("CONTRACT_MODEL_REGISTRY", "model_registry"),
		ProofOfInference: manifestHashOrEnv("CONTRACT_PROOF_OF_INFERENCE", "proof_of_inference"),
		Governance:       manifestHashOrEnv("CONTRACT_GOVERNANCE", "governance"),
		ZkVerifier:       manifestHashOrEnv("CONTRACT_ZK_VERIFIER", "zk_verifier"),
	}

	nodeURL := os.Getenv("CASPER_NODE_URL")
	if nodeURL == "" {
		nodeURL = "https://rpc.testnet.casperlabs.io"
	}
	chain := os.Getenv("CASPER_CHAIN")
	if chain == "" {
		chain = "casper-test"
	}
	keyPath := os.Getenv("DEPLOYER_KEY_PATH")

	var sub *submitter.CasperSubmitter
	if keyPath != "" {
		sub = submitter.New(nodeURL, chain, keyPath)
		slog.Info("submitter configured", "node", nodeURL, "chain", chain)
	} else {
		slog.Warn("no deployer key configured, anchored mode uses computed hashes")
	}

	realZK, err := gnarkzk.NewSetup()
	if err != nil {
		slog.Warn("real Groth16 (gnark) setup failed, /zk/groth16-real/* will return 503", "error", err)
		realZK = nil
	}

	// v1 circuit registry — bundles the same MiMCPreimage circuit above
	// under a stable id plus a ModelInference circuit, and persists
	// ccs/pk/vk to disk when CP_ZK_KEYS_DIR is set so restarts don't
	// invalidate previously-issued proofs.
	zkReg := gnarkzk.NewRegistry()
	_ = zkReg.Register(gnarkzk.MiMCPreimageCircuit{})
	_ = zkReg.Register(gnarkzk.ModelInferenceCircuit{})
	_ = zkReg.Register(gnarkzk.PerceptronCircuit{})
	zkKeysDir := os.Getenv("CP_ZK_KEYS_DIR")
	forceRegen := os.Getenv("CP_ZK_REGENERATE") == "1"
	for _, id := range zkReg.IDs() {
		if zkKeysDir != "" {
			if manifest, err := zkReg.LoadOrCreate(id, zkKeysDir, forceRegen); err != nil {
				slog.Warn("zk registry LoadOrCreate failed, in-memory fallback", "circuit", id, "err", err)
				if err := zkReg.Compile(id); err != nil {
					slog.Error("zk registry Compile fallback failed", "circuit", id, "err", err)
				}
			} else {
				slog.Info("zk circuit ready", "id", id, "vk_digest", manifest.VKDigest, "constraints", manifest.Constraints, "keys_dir", filepath.Join(zkKeysDir, id))
			}
		} else {
			if err := zkReg.Compile(id); err != nil {
				slog.Warn("zk registry in-memory Compile failed", "circuit", id, "err", err)
			}
		}
	}

	strict := os.Getenv("CP_STRICT") == "1"
	apiKey := os.Getenv("API_KEY")
	switch {
	case apiKey == "" && strict:
		// Fail-loud precondition: under CP_STRICT=1 we refuse to boot
		// with an empty API_KEY. Rationale in the New() docstring above
		// and CP_AGENT_SPEC v2 (Gate 1.2). Operator gets an immediate
		// crash instead of a running-but-broken app that accepts
		// anonymous writes.
		return nil, fmt.Errorf("CP_STRICT=1 but API_KEY is empty -- refusing to start with anonymous writes enabled (set API_KEY or unset CP_STRICT)")
	case apiKey == "":
		slog.Warn("API_KEY not set - all write endpoints are unauthenticated (fine for local dev/demo, not for a real deployment; enable CP_STRICT=1 to fail-close)")
	default:
		slog.Info("API_KEY configured - write endpoints require X-API-Key header")
	}

	// Optional tenant store (BA / backlog 10.1 + 10.2). Off by default:
	// TENANTS_FILE points at a JSON registry per docs/TENANT_ISOLATION.md.
	// When absent, the server behaves exactly as before — single shared
	// API_KEY authenticates every write.
	var tenants *tenant.Store
	if path := os.Getenv("TENANTS_FILE"); path != "" {
		ts := tenant.NewStore()
		if err := ts.LoadFile(path); err != nil {
			slog.Warn("TENANTS_FILE load failed, tenant mode disabled", "err", err)
		} else {
			tenants = ts
			slog.Info("tenant mode ENABLED", "file", path, "tenants", len(ts.List()))
		}
	}

	demoKYC := kyc.NewDemo(eng)
	if db != nil {
		entries, err := db.LoadKYC()
		if err != nil {
			slog.Warn("failed to load kyc whitelist from db", "err", err)
		} else {
			for _, e := range entries {
				demoKYC.Restore(e.User, e.ProofID)
			}
			slog.Info("loaded kyc whitelist from postgres", "count", len(entries))
		}
	}

	srv := &Server{
		eng:       eng,
		ver:       verifier.New(),
		kyc:       demoKYC,
		db:        db,
		sub:       sub,
		inf:       inference.New(eng, db, sub),
		zk:        zkverifier.NewGroth16Verifier(),
		realZK:    realZK,
		zkReg:     zkReg,
		keyRing:   nil,
		contracts: contracts,
		port:      port,
		log:       slog.Default(),
		start:     time.Now(),
		apiKey:    apiKey,
		strict:    strict,
		tenants:   tenants,

		aggBatches: make(map[string]*aggBatch),

		siwe: siwe.NewStore(0),
	}

	// Webhook subsystem is always on — in-memory, cheap, no external
	// dependencies. A durable variant belongs in Postgres and is
	// tracked in KNOWN_LIMITATIONS.md.
	srv.webhooks = newWebhookStore()

	// PQ keystore: default memory backend, opt-in file/remote via env.
	// Errors setting up the requested backend are surfaced but do not
	// crash the server — the endpoint layer refuses signing operations
	// when the keystore isn't wired.
	if ks, backing, err := keystore.FromEnv(); err == nil {
		srv.keystore = ks
		if mks, ok := ks.(*keystore.MemoryKeystore); ok {
			srv.keyRing = mks.Ring()
		} else {
			// For non-memory backends, the deprecated keyRing shim stays
			// nil — handler paths that still reach for it will fall back
			// through the interface.
			srv.keyRing = nil
		}
		slog.Info("pq keystore configured", "backing", backing)
	} else {
		slog.Warn("pq keystore setup failed — falling back to memory", "err", err)
		mks := keystore.NewMemory(pqcrypto.NewKeyRing())
		srv.keystore = mks
		srv.keyRing = mks.Ring()
	}

	// Scoped API key registry: opt-in via $CP_SCOPES_FILE. Missing
	// file = fallback to blanket $API_KEY auth (backward compat).
	if path := os.Getenv("CP_SCOPES_FILE"); path != "" {
		srv.scopes = newScopeRegistry()
		if err := srv.scopes.loadFromFile(path); err != nil {
			slog.Warn("failed to load scoped keys file", "path", path, "err", err)
		} else {
			slog.Info("scoped api keys loaded", "path", path)
		}
	}

	// Decision / A2A / HITL pipeline: opt-in via CP_DECISION_ENABLE=1.
	// The default fixture provider registers under trust=system with
	// full capabilities so an operator gets a working pool out of the
	// box; setting CP_DECISION_PROVIDER_URL swaps it for a real remote.
	if os.Getenv("CP_DECISION_ENABLE") == "1" {
		srv.initDecisionPipeline()
	}

	// Provenance-lineage receipts (Pack AR). Opt-in via CP_RECEIPTS_ENABLE=1.
	// The receipts service is layered on top of the decision pipeline; it
	// only requires the keystore + an in-memory store. If CP_RECEIPTS_JSONL
	// is set, an OTel-compatible JSONL sink is attached.
	if os.Getenv("CP_RECEIPTS_ENABLE") == "1" {
		srv.initReceiptsService()
	}

	// BLS12-381 threshold quorum (Pack AS). Opt-in via CP_QUORUM_ENABLE=1.
	// The registry starts empty; operators register signers via
	// POST /v1/quorum/signers. There is no default committee — an empty
	// registry rejects every verify call, which is the safe default.
	if os.Getenv("CP_QUORUM_ENABLE") == "1" {
		srv.quorumRegistry = quorum.NewRegistry()
		srv.log.Info("quorum service enabled")
	}

	// Observability: always on. A Prometheus /metrics endpoint plus
	// W3C traceparent propagation costs nothing when nobody scrapes
	// or sends a header, so we default it wired up.
	srv.metrics = observability.NewRegistry()
	srv.httpMetric = observability.NewHTTPMetrics(srv.metrics, "cp_http")

	// Webhook subsystem metrics — enqueue/attempts/delivered/
	// dead_lettered/replayed counters + attempt-duration histogram
	// + queue/dead-letter gauges, all on the same /metrics endpoint.
	if srv.webhooks != nil {
		srv.webhooks.SetMetrics(observability.NewWebhookMetrics(srv.metrics, "cp_webhook"))
	}

	// Rehydrate aggregation batches from Postgres
	if db != nil {
		rows, err := db.LoadAggBatches()
		if err != nil {
			slog.Warn("failed to load aggregation batches from db", "err", err)
		} else {
			for _, row := range rows {
				b := &aggBatch{
					ID: row.BatchID, MerkleRoot: row.MerkleRoot, MaxProofs: row.MaxProofs,
					ProofHashes: row.ProofHashes, Status: row.Status,
					CreatedAt: row.CreatedAt, FinalizedAt: row.FinalizedAt,
				}
				if row.AggregateProofHash != "" {
					b.Pack = &aggregator.STARKPack{
						AggregateProofHash:    row.AggregateProofHash,
						IndividualProofHashes: row.IndividualProofHashes,
						ProofCount:            row.ProofCount,
						Timestamp:             row.FinalizedAt,
					}
				}
				srv.aggBatches[row.BatchID] = b
			}
			slog.Info("loaded aggregation batches from postgres", "count", len(rows))
		}
	}

	return srv, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /onchain.json", s.onchainManifest)
	mux.HandleFunc("GET /proofs", s.listProofs)
	mux.HandleFunc("GET /proofs/{id}", s.getProof)
	mux.HandleFunc("POST /proofs", s.submitProof)
	mux.HandleFunc("POST /proofs/batch", s.batchProofs)
	mux.HandleFunc("POST /verify", s.verifyProof)
	mux.HandleFunc("POST /verify/batch", s.verifyBatch)
	mux.HandleFunc("POST /proofs/{id}/revoke", s.revokeProof)
	mux.HandleFunc("GET /proofs/{id}/export", s.exportProof)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("POST /kyc/check", s.kycCheck)
	mux.HandleFunc("POST /kyc/grant", s.kycGrant)
	mux.HandleFunc("GET /kyc/whitelist/{user}", s.kycWhitelist)

	// Inference routes
	mux.HandleFunc("POST /inference/prove", s.inferenceProve)
	mux.HandleFunc("POST /inference/verify", s.inferenceVerify)
	mux.HandleFunc("POST /inference/register-model", s.inferenceRegisterModel)
	mux.HandleFunc("GET /inference/model/{id}", s.inferenceGetModel)
	mux.HandleFunc("POST /inference/judge", s.judgeHandler)
	// Aggregation routes
	mux.HandleFunc("POST /aggregation/create-batch", s.aggregationCreateBatch)
	mux.HandleFunc("POST /aggregation/add-proof", s.aggregationAddProof)
	mux.HandleFunc("POST /aggregation/finalize", s.aggregationFinalize)
	mux.HandleFunc("GET /aggregation/batch/{id}", s.aggregationGetBatch)
	mux.HandleFunc("GET /aggregation/verify-batch/{id}", s.aggregationVerifyBatch)
	// ZK Verification routes
	//
	// PRIMARY (real cryptography, gnark BN254 Groth16 with pairing checks) -
	// see internal/zkverifier/gnarkzk/circuit.go. These are the endpoints
	// documented as CasperProver's real ZK path.
	mux.HandleFunc("POST /zk/groth16-real/prove", s.zkGroth16RealProve)
	mux.HandleFunc("POST /zk/groth16-real/verify", s.zkGroth16RealVerify)
	// SIMULATION (hash-based, NOT real BN254 pairing math) - kept for
	// legacy demo/comparison; responses carry {simulation:true, deprecated:true}
	// and a Warning header. Prefer /zk/groth16-real/* for anything real.
	// The /zk/verify-groth16-sim and /zk/batch-verify-sim spellings are the
	// canonical simulation names; /zk/verify-groth16 and /zk/batch-verify
	// are kept as deprecated aliases.
	mux.HandleFunc("POST /zk/verify-groth16-sim", s.zkVerifyGroth16)
	mux.HandleFunc("POST /zk/batch-verify-sim", s.zkBatchVerify)
	mux.HandleFunc("POST /zk/verify-groth16", s.zkVerifyGroth16) // deprecated alias
	mux.HandleFunc("POST /zk/batch-verify", s.zkBatchVerify)     // deprecated alias

	// v1 circuit registry
	mux.HandleFunc("GET /v1/circuits", s.circuitsList)
	mux.HandleFunc("GET /v1/circuits/{id}", s.circuitsGet)
	mux.HandleFunc("GET /v1/circuits/{id}/vk", s.circuitsGetVK)
	mux.HandleFunc("POST /v1/zk/prove", s.zkProveGeneric)
	mux.HandleFunc("POST /v1/zk/verify", s.zkVerifyGeneric)
	mux.HandleFunc("POST /v1/zk/anchor-verdict", s.zkAnchorVerdict)
	mux.HandleFunc("POST /zk/challenge", s.zkChallenge)
	mux.HandleFunc("GET /zk/challenge/{id}", s.zkGetChallenge)
	// Phase 2: proof chains (DAG validation)
	mux.HandleFunc("POST /proof-chain/validate", s.proofChainValidate)
	// Auditable decision logging (backlog 3.2)
	mux.HandleFunc("POST /decisions/log", s.decisionLog)
	mux.HandleFunc("GET /decisions/log", s.decisionRecent)
	mux.HandleFunc("GET /decisions/log/{id}", s.decisionGet)
	mux.HandleFunc("GET /decisions/log/{id}/lineage", s.decisionLineage)
	// Post-quantum routes
	mux.HandleFunc("POST /pq/sign-sphincs", s.pqSignSPHINCS)
	mux.HandleFunc("POST /pq/verify-sphincs", s.pqVerifySPHINCS)
	mux.HandleFunc("POST /pq/hybrid-sign", s.pqHybridSign)
	mux.HandleFunc("POST /pq/hybrid-verify", s.pqHybridVerify)
	// SIWE-like challenge routes (unauthenticated primitive; rate-limited
	// via the shared rate-limit middleware). See engine/internal/api/siwe.
	mux.HandleFunc("POST /auth/siwe/challenge", s.siweChallenge)
	mux.HandleFunc("POST /auth/siwe/verify", s.siweVerify)

	// Observability: /metrics + RED middleware. Zero-dep; opt-in tracer
	// (nil = metrics-only). See docs/OBSERVABILITY.md.
	registry := obs.NewRegistry()
	httpMetrics := obs.NewHTTPMetrics(registry)
	s.obsRegistry = registry
	// Route registered here only when the newer Prometheus registry (s.metrics)
	// isn't wired up -- otherwise the two collide (net/http's ServeMux panics
	// on duplicate exact-pattern registration). See the s.metrics != nil block
	// below, which takes over "GET /metrics" when available.
	if s.metrics == nil {
		mux.Handle("GET /metrics", obs.Handler(registry))
	}

	var tracer *obs.Tracer
	if os.Getenv("CP_TRACES_ENABLED") == "1" {
		tracer = obs.NewTracer("casperprover-engine", os.Stderr)
	}

	instrumented := httpMetrics.MiddlewareRoute(tracer, mux, obs.MuxRouteResolver(mux))

	// Tenant admin routes (BA / backlog 10.1 + 10.2). Registered only
	// when tenant mode is enabled; otherwise these paths 404 through
	// the default mux miss.
	if s.tenants != nil {
		mux.HandleFunc("GET /admin/tenants", s.tenantList)
		mux.HandleFunc("POST /admin/tenants", s.tenantCreate)
		mux.HandleFunc("POST /admin/tenants/{id}/keys", s.tenantAddKey)
		mux.HandleFunc("POST /admin/tenants/{id}/keys/revoke", s.tenantRevokeKeys)
		mux.HandleFunc("GET /admin/tenants/{id}/audit", s.tenantAudit)
		mux.HandleFunc("GET /admin/tenants/audit", s.tenantAudit)
	}

	// PQ key rotation + versioning (in-memory keyring, gated by CP_KEYRING_ENABLE)
	s.registerKeyRingRoutes(mux)
	// Nova / folding aggregation harness (hash-fold-v1 stand-in)
	s.registerNovaRoutes(mux)

	// Webhooks & OpenAPI — versioned surface only.
	s.registerWebhookRoutes(mux)

	// Decision / A2A / HITL pipeline. Registration is a no-op when the
	// pipeline is disabled (CP_DECISION_ENABLE!=1) but the routes still
	// return a well-formed 503, so callers can probe availability.
	s.registerDecisionRoutes(mux)
	s.registerReceiptRoutes(mux)
	s.registerHITLRoutes(mux)
	s.registerQuorumRoutes(mux)
	s.registerMerkleRecursionRoutes(mux)

	mux.HandleFunc("GET /v1/openapi.json", s.openAPIHandler)
	mux.HandleFunc("GET /v1/routes", s.routesHandler)

	// Admin dashboard rollup (9.6): one read-only endpoint that
	// answers "what is this engine doing right now?" for the FE.
	mux.HandleFunc("GET /v1/admin/summary", s.adminSummaryHandler)

	// Prometheus /metrics + a lightweight alias under /v1/.
	if s.metrics != nil {
		mux.Handle("GET /metrics", observability.MetricsHandler(s.metrics))
		mux.Handle("GET /v1/metrics", observability.MetricsHandler(s.metrics))
	}

	// Start the webhook worker with a 1s tick. The tick is short
	// enough that a subscriber with backoff=1s sees near-immediate
	// retry; long enough not to burn CPU under an empty queue.
	if s.webhooks != nil {
		go s.webhooks.runWorker(context.Background(), time.Second)
	}

	scopeMW := func(next http.Handler) http.Handler { return next }
	if s.scopes != nil {
		scopeMW = s.scopeMiddleware(mux)
	}

	addr := fmt.Sprintf(":%d", s.port)
	// Observability wraps the entire chain so /metrics + traceparent
	// see every request. Cardinality stays bounded because we label
	// route="all" here; per-route detail lives in the app metrics
	// (proofs generated, quorum verified, etc.).
	srv := &http.Server{
		Addr: addr,
		Handler: s.v1AliasMiddleware(
			s.acceptVersionMiddleware(
				s.rateLimitMiddleware(
					s.perKeyRateLimitMiddleware(
						s.corsMiddleware(
							s.authMiddleware(
								scopeMW(
									s.idempotencyMiddleware(
										s.logMiddleware(instrumented),
									),
								),
							),
						),
					),
				),
			),
		),

		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	s.log.Info("api starting", "addr", addr)
	return srv.ListenAndServe()
}

// rateLimiter tracks per-IP request counts (60 req/min).
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rlEntry
}

type rlEntry struct {
	count int
	reset time.Time
}

var rl = &rateLimiter{clients: make(map[string]*rlEntry)}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		now := time.Now()
		rl.mu.Lock()
		entry, ok := rl.clients[ip]
		if !ok || now.After(entry.reset) {
			rl.clients[ip] = &rlEntry{count: 1, reset: now.Add(time.Minute)}
			rl.mu.Unlock()
		} else {
			entry.count++
			if entry.count > 60 {
				rl.mu.Unlock()
				s.jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			rl.mu.Unlock()
		}
		// Limit POST body to 1MB
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware requires a matching X-API-Key header on every mutating
// request (anything other than GET/HEAD/OPTIONS) once API_KEY is configured.
// Read-only endpoints (health, listing proofs, checking whitelist status,
// etc.) stay public - this is a demo/hackathon API, not a multi-tenant
// product, so a single shared secret is intentionally simple rather than
// per-agent keys/JWTs. If API_KEY is unset, every request is allowed through
// (documented in KNOWN_LIMITATIONS.md as the local-dev/demo default).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tenant mode: resolve X-API-Key against the tenant store, enforce
		// per-tenant rate + quota, audit outcome, and stash the tenant on
		// the request context. This runs *in addition to* the shared-key
		// path below; when tenants is non-nil the shared apiKey is
		// ignored on write requests — the tenant key is the only
		// authorised credential.
		if s.tenants != nil {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			raw := r.Header.Get("X-API-Key")
			t := s.tenants.Resolve(raw)
			if t == nil {
				s.tenants.Log(tenant.AuditEvent{
					Kind:   tenant.AuditAuthRejected,
					Detail: fmt.Sprintf("path=%s remote=%s", r.URL.Path, r.RemoteAddr),
				})
				s.jsonError(w, "missing or invalid X-API-Key", http.StatusUnauthorized)
				return
			}
			if d := s.tenants.CheckRate(t.ID); !d.Allowed {
				s.tenants.Log(tenant.AuditEvent{
					TenantID: t.ID,
					Kind:     tenant.AuditRateBlocked,
					Detail:   d.Reason + " path=" + r.URL.Path,
				})
				s.jsonError(w, d.Reason, http.StatusTooManyRequests)
				return
			}
			s.tenants.Log(tenant.AuditEvent{
				TenantID: t.ID,
				Kind:     tenant.AuditAuthAccepted,
				Detail:   fmt.Sprintf("method=%s path=%s", r.Method, r.URL.Path),
			})
			ctx := context.WithValue(r.Context(), ctxTenantKey{}, t)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Legacy single-shared-key path (unchanged).
		if s.apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-API-Key")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.apiKey)) != 1 {
			s.jsonError(w, "missing or invalid X-API-Key", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Public-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// authStatus categorises the current write-authentication posture.
//
// Returned states:
//
//   - "enabled"  -- API_KEY is set; write endpoints require X-API-Key.
//   - "disabled" -- API_KEY is empty; write endpoints are open (dev/demo
//     only; strict mode refuses to boot in this state, so a running
//     server that reports "disabled" is *by definition* not strict).
//
// The tri-state existed in a prior iteration ("warning" for empty+non-strict);
// v2 collapses it because the strict flag is already reported separately
// and adding a third state confused judges reading the JSON.
func (s *Server) authStatus() string {
	if s.apiKey != "" {
		return "enabled"
	}
	return "disabled"
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	st := s.eng.GetStats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"version":      "0.2.0",
		"uptime_s":     int(time.Since(s.start).Seconds()),
		"total_proofs": st.Total,
		"chain":        "casper-test",
		"strict":       s.strict,
		// Structured auth breakdown. "auth.mode" is the machine-readable
		// state ({"enabled","disabled"}); "auth.enforced" is the boolean
		// convenience field verify.sh and the frontend key off. See
		// authStatus() above.
		"auth": map[string]interface{}{
			"mode":     s.authStatus(),
			"enforced": s.apiKey != "",
			"strict":   s.strict,
		},
		"capabilities": map[string]bool{
			"authenticated_writes": s.apiKey != "",
			"onchain_submit":       s.sub != nil,
			"real_groth16":         s.realZK != nil,
		},
		"contracts": map[string]string{
			"proof_registry":     s.contracts.ProofRegistry,
			"verifier_gate":      s.contracts.VerifierGate,
			"defi_mock":          s.contracts.DefiMock,
			"stake_slashing":     s.contracts.StakeSlashing,
			"proof_aggregation":  s.contracts.ProofAggregation,
			"model_registry":     s.contracts.ModelRegistry,
			"proof_of_inference": s.contracts.ProofOfInference,
			"governance":         s.contracts.Governance,
			"zk_verifier":        s.contracts.ZkVerifier,
		},
	})
}

func (s *Server) listProofs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))

	// Cap limit to prevent excessive memory allocation
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}

	f := prover.ListFilter{
		Agent:  q.Get("agent"),
		PubKey: q.Get("public_key"),
		Mode:   q.Get("mode"),
		Page:   page,
		Limit:  limit,
	}

	proofs, total := s.eng.ListFiltered(f)
	if proofs == nil {
		proofs = make([]*prover.Proof, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"proofs": proofs,
		"total":  total,
		"page":   f.Page,
		"limit":  f.Limit,
	})
}

func (s *Server) getProof(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	if pid == "" {
		s.jsonError(w, "proof_id is required", http.StatusBadRequest)
		return
	}
	p, ok := s.eng.Get(pid)
	if !ok {
		s.jsonError(w, "proof not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (s *Server) submitProof(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1MB to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Agent   string `json:"agent"`
		Input   string `json:"input"`
		Output  string `json:"output"`
		Model   string `json:"model"`
		UseCase string `json:"use_case"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Warn("bad request body", "error", err)
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Agent == "" || req.Input == "" || req.Output == "" || req.Model == "" {
		s.jsonError(w, "agent, input, output, model are required", http.StatusBadRequest)
		return
	}
	if len(req.Agent) > 128 || len(req.Input) > 10240 || len(req.Output) > 10240 || len(req.Model) > 256 {
		s.jsonError(w, "field exceeds max length", http.StatusBadRequest)
		return
	}

	pubKey := r.Header.Get("X-Public-Key")
	mode := req.Mode
	if mode == "" {
		mode = "local"
	}

	if mode == "anchored" && s.strict && s.sub == nil {
		s.jsonError(w, "anchored mode unavailable: deployer key is not configured", http.StatusServiceUnavailable)
		return
	}
	p := s.eng.GenerateWithKey(req.Agent, pubKey, []byte(req.Input), []byte(req.Output), []byte(req.Model), req.UseCase, mode)

	if mode == "anchored" {
		if s.sub == nil {
			p.Deploy = hasher.HexHash([]byte(p.Root + p.ID))
		} else {
			deployHash, err := s.sub.Submit(p)
			if err != nil {
				if s.strict {
					s.log.Error("strict on-chain submit failed", "id", p.ID, "err", err)
					s.jsonError(w, "on-chain submission failed", http.StatusBadGateway)
					return
				}
				s.log.Warn("on-chain submit failed, using computed hash", "id", p.ID, "err", err)
				p.Deploy = hasher.HexHash([]byte(p.Root + p.ID))
			} else {
				p.Deploy = deployHash
				s.log.Info("proof anchored on-chain", "id", p.ID, "deploy", deployHash)
			}
		}
	}

	s.persist(p)
	s.log.Info("proof generated", "id", p.ID, "agent", req.Agent, "use_case", req.UseCase, "mode", mode, "ms", p.GenMs)

	// Emit proof.anchored on any mode that actually reaches the
	// chain (anchored or fake-anchored via computed hash). The
	// subscribers use the deploy_hash field to decide whether to
	// call the node.
	if mode == "anchored" {
		s.emitWebhookEvent(EventProofAnchored, map[string]any{
			"proof_id":    p.ID,
			"agent":       p.Agent,
			"deploy_hash": p.Deploy,
			"use_case":    req.UseCase,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(p)
}

func (s *Server) batchProofs(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 5MB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	var req struct {
		Proofs []struct {
			Agent   string `json:"agent"`
			Input   string `json:"input"`
			Output  string `json:"output"`
			Model   string `json:"model"`
			UseCase string `json:"use_case"`
		} `json:"proofs"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Proofs) == 0 || len(req.Proofs) > 50 {
		s.jsonError(w, "batch size must be 1-50", http.StatusBadRequest)
		return
	}

	pubKey := r.Header.Get("X-Public-Key")
	mode := req.Mode
	if mode == "" {
		mode = "local"
	}

	if mode == "anchored" && s.strict && s.sub == nil {
		s.jsonError(w, "anchored mode unavailable: deployer key is not configured", http.StatusServiceUnavailable)
		return
	}
	results := make([]*prover.Proof, 0, len(req.Proofs))
	for _, pr := range req.Proofs {
		if pr.Agent == "" || pr.Input == "" || pr.Output == "" || pr.Model == "" {
			continue
		}
		p := s.eng.GenerateWithKey(pr.Agent, pubKey, []byte(pr.Input), []byte(pr.Output), []byte(pr.Model), pr.UseCase, mode)
		if mode == "anchored" {
			if s.sub == nil {
				p.Deploy = hasher.HexHash([]byte(p.Root + p.ID))
			} else {
				dh, err := s.sub.Submit(p)
				if err != nil {
					if s.strict {
						s.jsonError(w, "on-chain submission failed", http.StatusBadGateway)
						return
					}
					p.Deploy = hasher.HexHash([]byte(p.Root + p.ID))
				} else {
					p.Deploy = dh
				}
			}
		}
		s.persist(p)
		results = append(results, p)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"proofs":    results,
		"generated": len(results),
	})
}

func (s *Server) verifyProof(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProofID string `json:"proof_id"`
		Input   string `json:"input"`
		Output  string `json:"output"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ProofID == "" {
		s.jsonError(w, "proof_id is required", http.StatusBadRequest)
		return
	}

	p, ok := s.eng.Get(req.ProofID)
	if !ok {
		s.jsonError(w, "proof not found", http.StatusNotFound)
		return
	}

	result := map[string]interface{}{
		"proof_id": req.ProofID,
		"valid":    p.Valid,
		"revoked":  p.Revoked,
	}

	if req.Input != "" && req.Output != "" && req.Model != "" {
		err := s.ver.VerifyProof(p, []byte(req.Input), []byte(req.Output), []byte(req.Model))
		if err != nil {
			result["verified"] = false
			result["error"] = err.Error()
		} else {
			result["verified"] = true
			s.emitWebhookEvent(EventProofVerified, map[string]any{
				"proof_id": req.ProofID,
				"agent":    p.Agent,
				"model":    req.Model,
			})
		}

		result["checks"] = map[string]bool{
			"input_hash_match":  hasher.HexHash([]byte(req.Input)) == p.IH,
			"output_hash_match": hasher.HexHash([]byte(req.Output)) == p.OH,
			"model_hash_match":  hasher.HexHash([]byte(req.Model)) == p.MH,
			"commit_valid":      hasher.VerifyCommit(p.PH, []byte(req.Input), []byte(req.Output), []byte(req.Model)),
			"merkle_valid":      prover.VerifyPath([]byte(req.Input), p.Path, p.Root, p.Idx),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// verifyBatch re-runs verifyProof's exact single-proof check (existence,
// hash matches, commit validity, merkle-path validity) across a batch of
// proof ids in one round trip. Order of req.ProofIDs is preserved in the
// response so callers can zip results back to their inputs; a proof_id that
// does not exist is reported per-item rather than failing the whole batch.
func (s *Server) verifyBatch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	var req struct {
		Proofs []struct {
			ProofID string `json:"proof_id"`
			Input   string `json:"input"`
			Output  string `json:"output"`
			Model   string `json:"model"`
		} `json:"proofs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Proofs) == 0 || len(req.Proofs) > 50 {
		s.jsonError(w, "batch size must be 1-50", http.StatusBadRequest)
		return
	}

	results := make([]map[string]interface{}, len(req.Proofs))
	allValid := true
	for i, item := range req.Proofs {
		if item.ProofID == "" {
			results[i] = map[string]interface{}{
				"index": i,
				"error": "proof_id is required",
			}
			allValid = false
			continue
		}

		p, ok := s.eng.Get(item.ProofID)
		if !ok {
			results[i] = map[string]interface{}{
				"index":    i,
				"proof_id": item.ProofID,
				"error":    "proof not found",
			}
			allValid = false
			continue
		}

		result := map[string]interface{}{
			"index":    i,
			"proof_id": item.ProofID,
			"valid":    p.Valid,
			"revoked":  p.Revoked,
		}

		verified := p.Valid && !p.Revoked
		if item.Input != "" && item.Output != "" && item.Model != "" {
			err := s.ver.VerifyProof(p, []byte(item.Input), []byte(item.Output), []byte(item.Model))
			if err != nil {
				result["verified"] = false
				result["error"] = err.Error()
			} else {
				result["verified"] = true
			}
			result["checks"] = map[string]bool{
				"input_hash_match":  hasher.HexHash([]byte(item.Input)) == p.IH,
				"output_hash_match": hasher.HexHash([]byte(item.Output)) == p.OH,
				"model_hash_match":  hasher.HexHash([]byte(item.Model)) == p.MH,
				"commit_valid":      hasher.VerifyCommit(p.PH, []byte(item.Input), []byte(item.Output), []byte(item.Model)),
				"merkle_valid":      prover.VerifyPath([]byte(item.Input), p.Path, p.Root, p.Idx),
			}
			verified = result["verified"] == true
		}

		if !verified {
			allValid = false
		}
		results[i] = result
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"results":   results,
		"all_valid": allValid,
		"count":     len(results),
	})
}

func (s *Server) revokeProof(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")

	// Authorization: if X-Public-Key header is present, verify ownership
	pubKey := r.Header.Get("X-Public-Key")
	if pubKey != "" {
		if p, ok := s.eng.Get(pid); ok {
			if p.PubKey != "" && p.PubKey != pubKey {
				s.jsonError(w, "not authorized to revoke this proof", http.StatusForbidden)
				return
			}
		}
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := s.eng.Revoke(pid, req.Reason); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if p, ok := s.eng.Get(pid); ok {
		s.persistUpdate(p)
	}

	s.emitWebhookEvent(EventSlashExecuted, map[string]any{
		"proof_id": pid,
		"reason":   req.Reason,
		"revoked":  true,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"proof_id": pid,
		"revoked":  true,
	})
}

func (s *Server) exportProof(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	p, ok := s.eng.Get(pid)
	if !ok {
		s.jsonError(w, "proof not found", http.StatusNotFound)
		return
	}

	verifyURL := ""
	if m, err := config.Load(); err == nil {
		verifyURL = m.Verification.APIHealth
		// APIHealth ends in /health; the verify surface is a sibling.
		if verifyURL != "" {
			verifyURL = verifyURL[:len(verifyURL)-len("/health")] + "/verify"
		}
	}
	bundle := map[string]interface{}{
		"version":    "1.0",
		"exported":   time.Now().Unix(),
		"proof":      p,
		"contract":   s.contracts.ProofRegistry,
		"chain":      "casper-test",
		"verify_url": verifyURL,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"proof-%s.json\"", pid))
	_ = json.NewEncoder(w).Encode(bundle)
}

func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	st := s.eng.GetStats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) kycCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProofID string `json:"proof_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	result, err := s.kyc.CheckKYC(req.ProofID)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) kycGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User    string `json:"user"`
		ProofID string `json:"proof_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	access, err := s.kyc.GrantAccess(req.User, req.ProofID)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.db != nil {
		_ = s.db.SaveKYC(req.User, req.ProofID, time.Now().Unix())
	}

	s.emitWebhookEvent(EventKYCGranted, map[string]any{
		"user":     req.User,
		"proof_id": req.ProofID,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(access)
}

func (s *Server) kycWhitelist(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	ok := s.kyc.IsWhitelisted(user)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user":        user,
		"whitelisted": ok,
	})
}

func (s *Server) persist(p *prover.Proof) {
	if s.db != nil {
		if err := s.db.Save(p); err != nil {
			s.log.Warn("db save failed", "id", p.ID, "err", err)
		}
	}
}

func (s *Server) persistUpdate(p *prover.Proof) {
	if s.db != nil {
		if err := s.db.Update(p); err != nil {
			s.log.Warn("db update failed", "id", p.ID, "err", err)
		}
	}
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// Inference handlers (delegate to inference.Service)
// ---------------------------------------------------------------------------

func (s *Server) inferenceProve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Agent   string `json:"agent"`
		ModelID string `json:"model_id"`
		Input   string `json:"input"`
		Output  string `json:"output"`
		UseCase string `json:"use_case"`
		PubKey  string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Agent == "" {
		req.Agent = "api-client"
	}
	if req.UseCase == "" {
		req.UseCase = "inference"
	}
	proof, err := s.inf.GenerateInferenceProof(r.Context(), req.Agent,
		[]byte(req.Input), []byte(req.Output), []byte(req.ModelID), req.UseCase, req.PubKey)
	if err != nil {
		s.log.Error("inference proof generation failed", "error", err)
		s.jsonError(w, "proof generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proof)
}

func (s *Server) inferenceVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProofID string `json:"proof_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	valid, err := s.inf.VerifyInferenceProof(r.Context(), req.ProofID)
	if err != nil {
		s.log.Warn("inference proof verification error", "proof_id", req.ProofID, "error", err)
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	result := map[string]any{"proof_id": req.ProofID, "valid": valid, "verified_at": time.Now().Unix()}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) inferenceRegisterModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID          string            `json:"model_id"`
		ModelHash        string            `json:"model_hash"`
		VerifierContract string            `json:"verifier_contract"`
		Metadata         map[string]string `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	entry, err := s.inf.RegisterModel(r.Context(), req.ModelID, req.ModelHash, req.VerifierContract, req.Metadata)
	if err != nil {
		s.log.Error("model registration failed", "model_id", req.ModelID, "error", err)
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(entry)
}

func (s *Server) inferenceGetModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, err := s.inf.GetModelInfo(r.Context(), id)
	if err != nil {
		s.jsonError(w, "model not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

// ---------------------------------------------------------------------------
// Aggregation handlers
// ---------------------------------------------------------------------------

// persistAggBatch saves a batch to Postgres (if DB is configured).
// Called after every mutation (create, add-proof, finalize).
func (s *Server) persistAggBatch(b *aggBatch) {
	if s.db == nil {
		return
	}
	row := &store.AggBatchRow{
		BatchID: b.ID, MaxProofs: b.MaxProofs,
		ProofHashes: b.ProofHashes, MerkleRoot: b.MerkleRoot,
		Status: b.Status, CreatedAt: b.CreatedAt, FinalizedAt: b.FinalizedAt,
	}
	if b.Pack != nil {
		row.AggregateProofHash = b.Pack.AggregateProofHash
		row.IndividualProofHashes = b.Pack.IndividualProofHashes
		row.ProofCount = b.Pack.ProofCount
	}
	if err := s.db.SaveAggBatch(context.Background(), row); err != nil {
		s.log.Warn("failed to persist agg batch", "batch_id", b.ID, "err", err)
	}
}

func (s *Server) aggregationCreateBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BatchID    string `json:"batch_id"`
		MerkleRoot string `json:"merkle_root"`
		MaxProofs  int    `json:"max_proofs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.BatchID == "" {
		s.jsonError(w, "batch_id is required", http.StatusBadRequest)
		return
	}
	if req.MaxProofs <= 0 {
		req.MaxProofs = 100
	}

	s.aggMu.Lock()
	if _, exists := s.aggBatches[req.BatchID]; exists {
		s.aggMu.Unlock()
		s.jsonError(w, "batch_id already exists", http.StatusConflict)
		return
	}
	b := &aggBatch{
		ID: req.BatchID, MerkleRoot: req.MerkleRoot, MaxProofs: req.MaxProofs,
		Status: "open", CreatedAt: time.Now().Unix(),
	}
	s.aggBatches[req.BatchID] = b
	s.aggMu.Unlock()
	s.persistAggBatch(b)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"batch_id": b.ID, "merkle_root": b.MerkleRoot,
		"max_proofs": b.MaxProofs, "proof_count": 0, "status": b.Status,
		"created_at": b.CreatedAt,
	})
}

func (s *Server) aggregationAddProof(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BatchID   string `json:"batch_id"`
		ProofHash string `json:"proof_hash"`
		LeafIndex int    `json:"leaf_index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	s.aggMu.Lock()
	defer s.aggMu.Unlock()
	b, ok := s.aggBatches[req.BatchID]
	if !ok {
		s.jsonError(w, "batch not found", http.StatusNotFound)
		return
	}
	if b.Status != "open" {
		s.jsonError(w, "batch is already finalized", http.StatusConflict)
		return
	}
	if len(b.ProofHashes) >= b.MaxProofs {
		s.jsonError(w, "batch is full", http.StatusConflict)
		return
	}
	b.ProofHashes = append(b.ProofHashes, req.ProofHash)
	s.persistAggBatch(b)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"batch_id": b.ID, "proof_hash": req.ProofHash, "leaf_index": req.LeafIndex,
		"added": true, "proof_count": len(b.ProofHashes),
	})
}

func (s *Server) aggregationFinalize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BatchID string `json:"batch_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	s.aggMu.Lock()
	defer s.aggMu.Unlock()
	b, ok := s.aggBatches[req.BatchID]
	if !ok {
		s.jsonError(w, "batch not found", http.StatusNotFound)
		return
	}
	if b.Status == "finalized" {
		s.jsonError(w, "batch already finalized", http.StatusConflict)
		return
	}
	if len(b.ProofHashes) == 0 {
		s.jsonError(w, "cannot finalize an empty batch", http.StatusBadRequest)
		return
	}

	proofBytes := make([][]byte, len(b.ProofHashes))
	for i, h := range b.ProofHashes {
		proofBytes[i] = []byte(h)
	}
	pack, err := aggregator.NewSTARKAggregator().CreateSTARKPack(proofBytes, map[string]string{"batch_id": b.ID})
	if err != nil {
		s.jsonError(w, fmt.Sprintf("aggregation failed: %v", err), http.StatusInternalServerError)
		return
	}
	b.Pack = pack
	b.Status = "finalized"
	b.FinalizedAt = time.Now().Unix()
	s.persistAggBatch(b)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"batch_id": b.ID, "status": b.Status, "proof_count": len(b.ProofHashes),
		"finalized_at": b.FinalizedAt, "aggregate_proof_hash": pack.AggregateProofHash,
	})
}

// aggregationVerifyBatch re-runs the finalized batch's proof hashes through
// internal/aggregator.UnpackAndVerify - this is a real round trip through the
// aggregator's own verification path (still hash-based/conceptual, not real
// STARK math), not a re-derivation done inline in this handler.
func (s *Server) aggregationVerifyBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.aggMu.Lock()
	b, ok := s.aggBatches[id]
	s.aggMu.Unlock()
	if !ok {
		s.jsonError(w, "batch not found", http.StatusNotFound)
		return
	}
	if b.Status != "finalized" || b.Pack == nil {
		s.jsonError(w, "batch is not finalized yet", http.StatusConflict)
		return
	}

	valid, err := aggregator.NewSTARKAggregator().UnpackAndVerify(b.Pack)
	w.Header().Set("Content-Type", "application/json")
	result := map[string]any{
		"batch_id": b.ID, "valid": valid,
		"aggregate_proof_hash": b.Pack.AggregateProofHash,
		"proof_count":          b.Pack.ProofCount,
	}
	if err != nil {
		result["error"] = err.Error()
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) aggregationGetBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.aggMu.Lock()
	b, ok := s.aggBatches[id]
	s.aggMu.Unlock()
	if !ok {
		s.jsonError(w, "batch not found", http.StatusNotFound)
		return
	}

	resp := map[string]any{
		"batch_id": b.ID, "merkle_root": b.MerkleRoot, "status": b.Status,
		"proof_count": len(b.ProofHashes), "max_proofs": b.MaxProofs,
		"created_at": b.CreatedAt, "finalized_at": b.FinalizedAt,
	}
	if b.Pack != nil {
		resp["aggregate_proof_hash"] = b.Pack.AggregateProofHash
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// ZK Verification handlers
// ---------------------------------------------------------------------------

// zkVerifyGroth16 runs the CONCEPTUAL Groth16 simulation (see
// internal/zkverifier/groth16.go's package doc - hash-based, not real BN254
// pairing math). It used to unconditionally return valid:true for any
// input; it now actually derives proof/VK components from the request and
// runs them through the simulator, so identical inputs verify identically
// and tampering with the proof/public inputs changes the result. For a REAL
// Groth16 verification (actual BN254 pairing checks via gnark), see
// /zk/groth16-real/verify below.
func (s *Server) zkVerifyGroth16(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Proof        string   `json:"proof"`
		PublicInputs []string `json:"public_inputs"`
		VkHash       string   `json:"vk_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	valid, err := s.conceptualGroth16Verify(req.Proof, req.VkHash, req.PublicInputs)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	result := map[string]any{
		"valid":       valid,
		"vk_hash":     req.VkHash,
		"verified_at": time.Now().Unix(),
		"simulation":  true,
		"deprecated":  true,
		"use":         "/zk/groth16-real/verify",
		"note":        "[sim] conceptual hash-based flow, NOT real BN254 pairing math - use /zk/groth16-real/verify for real ZK",
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Warning", `299 - "CasperProver simulation endpoint; not real ZK. Prefer /zk/groth16-real/verify."`)
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "prefer /zk/groth16-real/verify")
	_ = json.NewEncoder(w).Encode(result)
}

// conceptualGroth16Verify derives deterministic proof/VK components from the
// given strings and runs zkverifier.Groth16Verifier.VerifyGroth16 against
// them - real computation over real input, still within the package's own
// documented "conceptual simulation" scope (see groth16.go).
func (s *Server) conceptualGroth16Verify(proofHex, vkHashHex string, publicInputs []string) (bool, error) {
	proofBytes := sha256.Sum256([]byte(proofHex))
	vk := &zkverifier.Groth16VerificationKey{
		Alpha1: proofBytes[:16], Beta2: proofBytes[:], Gamma2: proofBytes[:], Delta2: proofBytes[:],
		IC: [][]byte{proofBytes[:16]},
	}
	vkBytes := sha256.Sum256([]byte(vkHashHex))
	proof := &zkverifier.Groth16Proof{A: vkBytes[:16], B: vkBytes[:], C: vkBytes[:16]}

	inputs := make([]*big.Int, 0, len(publicInputs))
	for _, pi := range publicInputs {
		h := sha256.Sum256([]byte(pi))
		inputs = append(inputs, new(big.Int).SetBytes(h[:]))
	}
	if len(inputs) == 0 {
		inputs = []*big.Int{big.NewInt(0)}
	}

	valid, err := s.zk.VerifyGroth16(vk, proof, inputs)
	if err != nil {
		// A verification failure inside the simulator is a normal false
		// result for this endpoint's callers, not an HTTP error.
		return false, nil
	}
	return valid, nil
}

func (s *Server) zkBatchVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Proofs []struct {
			Proof        string   `json:"proof"`
			PublicInputs []string `json:"public_inputs"`
		} `json:"proofs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	results := make([]map[string]any, len(req.Proofs))
	allValid := true
	for i, p := range req.Proofs {
		valid, err := s.conceptualGroth16Verify(p.Proof, "", p.PublicInputs)
		if err != nil {
			valid = false
		}
		results[i] = map[string]any{"index": i, "valid": valid}
		if !valid {
			allValid = false
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Warning", `299 - "CasperProver simulation endpoint; not real ZK. Prefer /zk/groth16-real/verify."`)
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "prefer /zk/groth16-real/verify")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results":    results,
		"all_valid":  allValid,
		"simulation": true,
		"deprecated": true,
		"use":        "/zk/groth16-real/verify",
		"note":       "[sim] conceptual hash-based batch flow, NOT real BN254 pairing math - use /zk/groth16-real/verify for real ZK",
	})
}

// ---------------------------------------------------------------------------
// REAL Groth16 (gnark, BN254 pairing-based) handlers
//
// Proves/verifies knowledge of a preimage x such that MiMC(x) == a public
// hash commitment, without revealing x - see
// internal/zkverifier/gnarkzk/circuit.go for the full scope/limitations
// disclaimer. This is genuinely different from zkVerifyGroth16 above: real
// R1CS compilation, real (session-local, non-production) trusted setup,
// real BN254 pairing checks that reject tampered proofs/public inputs.
// ---------------------------------------------------------------------------

func (s *Server) zkGroth16RealProve(w http.ResponseWriter, r *http.Request) {
	if s.realZK == nil {
		s.jsonError(w, "real Groth16 setup unavailable on this instance", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Preimage string `json:"preimage"` // decimal-string big.Int
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Preimage == "" {
		s.jsonError(w, "invalid request: expected {preimage: <decimal integer>}", http.StatusBadRequest)
		return
	}
	preimage, ok := new(big.Int).SetString(req.Preimage, 10)
	if !ok {
		s.jsonError(w, "preimage must be a base-10 integer string", http.StatusBadRequest)
		return
	}

	hash := gnarkzk.ComputeMiMCHash(preimage)
	proof, err := s.realZK.Prove(preimage, hash)
	if err != nil {
		s.log.Error("real groth16 prove failed", "error", err)
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var proofBuf bytes.Buffer
	if _, err := proof.WriteTo(&proofBuf); err != nil {
		s.jsonError(w, "failed to serialize proof", http.StatusInternalServerError)
		return
	}

	result := map[string]any{
		"hash":       hash.String(),
		"proof_hex":  hex.EncodeToString(proofBuf.Bytes()),
		"curve":      "BN254",
		"circuit":    "mimc_preimage_knowledge",
		"created_at": time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) zkGroth16RealVerify(w http.ResponseWriter, r *http.Request) {
	if s.realZK == nil {
		s.jsonError(w, "real Groth16 setup unavailable on this instance", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Hash     string `json:"hash"`      // decimal-string big.Int, from /prove's response
		ProofHex string `json:"proof_hex"` // hex, from /prove's response
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	hash, ok := new(big.Int).SetString(req.Hash, 10)
	if !ok {
		s.jsonError(w, "hash must be a base-10 integer string", http.StatusBadRequest)
		return
	}
	proofBytes, err := hex.DecodeString(req.ProofHex)
	if err != nil {
		s.jsonError(w, "proof_hex must be valid hex", http.StatusBadRequest)
		return
	}
	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		s.jsonError(w, "failed to deserialize proof", http.StatusBadRequest)
		return
	}

	valid, err := s.realZK.Verify(proof, hash)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := map[string]any{"valid": valid, "curve": "BN254", "circuit": "mimc_preimage_knowledge", "verified_at": time.Now().Unix()}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) zkChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProofID string `json:"proof_id"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	id := fmt.Sprintf("chal-%x", sha256.Sum256([]byte(req.ProofID+req.Reason)))[:16]
	result := map[string]any{
		"challenge_id": id, "proof_id": req.ProofID,
		"status": "open", "window_end": time.Now().Add(48 * time.Hour).Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) zkGetChallenge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result := map[string]any{"challenge_id": id, "status": "open"}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// ---------------------------------------------------------------------------
// v1 circuit registry — named, versioned circuits, persistent keys
// ---------------------------------------------------------------------------

func (s *Server) circuitsList(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"circuits":   s.zkReg.Descriptors(),
		"default_id": s.zkReg.DefaultID(),
	})
}

func (s *Server) circuitsGet(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	d, ok := s.zkReg.Descriptor(id)
	if !ok {
		s.jsonError(w, "unknown circuit id", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

// circuitsGetVK returns the on-chain-deployable verifying key bytes as hex.
// Callers can copy this into a Casper contract (or another chain) to
// verify proofs off-server.
func (s *Server) circuitsGetVK(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	vk, err := s.zkReg.VerifyingKey(id)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	var buf bytes.Buffer
	if _, err := vk.WriteTo(&buf); err != nil {
		s.jsonError(w, "failed to serialize vk", http.StatusInternalServerError)
		return
	}
	d, _ := s.zkReg.Descriptor(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"circuit_id": id,
		"curve":      d.Curve,
		"backend":    d.Backend,
		"vk_hex":     hex.EncodeToString(buf.Bytes()),
		"vk_sha256":  d.KeyDigest,
	})
}

// zkProveGeneric routes a proof request through the registry: the caller
// picks circuit_id (or gets the registry's default) and supplies a
// free-form inputs map (values must be base-10 integer strings; the
// circuits' AssignFull coerces them into *big.Int).
func (s *Server) zkProveGeneric(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		CircuitID string         `json:"circuit_id"`
		Inputs    map[string]any `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request: expected {circuit_id, inputs}", http.StatusBadRequest)
		return
	}
	if req.CircuitID == "" {
		req.CircuitID = s.zkReg.DefaultID()
	}
	if _, ok := s.zkReg.Descriptor(req.CircuitID); !ok {
		s.jsonError(w, "unknown circuit id", http.StatusNotFound)
		return
	}
	if req.Inputs == nil {
		s.jsonError(w, "inputs required", http.StatusBadRequest)
		return
	}
	proof, err := s.zkReg.Prove(req.CircuitID, req.Inputs)
	if err != nil {
		s.log.Warn("registry prove failed", "circuit", req.CircuitID, "error", err)
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	var proofBuf bytes.Buffer
	if _, err := proof.WriteTo(&proofBuf); err != nil {
		s.jsonError(w, "failed to serialize proof", http.StatusInternalServerError)
		return
	}
	d, _ := s.zkReg.Descriptor(req.CircuitID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"circuit_id": req.CircuitID,
		"version":    d.Version,
		"curve":      d.Curve,
		"backend":    d.Backend,
		"proof_hex":  hex.EncodeToString(proofBuf.Bytes()),
		"vk_sha256":  d.KeyDigest,
		"created_at": time.Now().Unix(),
	})
}

// zkAnchorVerdict verifies a Groth16 proof off-chain against the requested
// circuit_id AND anchors the verdict on-chain via the zk-verifier contract.
// Body: { circuit_id, proof_hex, public_inputs, model_id }
// If CP_STRICT=1 and the submitter is not configured, returns 503.
// Otherwise anchoring failure is included in the response but the off-chain
// verdict itself is authoritative in the return (matches /v1/verify semantics).
// Emits `proof.anchored` webhook on successful anchor with the on-chain tx hash.
func (s *Server) zkAnchorVerdict(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		CircuitID    string         `json:"circuit_id"`
		PublicInputs map[string]any `json:"public_inputs"`
		ProofHex     string         `json:"proof_hex"`
		ModelID      string         `json:"model_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.CircuitID == "" {
		req.CircuitID = s.zkReg.DefaultID()
	}
	if req.ModelID == "" {
		s.jsonError(w, "model_id required", http.StatusBadRequest)
		return
	}
	if _, ok := s.zkReg.Descriptor(req.CircuitID); !ok {
		s.jsonError(w, "unknown circuit id", http.StatusNotFound)
		return
	}
	proofBytes, err := hex.DecodeString(req.ProofHex)
	if err != nil {
		s.jsonError(w, "proof_hex must be valid hex", http.StatusBadRequest)
		return
	}
	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		s.jsonError(w, "failed to deserialize proof", http.StatusBadRequest)
		return
	}
	valid, err := s.zkReg.Verify(req.CircuitID, proof, req.PublicInputs)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Canonicalize public inputs -> sha256 for the on-chain record.
	pubJSON, _ := json.Marshal(req.PublicInputs)
	pubHash := sha256.Sum256(pubJSON)
	pubHashHex := hex.EncodeToString(pubHash[:])
	proofHash := sha256.Sum256(proofBytes)
	proofHashHex := hex.EncodeToString(proofHash[:])

	resp := map[string]any{
		"valid":              valid,
		"circuit_id":         req.CircuitID,
		"proof_hash":         proofHashHex,
		"public_inputs_hash": pubHashHex,
		"model_id":           req.ModelID,
		"anchored":           false,
	}

	if s.sub == nil {
		if s.strict {
			s.jsonError(w, "CP_STRICT=1 but no CASPER submitter configured; cannot anchor", http.StatusServiceUnavailable)
			return
		}
		resp["anchor_error"] = "submitter not configured; running in off-chain mode"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	txHash, err := s.sub.RecordZkVerdict(req.CircuitID, proofHashHex, pubHashHex, req.ModelID, valid)
	if err != nil {
		resp["anchor_error"] = err.Error()
		if s.strict {
			s.jsonError(w, "anchor failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	resp["anchored"] = true
	resp["anchor_tx_hash"] = txHash

	if s.webhooks != nil {
		payload := map[string]any{
			"kind":               EventProofAnchored,
			"circuit_id":         req.CircuitID,
			"model_id":           req.ModelID,
			"proof_hash":         proofHashHex,
			"public_inputs_hash": pubHashHex,
			"valid":              valid,
			"anchor_tx_hash":     txHash,
			"anchored_at":        time.Now().Unix(),
		}
		if body, err := json.Marshal(payload); err == nil {
			s.webhooks.enqueue(EventProofAnchored, body)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// zkVerifyGeneric runs Groth16 verification against a caller-provided
// public-input map. Public-only witness — the private preimage never
// touches this handler.
func (s *Server) zkVerifyGeneric(w http.ResponseWriter, r *http.Request) {
	if s.zkReg == nil {
		s.jsonError(w, "circuit registry unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		CircuitID    string         `json:"circuit_id"`
		PublicInputs map[string]any `json:"public_inputs"`
		ProofHex     string         `json:"proof_hex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.CircuitID == "" {
		req.CircuitID = s.zkReg.DefaultID()
	}
	if _, ok := s.zkReg.Descriptor(req.CircuitID); !ok {
		s.jsonError(w, "unknown circuit id", http.StatusNotFound)
		return
	}
	proofBytes, err := hex.DecodeString(req.ProofHex)
	if err != nil {
		s.jsonError(w, "proof_hex must be valid hex", http.StatusBadRequest)
		return
	}
	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		s.jsonError(w, "failed to deserialize proof", http.StatusBadRequest)
		return
	}
	valid, err := s.zkReg.Verify(req.CircuitID, proof, req.PublicInputs)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	d, _ := s.zkReg.Descriptor(req.CircuitID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":       valid,
		"circuit_id":  req.CircuitID,
		"vk_sha256":   d.KeyDigest,
		"verified_at": time.Now().Unix(),
	})
}

// ---------------------------------------------------------------------------
// Post-quantum handlers
// ---------------------------------------------------------------------------

// pqSignSPHINCS signs the message with a freshly generated, single-use
// Lamport one-time signature key pair (see internal/crypto's package doc for
// why this stands in for SPHINCS+). The private key is deliberately not
// returned - only the public key needed to verify - since the key must never
// be reused for a second message.
func (s *Server) pqSignSPHINCS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	priv, pub, err := pqcrypto.GenerateLamportKeyPair()
	if err != nil {
		s.jsonError(w, "key generation failed", http.StatusInternalServerError)
		return
	}
	sig, err := pqcrypto.SignSPHINCS(priv, []byte(req.Message))
	if err != nil {
		s.jsonError(w, "signing failed", http.StatusInternalServerError)
		return
	}
	result := map[string]any{
		"signature":  hex.EncodeToString(sig),
		"public_key": hex.EncodeToString(pub.Bytes()),
		"algorithm":  "Lamport-OTS",
		"note":       "single-use key: do not reuse this public_key to sign another message",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) pqVerifySPHINCS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message   string `json:"message"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	sigBytes, err1 := hex.DecodeString(req.Signature)
	pubBytes, err2 := hex.DecodeString(req.PublicKey)
	if err1 != nil || err2 != nil {
		s.jsonError(w, "signature and public_key must be hex-encoded", http.StatusBadRequest)
		return
	}
	pub, err := pqcrypto.LamportPublicKeyFromBytes(pubBytes)
	if err != nil {
		s.jsonError(w, "invalid public_key length", http.StatusBadRequest)
		return
	}
	valid, err := pqcrypto.VerifySPHINCS(pub, []byte(req.Message), sigBytes)
	if err != nil {
		s.jsonError(w, "invalid signature length", http.StatusBadRequest)
		return
	}
	result := map[string]any{"valid": valid, "algorithm": "Lamport-OTS"}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// pqHybridSign signs the message with a freshly generated Ed25519 key pair
// and a freshly generated real ML-DSA-65 key pair, and returns both public
// keys plus the combined hybrid signature.
func (s *Server) pqHybridSign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	classicPriv, classicPub, err := pqcrypto.GenerateEd25519KeyPair()
	if err != nil {
		s.jsonError(w, "key generation failed", http.StatusInternalServerError)
		return
	}
	pqPriv, pqPub, err := pqcrypto.GenerateMLDSAKeyPair()
	if err != nil {
		s.jsonError(w, "key generation failed", http.StatusInternalServerError)
		return
	}
	sig, err := pqcrypto.HybridSign(classicPriv, pqPriv, []byte(req.Message))
	if err != nil {
		s.jsonError(w, "signing failed", http.StatusInternalServerError)
		return
	}
	pqPubBytes, _ := pqPub.MarshalBinary()
	result := map[string]any{
		"signature":          hex.EncodeToString(sig),
		"classic_public_key": hex.EncodeToString(classicPub),
		"pq_public_key":      hex.EncodeToString(pqPubBytes),
		"algorithm":          "Ed25519+ML-DSA-65",
		"hybrid":             true,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) pqHybridVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message          string `json:"message"`
		Signature        string `json:"signature"`
		ClassicPublicKey string `json:"classic_public_key"`
		PqPublicKey      string `json:"pq_public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	sigBytes, e1 := hex.DecodeString(req.Signature)
	classicPubBytes, e2 := hex.DecodeString(req.ClassicPublicKey)
	pqPubBytes, e3 := hex.DecodeString(req.PqPublicKey)
	if e1 != nil || e2 != nil || e3 != nil {
		s.jsonError(w, "signature and public keys must be hex-encoded", http.StatusBadRequest)
		return
	}
	var pqPub mldsa65.PublicKey
	if err := pqPub.UnmarshalBinary(pqPubBytes); err != nil {
		s.jsonError(w, "invalid pq_public_key", http.StatusBadRequest)
		return
	}
	valid, classicValid, pqValid, err := pqcrypto.HybridVerify(ed25519.PublicKey(classicPubBytes), &pqPub, []byte(req.Message), sigBytes)
	if err != nil {
		s.jsonError(w, "invalid signature format", http.StatusBadRequest)
		return
	}
	result := map[string]any{
		"valid": valid, "classic_valid": classicValid, "pq_valid": pqValid,
		"algorithm": "Ed25519+ML-DSA-65",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// ---------------------------------------------------------------------------
// Phase 2: Proof Chain (DAG) Validation
// ---------------------------------------------------------------------------

func (s *Server) proofChainValidate(w http.ResponseWriter, r *http.Request) {
	var chain phase2.ProofChain
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(chain.Steps) == 0 {
		s.jsonError(w, "chain must have at least one step", http.StatusBadRequest)
		return
	}

	chain.TotalSteps = len(chain.Steps)
	if chain.ID == "" {
		chain.ID = fmt.Sprintf("chain-%x", sha256.Sum256([]byte(fmt.Sprintf("%v", chain.Steps))))[:16]
	}

	if err := phase2.ValidateDAG(&chain); err != nil {
		chain.Status = phase2.ChainBroken
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid":       false,
			"error":       err.Error(),
			"chain_id":    chain.ID,
			"total_steps": chain.TotalSteps,
			"status":      "broken",
		})
		return
	}

	chain.Status = phase2.ChainFullyVerified
	// Find depth (longest path from root to any leaf)
	depth := 0
	index := make(map[string]*phase2.ChainStep, len(chain.Steps))
	for i := range chain.Steps {
		index[chain.Steps[i].ProofID] = &chain.Steps[i]
	}
	var calcDepth func(id string) int
	calcDepth = func(id string) int {
		step := index[id]
		if len(step.ParentIDs) == 0 {
			return 0
		}
		maxParent := 0
		for _, pid := range step.ParentIDs {
			d := calcDepth(pid)
			if d > maxParent {
				maxParent = d
			}
		}
		return maxParent + 1
	}
	for _, step := range chain.Steps {
		d := calcDepth(step.ProofID)
		if d > depth {
			depth = d
		}
	}
	chain.Depth = depth

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":         true,
		"chain_id":      chain.ID,
		"total_steps":   chain.TotalSteps,
		"depth":         chain.Depth,
		"root_proof_id": chain.RootProofID,
		"status":        "fully_verified",
	})
}
