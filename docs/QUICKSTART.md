# Two-Minute Quickstart

Point three SDKs at the same running API. Copy-paste, expect JSON back.

## 1. Boot the API locally

```bash
cd engine
go run ./cmd/casperprover serve
# server now listens on :9090
```

Optional: `export API_KEY=secret` to require `X-API-Key` on writes.

## 2. Pick an SDK

### Go

```bash
cd sdk
go run ./examples/go/quickstart
```

### Python

```bash
pip install requests
python -m sdk.examples.python.quickstart
```

### TypeScript

```bash
# from repo root
npx tsx sdk/examples/typescript/quickstart.ts
```

## 3. What you should see

Each quickstart runs:

1. `GET  /health`             — sanity check.
2. `POST /proofs`             — submits a demo proof, prints `proof_id`.
3. `GET  /proofs/{id}`        — reads it back.
4. `POST /proofs/{id}/verify` — server-side merkle re-check.

Success = four JSON responses printed, `quickstart OK.` on stdout, exit code `0`.

## Common overrides

```
CP_API_URL   base URL      (default http://localhost:9090)
CP_API_KEY   X-API-Key     (optional; matches server API_KEY)
```

## Next

- `docs/JUDGE_GUIDE.md` — the 8-criteria hackathon walkthrough.
- `docs/ARCHITECTURE.md` + `docs/ARCHITECTURE_EXTENSIONS.md` — component map, trust boundaries.
- `docs/API_CHANGELOG.md` — API version history, deprecation timeline.
