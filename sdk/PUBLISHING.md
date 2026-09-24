# SDK publishing

This document is the source of truth for how the three CasperProver SDKs
ship. There are no paid services in the pipeline — everything runs on
GitHub Actions minutes, npm, PyPI, and the public Go proxy.

## The three components

| SDK        | Package                              | Registry             | Tag prefix    | Manifest of record            |
|------------|--------------------------------------|----------------------|---------------|-------------------------------|
| TypeScript | `@casperprover/sdk`                  | npmjs.com            | `sdk-ts-v*`   | `sdk/typescript/package.json` |
| Python     | `casperprover`                       | pypi.org             | `sdk-py-v*`   | `sdk/python/pyproject.toml`   |
| Go         | `.../CasperProver/sdk`               | proxy.golang.org     | `sdk/v*`      | `sdk/version.txt` (+ go.mod)  |

Every component is independently versioned. A `feat(sdk):` commit that
touches all three bumps all three; a `fix(sdk-py):` touches only Python.

## Two release triggers

Both routes end at the same `sdk-publish-*` workflows and use the same
version-guard.

### 1. release-please (default)

`.github/workflows/sdk-release-please.yml` runs on every push to `main`
and to `post-hackathon/roadmap`. release-please reads the Conventional
Commits history and opens (or updates) one PR per SDK that has unreleased
changes. Merge the PR → release-please cuts the GitHub Release with the
right `sdk-ts-vX.Y.Z` / `sdk-py-vX.Y.Z` / `sdk/vX.Y.Z` tag → the per-
language `sdk-publish-*` workflow fires on that Release event.

Config lives in [`.release-please-config.json`](../.release-please-config.json) and
[`.release-please-manifest.json`](../.release-please-manifest.json). The manifest
tracks the last released version per SDK; do not hand-edit it — release-please
owns it.

### 2. Manual dispatch

Every publish workflow has a `workflow_dispatch` trigger with a
`dry_run` boolean (default `true`) so an operator can do an out-of-band
release without touching the release-please queue. Set `dry_run=false`
to actually publish.

Manual dispatch is the escape hatch. Use it for:

- Emergency patch when the release-please PR is not yet cut.
- Publishing a `next` / `beta` npm dist-tag.
- Uploading a build to TestPyPI (`test_pypi=true`).

## Semver enforcement

`.github/workflows/sdk-semver-check.yml` runs on every PR that touches
`sdk/**` and refuses to merge when:

1. The PR title is not a Conventional Commit
   (`feat|fix|perf|refactor|docs|test|ci|chore|revert|build`, optional
   scope, optional `!`, subject).
2. A commit downgrades any SDK's manifest (e.g. `0.2.0` → `0.1.9`).
3. A commit does a **MAJOR** bump on any SDK without a `!` in the
   commit type or a `BREAKING CHANGE:` footer in the PR body / commit
   log.

Together with release-please, this guarantees that a MAJOR ships only
when the changelog says so.

## Dry-run in CI

Every `sdk-publish-*.yml` file has a `build-and-verify` job that runs on
every push/PR touching `sdk/**`. It does the full publish pipeline
except the final `publish` / `upload` step, so a broken package.json,
missing README, or corrupt wheel fails the PR:

- **npm** — `npm pack --dry-run` + a grep on the tarball to prove
  `dist/index.js` and `dist/index.d.ts` are actually in the archive.
- **PyPI** — `python -m build` + `twine check dist/*` + a `zipfile -l`
  grep on the wheel that (a) `casperprover/client.py` is present and
  (b) `casperprover/tests/**` is NOT bundled into the release wheel.
- **Go** — `go mod tidy -diff`, `go vet`, `go test -race`, plus a check
  that the module path in `go.mod` matches the expected canonical path.

## Publishing credentials

- **npm** — repo secret `NPM_TOKEN`, an *automation token* owned by an
  account that is a maintainer of the `@casperprover` org. The publish
  job is gated on the `npm-publish` GitHub Environment so a reviewer
  approves each release.
- **PyPI** — no long-lived secret. The publish job uses **Trusted
  Publishing** (OIDC): configure a *pending publisher* on
  https://pypi.org/manage/account/publishing/ pointing at this repo, the
  `sdk-publish-pypi.yml` workflow, and the `pypi-publish` environment.
  First release promotes it to a real publisher automatically.
- **Go / pkg.go.dev** — no credentials. A tag on origin *is* the
  release. The publish job simply warms the public proxy so `go get`
  resolves immediately and pings `pkg.go.dev` to trigger doc indexing.

Rotation policy: the npm token is rotated on maintainer change or every
six months, whichever comes first. PyPI needs no rotation.

## Cutting a release by hand

If release-please is stuck and you need to ship right now:

```sh
# 1. Bump manifests. e.g. patch on all three:
jq '.version = "0.1.1"' sdk/typescript/package.json > _t && mv _t sdk/typescript/package.json
sed -i 's/^version = ".*"/version = "0.1.1"/' sdk/python/pyproject.toml
echo "0.1.1" > sdk/version.txt

# 2. Commit + tag each component we're releasing.
git commit -am "release: sdk 0.1.1"
git tag sdk-ts-v0.1.1
git tag sdk-py-v0.1.1
git tag sdk/v0.1.1
git push origin HEAD --tags

# 3. Cut a GitHub Release from each tag (gh release create sdk-ts-v0.1.1 …).
#    The release event fires sdk-publish-npm / sdk-publish-pypi / sdk-publish-go.
```

Then reconcile `.release-please-manifest.json` back to the new versions
so the release-please PR queue starts from the new state.

## Non-goals

- **Signed releases.** Sigstore / Cosign is on the roadmap — for now we
  rely on npm provenance (`--provenance`) and Trusted Publishing on
  PyPI. Go proxy pins sums via `sum.golang.org` automatically.
- **Auto-publish on `main`.** Every release is a merged
  release-please PR or a dispatched workflow — never a silent push.
