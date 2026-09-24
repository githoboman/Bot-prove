# Merkle Recursion — Intermediate Aggregation Scheme

**Status:** SHIPPED as `merkle-recursion-v1`. Pack AS, backlog #2.12.

This is an honest intermediate between the previous `hash-fold-v1`
(linear O(n) replay) and true STARK recursion (Winterfell,
arkworks-stark — both Rust-only today). It gives verifiers O(log n)
membership check on individual proof commitments, without pretending
to be a real recursive STARK.

**This is NOT STARK recursion.** It does not produce a single proof
whose validity implies the validity of every underlying proof. It
reduces the verifier's *reference* work from O(n) to O(log n), but
each membership check is against a commitment hash — not a
re-execution of the underlying computation.

## Construction

Given `k` opaque commitment digests `d_1..d_k` (the caller decides
what each digest binds — typically SHA-256 of a STARK proof's
public output):

1. Leaf tag: `L_i = SHA256(0x00 || d_i)`. The 0x00 tag isolates
   the leaf-hash domain from the interior-node domain, defeating
   second pre-image attacks that swap a leaf with an interior node.
2. Interior nodes: `N = SHA256(0x01 || left || right)` — same 0x01
   domain-separation tag.
3. Odd-count padding: at every level with odd width, the last node
   is duplicated (Bitcoin-style). This keeps the height predictable
   at the documented cost that a same-value adjacent leaf can be
   inserted without changing the root. Callers who need
   uniqueness-under-insertion MUST fold the leaf's position into
   its digest before submission.
4. Root: the top-level 32-byte hash, hex-serialised.

The aggregate is small — `(scheme, count, tree_height, root_hex)` —
independent of `k` in size.

## Verification

The verifier is given `(aggregate, inclusion_proof)`:

```go
inclusion_proof.leaf_hex       // the leaf-tagged hash of one leaf
inclusion_proof.path_hex[]     // O(log n) sibling hashes bottom-up
inclusion_proof.positions[]    // for each level: is the current node a right child?
```

The verifier re-hashes the path in O(log n) SHA-256 operations and
compares the reconstructed root to `aggregate.merkle_root_hex`.
Success ⇒ this leaf is in the aggregate at `leaf_index`.

## What IS real cryptography here

- **Second pre-image resistance** of the whole tree under SHA-256.
  Domain-separation tags (0x00 leaf, 0x01 interior) prevent
  leaf/interior collisions.
- **O(log n) verifier work.** A cold consumer with only the
  aggregate root can check membership of any single proof in
  `⌈log₂ k⌉` hashes.
- **Determinism.** Two aggregators fed the same leaves in the same
  order produce the same root. Ordering matters (verified in tests).
- **Compact commitment.** Independent of k, the aggregate is
  32 + 4 + 4 = 40 bytes on the wire (before hex).

## What is NOT here

- **Not STARK recursion.** Membership does not verify the
  underlying proof — it only verifies the *commitment* to the
  proof. A caller that wants "this STARK proof is valid AND is in
  the aggregate" must call the STARK verifier separately for the
  leaf hash's pre-image.
- **No support for streaming aggregation across independent
  batches.** Merging two roots into a bigger root is straightforward
  (concatenate the two roots as interior siblings) but is not
  exposed as a public API — batches remain flat by design so the
  wire form is minimal.
- **Not resistant to insertion of identical-value adjacent leaves.**
  The Bitcoin-style padding lets an attacker insert a duplicate of
  the last odd leaf without changing the root. Documented above;
  fix is to fold `leaf_index` into every submitted digest at the
  caller.

## Wire format

**Aggregate:**

```json
{
  "scheme": "merkle-recursion-v1",
  "count": 8,
  "tree_height": 3,
  "merkle_root_hex": "3f2e…c1"
}
```

**Inclusion proof:**

```json
{
  "leaf_index": 3,
  "leaf_hex": "b7c…",                 // 32 hex-encoded bytes
  "path_hex": ["…", "…", "…"],       // one 32-byte hash per level
  "positions": [true, false, true]    // matching bit for each level
}
```

## HTTP surface

Endpoints are public — no env gate — because the scheme label makes
the honest contract explicit on every response.

- `POST /v1/aggregation/merkle-aggregate` — leaves in, aggregate out.
- `POST /v1/aggregation/merkle-inclusion` — leaves + leaf_index in,
  inclusion proof out. Note the endpoint requires the full leaf list
  today; a Postgres-backed leaf store is a follow-up.
- `POST /v1/aggregation/merkle-verify` — aggregate + inclusion proof
  in, `{valid, scheme}` out.

## Comparison with existing folding schemes

|                             | hash-fold-v1  | pedersen-fold-v1        | merkle-recursion-v1 |
| --------------------------- | ------------- | ----------------------- | ------------------- |
| Real crypto hardness        | pre-image     | DLP on BLS12-381 G1     | 2nd pre-image       |
| Homomorphic across splits   | ✗             | ✓                       | ✗                   |
| Verifier work per member    | O(n) replay   | O(n) recompute          | **O(log n)**        |
| Aggregate size              | O(n)          | O(n) (step hashes)      | O(1)                |
| Real STARK recursion        | ✗             | ✗                       | ✗ (labelled)        |

## Testing

- `internal/aggregator/merkle_recursion_test.go` — 11 tests: empty
  rejection, power-of-two aggregate shape, odd-count padding,
  single-leaf edge, inclusion happy-path across every index,
  tampered-leaf rejection, tampered-path rejection, tampered-position
  rejection, scheme mismatch, determinism, order sensitivity,
  out-of-range index.
- `internal/api/merkle_recursion_test.go` — 5 handler tests: aggregate
  happy-path, inclusion + verify round trip, tampered-root rejection,
  inclusion out-of-range 400, bad-hex 400.

## Migration path to real STARK recursion

1. When a Go-native STARK implementation with recursion support
   ships (Winterfell exposes Rust FFI today; a Go binding is
   possible), add a `StarkFolder` implementing the same
   `MerkleRecursionProof` shape but where the aggregate is a
   *real* recursive STARK proof rather than a Merkle root.
2. Emit scheme label `stark-recursion-v1`.
3. The `merkle-recursion-v1` aggregates remain verifiable and
   discoverable — the label in every witness lets a multi-scheme
   consumer route correctly.
