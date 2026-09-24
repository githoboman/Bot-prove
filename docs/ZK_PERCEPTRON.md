# PerceptronCircuit — a real neural network encoded as R1CS

The engine's ZK pipeline used to expose only `MiMCPreimageCircuit`
(preimage-of-a-hash) and `ModelInferenceCircuit` (a MiMC commitment
scaffold). Neither ran any actual inference arithmetic in-circuit —
they merely proved knowledge of an input that hashed to a claimed
output.

`PerceptronCircuit` (`perceptron_linear_v1`) is a step forward: a
genuine single-perceptron classifier encoded end-to-end in gnark
R1CS constraints. The proof certifies that the caller ran the
committed linear model on their (private) input and got the claimed
public output — not merely that they knew a preimage.

Package: [`engine/internal/zkverifier/gnarkzk/perceptron_circuit.go`](../engine/internal/zkverifier/gnarkzk/perceptron_circuit.go).

## Model shape

The circuit models one perceptron unit:

```
output = 1  if <weights, input> + bias >= 0
         0  otherwise
```

- `PerceptronInputDim = 8` (fixed at compile time — small enough to
  keep constraints under ~500 for fast prove/verify).
- Weights and inputs are signed 16-bit integers lifted into the
  BN254 scalar field (negatives represented as `p - |x|`).
- The threshold is a single comparator emitting a boolean output.

## Public / private wires

| wire | visibility | role |
|---|---|---|
| `Input[8]` | private | The N-dim vector the classifier was run on. |
| `Bias` | private | Model bias term. |
| `Weights[8]` | public | Model weights (transparent model — see below). |
| `WeightsCommit` | public | `MiMC(w_0 ‖ w_1 ‖ … ‖ w_7 ‖ b)`, bound to the actual assignment inside the circuit. |
| `Output` | public | Booleanised classifier decision. |

The commitment is recomputed by the gadget: the circuit hashes the
assigned `Weights` and `Bias` with MiMC and asserts equality against
the public `WeightsCommit`. Tampering with either component changes
the recomputed hash and fails the proof.

## Honest scope limitations

- **Weights are public.** This is a modelling choice. Hiding weights
  behind a commitment while proving `<w,x>` in-circuit would roughly
  double the constraint count and require the caller to trust the
  commit scheme end-to-end. We chose the transparent-model path for
  this variant; a `PerceptronPrivateWeightsCircuit` variant may
  follow.
- **Linear model only.** No ReLU, sigmoid, softmax, or any other
  non-linearity. Adding those inside R1CS is a matter of bookkeeping
  (range checks + booleanise gadgets exist in gnark's std/) but they
  multiply the constraint count. Out of scope for this circuit.
- **Single unit, single layer.** Multi-layer networks compose but
  each layer roughly linearly multiplies the constraint count.
  Real production networks (thousands of neurons per layer) are
  not yet within reach of on-CPU Groth16 setup / prove. This is a
  demonstrator for the *shape* of a real inference proof, not a
  path to serving Llama through R1CS.

Backlog item **2.7 (remaining)** — "Full model-inference ZK circuit
for a real model" — is *partially* closed by this circuit: we have a
genuine, real inference in R1CS, but the network itself is a linear
classifier not a full deep net. The next step (multi-layer, ReLU
in-circuit) remains open, and I've kept the roadmap entry as
"partial" honestly.

## Comparator gadget — why the 40-bit shift

The BN254 scalar field has no sign bit. To compare `dot ⋛ 0` we
shift `dot` by a large positive constant that is guaranteed to make
every legal input non-negative, then compare against that same
constant. `PerceptronInputDim = 8`, weights/inputs bounded by ±2^15,
so `|dot| ≤ 8 · 2^30 + 2^15 < 2^33`. Shifting by `2^39` (safe by a
margin of 6 bits) keeps the shifted value in `[0, 2^40)`, which
gnark's `cmp.NewBoundedComparator` handles cheaply.

## Ceremony reuse

Same backend as the rest of the `/v1/zk/*` pipeline — Groth16 over
BN254. The registry generates ccs / pk / vk for this circuit
alongside the existing ones under `CP_ZK_KEYS_DIR`; the same
trusted-setup ceremony that covers `MiMCPreimageCircuit` and
`ModelInferenceCircuit` covers this one.

## Testing

- `perceptron_circuit_test.go` — 8 test cases:
  - classify positive class (dot ≥ 0 → output = 1)
  - classify negative class (dot < 0 → output = 0)
  - boundary (dot = 0 → output = 1, since threshold is ≥)
  - Prove REJECTS a caller who lies about output
  - Prove REJECTS a caller who provides a wrong `weights_commit`
  - `AssignFull` derives `weights_commit` and `output` automatically
  - `Descriptor()` reports the right public-input count and metadata
  - `canonicalScalarBytes(-5)` correctly lifts negatives into the field

The first three cases run the full **compile → setup → prove →
verify** roundtrip (about 1.5s each on stock CI hardware). The
"rejects" cases stop at `Prove`, which is where the malicious
constraint violation is caught by gnark.

## Endpoints

The circuit is registered under the id `perceptron_linear_v1` on
server startup. Callers use:

```
POST /v1/zk/prove       — generate a proof (needs full assignment)
POST /v1/zk/verify      — verify (needs public inputs only)
GET  /v1/circuits/perceptron_linear_v1        — descriptor
GET  /v1/circuits/perceptron_linear_v1/vk     — verifying key digest
```

## Next steps (backlog followups)

- **2.7 (remaining)** Full multi-layer model in R1CS with ReLU
  activations. Order of magnitude larger constraint count; needs a
  larger Groth16 setup (or Plonk, if we want circuit-agnostic
  universal setup).
- Optional: `PerceptronPrivateWeightsCircuit` variant that keeps
  weights private under a Pedersen commitment. Constraint count
  roughly doubles.
- Optional: witness-generation helper that ingests a trained
  scikit-learn linear classifier and emits the assignment map
  directly, so the caller doesn't hand-encode weights.
