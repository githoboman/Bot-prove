package main

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/anna-stolbovskaja/CasperProver/engine/internal/api"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/judge/hitl"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/kyc"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/llm"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/prover"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/store"
	"github.com/anna-stolbovskaja/CasperProver/engine/internal/verifier"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	eng := prover.New()

	switch os.Args[1] {
	case "prove":
		demoProve(eng)
	case "verify":
		demoVerify(eng)
	case "demo":
		demoFlow(eng)
	case "gate3":
		runGate3Demo()
	case "serve":
		serve(eng)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: casperprover <command>\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  prove   Generate a demo proof\n")
	fmt.Fprintf(os.Stderr, "  verify  Verify a demo proof\n")
	fmt.Fprintf(os.Stderr, "  demo    Run full KYC demo flow\n")
	fmt.Fprintf(os.Stderr, "  gate3   Run the Gate 3 agentic vertical slice: approve + abstain + malicious + conflict paths, no network I/O\n")
	fmt.Fprintf(os.Stderr, "  serve   Start API server\n")
}

func demoProve(eng *prover.ProofEngine) {
	input := []byte(`{"user":"bob","doc":"passport"}`)
	output := []byte(`{"verified":true}`)
	model := []byte("model-v1")

	p := eng.Generate("demo-agent", input, output, model, "kyc")
	slog.Info("proof generated", "id", p.ID, "hash", p.PH, "root", p.Root)
}

func demoVerify(eng *prover.ProofEngine) {
	input := []byte(`{"user":"bob","doc":"passport"}`)
	output := []byte(`{"verified":true}`)
	model := []byte("model-v1")

	p := eng.Generate("demo-agent", input, output, model, "kyc")
	v := verifier.New()
	err := v.VerifyProof(p, input, output, model)
	if err != nil {
		slog.Error("verification failed", "error", err, "proof_id", p.ID)
		os.Exit(1)
	}
	slog.Info("verification passed", "proof_id", p.ID)
}

func demoFlow(eng *prover.ProofEngine) {
	flow := kyc.NewDeFiFlow(eng)
	if err := flow.RunDemo("demo-agent"); err != nil {
		slog.Error("demo flow failed", "error", err)
		os.Exit(1)
	}
}

func serve(eng *prover.ProofEngine) {
	// Item 7.1 — CP_STRICT=1 requires all critical env to be set.
	if err := api.Preflight(nil); err != nil {
		slog.Error("strict preflight failed", "error", err)
		os.Exit(2)
	}

	// write deployer key from env to temp file if provided
	if keyB64 := os.Getenv("DEPLOYER_KEY_B64"); keyB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(keyB64)
		if err == nil {
			// Use secure random temp file instead of predictable /tmp/deployer.pem
			tmpFile, err := os.CreateTemp("", "deployer-*.pem")
			if err == nil {
				keyPath := tmpFile.Name()
				if _, err := tmpFile.Write(decoded); err == nil {
					_ = tmpFile.Close()
					if chmodErr := os.Chmod(keyPath, 0600); chmodErr != nil {
						slog.Warn("failed to restrict deployer key file permissions", "path", keyPath, "error", chmodErr)
					}
					_ = os.Setenv("DEPLOYER_KEY_PATH", keyPath)
					slog.Info("deployer key written from env", "path", keyPath)
					// Schedule cleanup on exit
					defer func() { _ = os.Remove(keyPath) }()
				} else {
					_ = tmpFile.Close()
					_ = os.Remove(keyPath)
				}
			}
		}
	}

	// try connecting to PostgreSQL
	var db *store.PG
	pg, err := store.Open()
	if err != nil {
		slog.Warn("postgres unavailable, using in-memory only", "err", err)
	} else if pg != nil {
		db = pg
		defer db.Close()
		loaded, err := db.Load(eng)
		if err != nil {
			slog.Warn("failed to load proofs from db", "err", err)
		} else {
			slog.Info("loaded proofs from postgres", "count", loaded)
		}
	}

	// seed demo data only if engine is empty
	if len(eng.List()) == 0 {
		eng.SeedDemoData()
		slog.Info("seeded demo data", "count", len(eng.List()))

		// persist seeds to db
		if db != nil {
			for _, p := range eng.List() {
				_ = db.Save(p)
			}
			slog.Info("persisted seed data to postgres")
		}
	} else {
		slog.Info("skipped seeding, engine has data", "count", len(eng.List()))
	}

	port := 8080
	if v := os.Getenv("API_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	srv, err := api.New(eng, port, db)
	if err != nil {
		// api.New returns an error on strict-mode precondition
		// violations (e.g. CP_STRICT=1 with an empty API_KEY). Crash
		// loudly instead of starting a broken server. See
		// api.New()'s docstring for the current list of preconditions.
		slog.Error("api.New failed -- refusing to start", "error", err)
		os.Exit(1)
	}

	// Wire the multi-provider judge if any LLM keys are configured. Absence is
	// not fatal — the /inference/judge endpoint will just 503 until keys land.
	if providers := llm.BuildProvidersFromEnv(0); len(providers) > 0 {
		cfg := llm.LoadConfig()
		runner := llm.NewRunner(providers, nil, cfg)
		srv.SetJudge(judge.NewFacetJudge(runner))
		slog.Info("judge wired", "providers", len(providers))
	} else {
		slog.Warn("judge NOT wired — no LLM provider keys in env; /inference/judge will 503")
	}

	// Wire HITL sinks from env (HITL_SINKS=slack,telegram,noop). Missing config
	// falls back to NoopSink so the server always boots; misconfigured sinks
	// fail loud at startup.
	hitlCfg := hitl.ConfigFromEnv()
	hitlSink, err := hitlCfg.Build()
	if err != nil {
		slog.Error("HITL sink config invalid", "error", err, "kinds", hitlCfg.Kinds)
		os.Exit(1)
	}
	srv.SetHITLSink(hitlSink)
	slog.Info("HITL sink wired", "kinds", hitlCfg.Kinds)

	if err := srv.Start(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
