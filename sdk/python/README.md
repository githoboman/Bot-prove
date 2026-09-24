# CasperProver Python SDK

Synchronous Python client and `cp` CLI for the CasperProver API.

## Install

```bash
pip install casperprover-sdk
# or from a local checkout:
pip install -e sdk/python
```

## Library

```python
from casperprover_sdk import ProverClient

client = ProverClient("http://localhost:9090")
print(client.health())

proof = client.submit(
    agent_id="agent-1",
    input_data="hello",
    output_data="world",
    model="model-v1",
    use_case="inference",
)
print(proof["id"])

assert client.verify(proof["id"])
```

## CLI

Installed entry points: **`cprover`** (short) and **`casperprover`** (long).
We avoid the name `cp` because it collides with the built-in Unix copy command.

```bash
export CP_BASE_URL=http://localhost:9090   # optional override

cprover health
cprover proofs list
cprover proofs get <proof_id>
cprover proofs verify <proof_id>
cprover proofs submit \
    --agent-id agent-1 \
    --input "hello" --output "world" --model "model-v1" \
    --use-case inference
cprover version
```

Exit codes: `0` success, `1` user error, `2` API/HTTP error (or verify → invalid).

## Full docs

See [`docs/CLI.md`](../../docs/CLI.md) in the repo root for complete reference and demo transcripts.
