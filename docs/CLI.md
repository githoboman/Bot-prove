# CasperProver CLI (`cprover`)

The `cprover` command ships with the [`casperprover-sdk`](../sdk/python) Python package. It gives agents and humans a shell-friendly way to submit and verify CasperProver proofs.

> The entry point is named **`cprover`** (short) and **`casperprover`** (long alias). We do not register `cp` because Unix already ships a `cp` (copy) command.

## Install

```bash
pip install casperprover-sdk
# or from source:
pip install -e sdk/python
```

## Configure

```bash
export CP_BASE_URL=https://api.casperprover.xyz   # or http://localhost:9090 in dev
```

Every command also accepts `--base-url` and `--timeout`.

## Commands

### `cprover health`
```bash
$ cprover health
{
  "status": "ok"
}
```

### `cprover proofs list`
Returns all proofs indexed by the API.

### `cprover proofs get <id>`
Fetches a single proof.

```bash
$ cprover proofs get proof_01H...
{
  "id": "proof_01H...",
  "agent_id": "agent-1",
  "use_case": "inference",
  ...
}
```

### `cprover proofs verify <id>`
Runs the on-node verifier for a given proof id. Exit code `0` means valid, `2` means invalid.

```bash
$ cprover proofs verify proof_01H...
{
  "proof_id": "proof_01H...",
  "valid": true
}
```

### `cprover proofs submit`
Generates a new proof. Inputs can be provided inline or as file paths.

```bash
cprover proofs submit \
    --agent-id agent-1 \
    --input "prompt text" \
    --output "model reply" \
    --model "model-v1" \
    --use-case inference

# from files
cprover proofs submit \
    --agent-id agent-1 \
    --input-file  prompt.txt \
    --output-file reply.txt \
    --model-file  model.bin \
    --use-case inference
```

### `cprover version`
```bash
$ cprover version
{
  "casperprover_sdk": "0.1.0"
}
```

## Exit codes
- `0` — success
- `1` — user/CLI error (e.g. missing arguments)
- `2` — API/HTTP error or `verify` returned `invalid`
- `130` — interrupted (Ctrl-C)

## Programmatic use

The same package exposes the SDK:

```python
from casperprover_sdk import ProverClient

client = ProverClient("http://localhost:9090")
proof = client.submit(
    agent_id="agent-1",
    input_data="hello",
    output_data="world",
    model="model-v1",
    use_case="inference",
)
assert client.verify(proof["id"])
```
