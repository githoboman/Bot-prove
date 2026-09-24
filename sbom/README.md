# Software Bill of Materials (SBOM)

CycloneDX SBOMs for every dependency graph in this repo, generated for the T14 hackathon deliverable.

| File | Ecosystem | Source |
|---|---|---|
| `engine-go-sbom.json` | Go | `engine/` module, via `cyclonedx-gomod mod` |
| `sdk-go-sbom.json` | Go | `sdk/` module, via `cyclonedx-gomod mod` |
| `frontend-sbom.json` | Node.js | `frontend/` (judge dashboard + lab UI), via `@cyclonedx/cyclonedx-npm` |
| `rust-contracts/*.cdx.json` | Rust | one CycloneDX doc per contract crate in `contracts/`, via `cargo cyclonedx --all` |

(No Python dependency manifest exists in this repo, so no Python SBOM is generated.)

## Regenerating

```bash
# Go
go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
cd engine && cyclonedx-gomod mod -json -output ../sbom/engine-go-sbom.json
cd ../sdk && cyclonedx-gomod mod -json -output ../sbom/sdk-go-sbom.json

# Node (frontend)
cd frontend && npx @cyclonedx/cyclonedx-npm --ignore-npm-errors --output-file ../sbom/frontend-sbom.json

# Rust (all contract crates)
cargo install cargo-cyclonedx
cd contracts && cargo cyclonedx --format json --all
# then move the generated *.cdx.json files from each crate dir into sbom/rust-contracts/
```

All formats use the CycloneDX 1.x JSON schema and can be scanned by any CycloneDX-compatible SCA tool
(Dependency-Track, Grype, OWASP Dependency-Check, Snyk, etc.).
