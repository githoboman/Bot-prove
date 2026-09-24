# CasperProver — VIDEO SCRIPT v2 (2026-07-27 update)

**Format:** Faceless tutorial | ~2 min | English
**Style:** Terminal-first, ZK-proof-first (old v1 opened on KYC; the site's own
hero now leads with "Your AI made a decision. Prove it." — matching that)
**No voiceover** — subtitles + tooltips + background music

Old script (deleted from repo history, last version `62b6cce`) matched a UI
that no longer exists (Generate/Proofs/Verify/Demo/Overview tabs). Current
site is an 11-tab Lab grouped into 4 categories: Core workflow (Overview,
Proofs, Models, Aggregation), Cryptography & chain (ZK Proofs, PQ Crypto,
Contracts), Developer tools (Playground), Explore (KYC, Attack Evidence,
Offline Verify). Nav is now a two-level category/tab menu (2026-07-27),
not a flat tab row — worth a quick establishing shot of that in the hook
or Section 1, since a flat-tab shot would look out of date. Rewritten to
match; Attack Evidence and Offline Verify aren't covered by name below —
add a beat for them if the runtime allows, they're live features with no
script coverage yet.

---

## HOOK (first 5 seconds)

[SHOW: casperprover.xyz hero — red "geni" figure, terminal: `casper-prover verify --model gpt-4o --input "loan_42" --anchor testnet`]
[SUBTITLE: "Your AI made a decision. Now prove it — cryptographically, on-chain."]

---

## SECTION 1 — Generate a real proof (0:05–0:30)

[SHOW: Proof Lab → Proofs tab → Input Parameters panel]
[SHOW: Fill Agent ID = `agent-alpha`, Input = `loan_approval_decision`, Model = `gpt-4o`, Output = `approved_with_conditions`]
[SHOW: Click "Generate Proof" → proof-output terminal fills: hash, Merkle root, leaf index]

[SUBTITLE: "SHA-256 + Merkle tree. Change one bit of input or output — the proof breaks."]

[SHOW: Contracts tab → click `proof_registry` → testnet.cspr.live opens on the real deploy]

[TOOLTIP: "9 contracts, all live on Casper testnet — not a mockup."]

---

## SECTION 2 — Real ZK proofs, not simulation (0:30–0:55)

[SHOW: ZK Proofs tab. Prove preimage `42` with Groth16]
[SUBTITLE: "Real BN254 pairing cryptography via gnark — full prove/verify cycle, not a stub."]
[SHOW: Verify → green "Valid" badge]
[SHOW: Flip one byte of `proof_hex` → Verify again → red "Invalid"]
[SUBTITLE: "Negative test: tampering is detected every time."]

[TOOLTIP: "Off-chain compute, on-chain anchor — 200ms prove, <5ms verify."]

---

## SECTION 3 — Post-quantum + stake economics (0:55–1:15)

[SHOW: PQ Crypto tab. Hybrid-sign a message → Ed25519 + ML-DSA-65 both valid]
[SUBTITLE: "Post-quantum ready: ML-DSA-65 (FIPS 204) + classical hybrid signing."]

[SHOW: Overview/Contracts → stake-slashing contract]
[SUBTITLE: "Dishonest agents lose 20% of stake — reporters get a permissionless bounty."]

---

## SECTION 4 — Aggregation, models, agent demo (1:15–1:40)

[SHOW: Aggregation tab — batch multiple proofs into one hash-chained anchor]
[SUBTITLE: "Batch proofs together — one on-chain write, many attestations."]

[SHOW: Models tab — model provenance registry entry]

[SHOW: Agent Demo tab — 6-step pipeline end to end: decision → hash → proof → anchor → verify → done]
[SUBTITLE: "One button, the full pipeline, on real testnet."]

---

## SECTION 5 — API Playground + judge path (1:40–1:55)

[SHOW: API Playground — pick an endpoint, hit run, see real request/response]
[SUBTITLE: "32 REST endpoints · 32 SDK methods · MCP server for any AI agent."]

[SHOW: Terminal: `python3 scripts/judge_demo.py` → 9/9 contracts PASS, API health PASS, frontend PASS]
[SUBTITLE: "One command verifies everything above — no trust required."]

---

## OUTRO (1:55–2:00)

[SHOW: casperprover.xyz hero, stats bar: `9 contracts · 250+ testnet txns · 32 endpoints · 32 SDK/MCP tools`]
[SUBTITLE: "CasperProver. AI accountability, on-chain."]
[B-ROLL: github.com/anna-stolbovskaja/CasperProver]

---

## TOOLTIP LIBRARY

| Context | Text |
|---------|------|
| Deploy hash appears | `Live on Casper testnet` |
| ZK verify pass | `Real Groth16, gnark BN254 — not simulated` |
| ZK verify fail (tamper test) | `Tamper detected — proof invalidated` |
| PQ sign | `ML-DSA-65 (FIPS 204) hybrid signing` |
| Stake-slash | `20% slash · permissionless bounty for reporters` |
| Aggregation | `Hash-chain batching, Postgres-persisted` |
| judge_demo run | `9/9 contracts verified — one command` |

## PRODUCTION NOTES (carried over from v1, still valid)

- 1920×1080, Chrome dark mode, URL bar visible (proves live site).
- Screen recording 1.5–2× speed; only slow down for the ZK tamper-test moment (needs to read clearly).
- Subtitles only, no voiceover, background music same energy class as AE402's (upbeat electronic, no vocals).
