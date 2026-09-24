package inference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/store"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/submitter"
)

// ModelRegistryEntry represents a registered AI model on the Casper blockchain.
type ModelRegistryEntry struct {
	ModelID          string            `json:"model_id"`
	ModelHash        string            `json:"model_hash"`         // Hash of the model's weights/architecture
	VerifierContract string            `json:"verifier_contract"`  // Casper contract address for on-chain verification
	Metadata         map[string]string `json:"metadata,omitempty"` // Additional model metadata
	RegisteredAt     int64             `json:"registered_at"`
	DeployHash       string            `json:"deploy_hash,omitempty"` // Hash of the Casper deploy transaction
}

// InferenceService provides functionalities for generating and verifying
// proofs of AI model inference, and managing a model registry.
type InferenceService struct {
	eng *prover.ProofEngine
	db  *store.PG
	sub *submitter.CasperSubmitter
	log *slog.Logger
}

// New creates a new InferenceService instance.
func New(eng *prover.ProofEngine, db *store.PG, sub *submitter.CasperSubmitter) *InferenceService {
	return &InferenceService{
		eng: eng,
		db:  db,
		sub: sub,
		log: slog.Default().With("module", "inference"),
	}
}

// GenerateInferenceProof generates a ZK-proof for an AI model inference.
// It commits the input, output, and model data, and creates a proof using the
// underlying ProofEngine. The proof can optionally be anchored on-chain by
// providing a public key.
func (s *InferenceService) GenerateInferenceProof(
	ctx context.Context,
	agent string,
	input, output, model []byte,
	uc string, // Use case
	pubKey string, // Public key for on-chain anchoring, if applicable
) (*prover.Proof, error) {
	s.log.Info("generating inference proof", "agent", agent, "use_case", uc)

	// Generate the proof using the core ProofEngine
	// The ProofEngine handles hashing and Merkle tree generation.
	proof := s.eng.GenerateWithKey(agent, pubKey, input, output, model, uc, "inference")

	// Optionally, submit the proof hash to the Casper blockchain if a submitter is available
	if s.sub != nil && pubKey != "" {
		s.log.Info("submitting inference proof to Casper", "proof_id", proof.ID, "public_key", pubKey)
		deployHash, err := s.sub.Submit(proof)
		if err != nil {
			s.log.Error("failed to submit proof to Casper", "proof_id", proof.ID, "error", err)
			// Continue without deploy hash, as local proof generation was successful
		} else {
			proof.Deploy = deployHash
			s.log.Info("proof submitted to Casper", "proof_id", proof.ID, "deploy_hash", deployHash)
		}
	}

	// Store the proof in the database
	if s.db != nil {
		if err := s.db.SaveProof(ctx, proof); err != nil {
			s.log.Error("failed to save proof to database", "proof_id", proof.ID, "error", err)
			return nil, fmt.Errorf("failed to save proof: %w", err)
		}
	}

	// Emit a CEP-88 event for proof generation
	_ = s.emitCEP88Event("ProofGenerated", map[string]interface{}{
		"proof_id":    proof.ID,
		"agent":       proof.Agent,
		"proof_hash":  proof.PH,
		"input_hash":  proof.IH,
		"output_hash": proof.OH,
		"model_hash":  proof.MH,
		"use_case":    proof.UseCase,
		"public_key":  proof.PubKey,
		"deploy_hash": proof.Deploy,
		"timestamp":   proof.TS,
	})

	s.log.Info("inference proof generated successfully", "proof_id", proof.ID, "gen_ms", proof.GenMs)
	return proof, nil
}

// VerifyInferenceProof verifies an existing inference proof by its ID.
// It retrieves the proof from the ProofEngine and performs a local verification.
// For on-chain verification, the deploy hash would be used to query the Casper blockchain.
func (s *InferenceService) VerifyInferenceProof(ctx context.Context, proofID string) (bool, error) {
	s.log.Info("verifying inference proof", "proof_id", proofID)

	p, ok := s.eng.Get(proofID)
	if !ok {
		return false, fmt.Errorf("proof %s not found", proofID)
	}

	// Perform local verification using the ProofEngine's internal logic
	isValid, err := s.eng.Verify(proofID)
	if err != nil {
		s.log.Error("local proof verification failed", "proof_id", proofID, "error", err)
		return false, fmt.Errorf("local verification error: %w", err)
	}

	if !isValid {
		s.log.Warn("inference proof is invalid or revoked", "proof_id", proofID)
		return false, nil
	}

	// If the proof was anchored on-chain, additional verification could involve
	// querying the Casper blockchain for the deploy hash and its status.
	// This example focuses on local verification.
	if p.Deploy != "" {
		s.log.Debug("proof has deploy hash, on-chain verification would be performed here", "proof_id", proofID, "deploy_hash", p.Deploy)
		// Example: Call a Casper RPC method to check deploy status or contract state
		// Local Merkle verification is authoritative; on-chain state is an immutable anchor.
	}

	s.log.Info("inference proof verified successfully", "proof_id", proofID, "valid", isValid)

	// Emit a CEP-88 event for proof verification
	_ = s.emitCEP88Event("ProofVerified", map[string]interface{}{
		"proof_id":  proofID,
		"is_valid":  isValid,
		"timestamp": time.Now().Unix(),
	})

	return isValid, nil
}

// RegisterModel registers an AI model with its hash and a Casper verifier contract address.
// This information is stored in the database and optionally submitted to the blockchain.
func (s *InferenceService) RegisterModel(
	ctx context.Context,
	modelID string,
	modelHash string,
	verifierContract string,
	metadata map[string]string,
) (*ModelRegistryEntry, error) {
	s.log.Info("registering model", "model_id", modelID, "model_hash", modelHash)

	if modelID == "" || modelHash == "" || verifierContract == "" {
		return nil, errors.New("modelID, modelHash, and verifierContract cannot be empty")
	}

	entry := &ModelRegistryEntry{
		ModelID:          modelID,
		ModelHash:        modelHash,
		VerifierContract: verifierContract,
		Metadata:         metadata,
		RegisteredAt:     time.Now().Unix(),
	}

	// Store in database
	if s.db != nil {
		if err := s.db.SaveModelRegistryEntry(ctx, toStoreEntry(entry)); err != nil {
			s.log.Error("failed to save model registry entry to database", "model_id", modelID, "error", err)
			return nil, fmt.Errorf("failed to save model registry entry: %w", err)
		}
	}

	// Optionally, submit model registration to Casper blockchain
	if s.sub != nil {
		s.log.Info("submitting model registration to Casper", "model_id", modelID)
		deployHash, err := s.sub.SubmitModelRegistration(modelID, modelHash, verifierContract, metadata)
		if err != nil {
			s.log.Error("failed to submit model registration to Casper", "model_id", modelID, "error", err)
			// Continue without deploy hash
		} else {
			entry.DeployHash = deployHash
			s.log.Info("model registration submitted to Casper", "model_id", modelID, "deploy_hash", deployHash)
			// Update the entry in DB with deploy hash
			if s.db != nil {
				_ = s.db.SaveModelRegistryEntry(ctx, toStoreEntry(entry)) // Update with deploy hash
			}
		}
	}

	// Emit a CEP-88 event for model registration
	_ = s.emitCEP88Event("ModelRegistered", map[string]interface{}{
		"model_id":          entry.ModelID,
		"model_hash":        entry.ModelHash,
		"verifier_contract": entry.VerifierContract,
		"metadata":          entry.Metadata,
		"registered_at":     entry.RegisteredAt,
		"deploy_hash":       entry.DeployHash,
	})

	s.log.Info("model registered successfully", "model_id", modelID)
	return entry, nil
}

// GetModelInfo retrieves a registered model's information by its ID.
func (s *InferenceService) GetModelInfo(ctx context.Context, modelID string) (*ModelRegistryEntry, error) {
	s.log.Debug("retrieving model info", "model_id", modelID)

	if s.db == nil {
		return nil, errors.New("database not configured for model registry")
	}

	stored, err := s.db.GetModelRegistryEntry(ctx, modelID)
	if err != nil {
		s.log.Error("failed to retrieve model info from database", "model_id", modelID, "error", err)
		return nil, fmt.Errorf("model %s not found: %w", modelID, err)
	}

	s.log.Debug("model info retrieved", "model_id", modelID)
	return fromStoreEntry(stored), nil
}

// toStoreEntry/fromStoreEntry convert between the API-facing ModelRegistryEntry
// and the persistence-layer store.ModelRegistryEntry (kept as distinct types so
// package store has no import back on package inference).
func toStoreEntry(e *ModelRegistryEntry) *store.ModelRegistryEntry {
	return &store.ModelRegistryEntry{
		ModelID:          e.ModelID,
		ModelHash:        e.ModelHash,
		VerifierContract: e.VerifierContract,
		Metadata:         e.Metadata,
		RegisteredAt:     e.RegisteredAt,
		DeployHash:       e.DeployHash,
	}
}

func fromStoreEntry(e *store.ModelRegistryEntry) *ModelRegistryEntry {
	return &ModelRegistryEntry{
		ModelID:          e.ModelID,
		ModelHash:        e.ModelHash,
		VerifierContract: e.VerifierContract,
		Metadata:         e.Metadata,
		RegisteredAt:     e.RegisteredAt,
		DeployHash:       e.DeployHash,
	}
}

// emitCEP88Event is a helper to simulate emitting a CEP-88 compliant event.
// In a real Casper environment, this would interact with a smart contract
// or a specific event emission mechanism. For this backend, it's a log entry.
func (s *InferenceService) emitCEP88Event(eventType string, data map[string]interface{}) error {
	eventData, err := json.Marshal(data)
	if err != nil {
		s.log.Error("failed to marshal CEP-88 event data", "event_type", eventType, "error", err)
		return fmt.Errorf("failed to marshal event data: %w", err)
	}
	s.log.Info("CEP-88 Event Emitted", "event_type", eventType, "data", string(eventData))
	return nil
}
