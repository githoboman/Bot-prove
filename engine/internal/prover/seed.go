package prover

import "github.com/anna-stolbovskaja/CasperProver/engine/internal/config"

// SeedDemoData populates the engine with realistic demo proofs so that
// the lab always has meaningful data even after a restart.
func (e *ProofEngine) SeedDemoData() {
	// Look up proof-registry hash from the canonical on-chain manifest.
	// Falls back to empty string if the manifest is not readable — an empty
	// Deploy is a signal to the UI that the seed row is not chain-anchored,
	// which is preferable to a hardcoded stale hash surviving a redeploy.
	proofRegistryHash, _ := config.ContractHash("proof_registry")
	demos := []struct {
		agent   string
		input   string
		output  string
		model   string
		useCase string
		pubKey  string
	}{
		{
			agent:   "kyc-verifier-v2",
			input:   `{"user_id":"alice_0x3f","doc_type":"passport","country":"DE","issued":"2022-03-15"}`,
			output:  `{"verified":true,"confidence":0.97,"risk_score":12,"flags":[]}`,
			model:   "kyc-model-v2.1",
			useCase: "kyc",
			pubKey:  "020260dd84fc2f98a96e6a62ad499e0bcf21e7edf0eb1b48ee0fba6fda0d9478af4c",
		},
		{
			agent:   "loan-decisioning-bot",
			input:   `{"applicant":"bob_0x7a","income":85000,"debt_ratio":0.23,"credit_score":742,"requested":25000}`,
			output:  `{"decision":"approved","limit":25000,"rate":5.4,"term_months":36,"conditions":["employment_verification"]}`,
			model:   "lending-llm-v3.0",
			useCase: "loan",
			pubKey:  "020260dd84fc2f98a96e6a62ad499e0bcf21e7edf0eb1b48ee0fba6fda0d9478af4c",
		},
		{
			agent:   "insurance-claims-ai",
			input:   `{"claim_id":"CLM-2847","type":"auto","damage_photos":3,"police_report":true,"amount_claimed":12500}`,
			output:  `{"assessment":"valid","approved_amount":11800,"deductible":500,"payout":11300,"fraud_probability":0.02}`,
			model:   "claims-assessor-v1.4",
			useCase: "insurance",
			pubKey:  "01a3f5b4c2d1e6f7089a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f",
		},
		{
			agent:   "content-moderator",
			input:   `{"post_id":"p_9823","text":"Check out this amazing product...","media_count":2,"user_flags":0}`,
			output:  `{"action":"approve","toxicity":0.03,"spam_score":0.12,"category":"commerce","review_needed":false}`,
			model:   "moderation-v2.0",
			useCase: "content_moderation",
			pubKey:  "01a3f5b4c2d1e6f7089a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f",
		},
		{
			agent:   "medical-diagnosis-ai",
			input:   `{"patient_hash":"0xf3c2","symptoms":["chest_pain","shortness_of_breath"],"vitals":{"bp":"140/90","hr":92}}`,
			output:  `{"preliminary":"cardiac_evaluation_needed","urgency":"high","confidence":0.89,"referral":"cardiology"}`,
			model:   "medscreen-v4.1",
			useCase: "medical_screening",
			pubKey:  "020260dd84fc2f98a96e6a62ad499e0bcf21e7edf0eb1b48ee0fba6fda0d9478af4c",
		},
		{
			agent:   "kyc-verifier-v2",
			input:   `{"user_id":"charlie_0x1b","doc_type":"drivers_license","country":"US","issued":"2023-09-01"}`,
			output:  `{"verified":true,"confidence":0.94,"risk_score":18,"flags":["address_mismatch"]}`,
			model:   "kyc-model-v2.1",
			useCase: "kyc",
			pubKey:  "01a3f5b4c2d1e6f7089a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f",
		},
		{
			agent:   "loan-decisioning-bot",
			input:   `{"applicant":"dave_0x5c","income":42000,"debt_ratio":0.51,"credit_score":580,"requested":15000}`,
			output:  `{"decision":"denied","reason":"debt_to_income_ratio_exceeded","max_eligible":5000,"suggestions":["reduce_existing_debt"]}`,
			model:   "lending-llm-v3.0",
			useCase: "loan",
			pubKey:  "020260dd84fc2f98a96e6a62ad499e0bcf21e7edf0eb1b48ee0fba6fda0d9478af4c",
		},
		{
			agent:   "aml-scanner",
			input:   `{"tx_hash":"0xabc123","from":"0x742d","to":"0x9f3e","amount_usd":48500,"chain":"ethereum"}`,
			output:  `{"risk":"low","sanctions_match":false,"pep_match":false,"score":8,"cleared":true}`,
			model:   "aml-graph-v2.3",
			useCase: "aml_screening",
			pubKey:  "01a3f5b4c2d1e6f7089a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f",
		},
	}

	for _, d := range demos {
		p := e.GenerateWithKey(d.agent, d.pubKey, []byte(d.input), []byte(d.output), []byte(d.model), d.useCase, "anchored")
		p.Deploy = proofRegistryHash
	}
}
