# Merkle scheme — engine/internal/prover/merkle.go

Backlog item **2.18 — reference / provenance vectors**.

This document pins down the exact Merkle scheme this codebase implements so
that any external verifier (JS, Rust, another Go module, a Casper contract)
can port it byte-for-byte. It also records the two known deviations from
common Merkle designs so nobody is surprised at audit time.

## Construction

Let `H(x) = SHA-256(x)`. Leaves are raw bytes.

1. Hash every leaf: `l_i = H(x_i)`.
2. If the current row has an odd number of nodes, **duplicate the last
   node** and append it.
3. Concatenate pairs left-to-right and hash: `p = H(l_{2k} || l_{2k+1})`.
   Repeat until one node remains — the root.

Path proofs list sibling hashes bottom-up. `VerifyPath` recombines
`H(current || sibling)` when the current node is a left child
(`idx % 2 == 0`) and `H(sibling || current)` when it is a right child,
walking up until it matches the claimed root.

## KAT — 4-leaf pinned root

Input:

```
leaves = ["alpha", "beta", "gamma", "delta"]
```

Compute:

```
h_a = SHA-256("alpha")
h_b = SHA-256("beta")
h_g = SHA-256("gamma")
h_d = SHA-256("delta")
h_ab = SHA-256(h_a || h_b)
h_gd = SHA-256(h_g || h_d)
root = SHA-256(h_ab || h_gd)
```

`root` is asserted byte-for-byte in
`TestMerkleProvenance_FourLeaves_PinnedRoot`.

## Deviations from RFC 6962

- **No domain-separation bytes.** RFC 6962 hashes leaves as `H(0x00 || leaf)`
  and interior nodes as `H(0x01 || left || right)` to make it impossible to
  present a leaf hash as an interior node. This codebase does **NOT** do
  that. A caller who needs CT-style domain separation must add the prefix
  bytes to the leaf payload before calling `BuildTree`.

- **Odd-count padding duplicates the last node.** RFC 6962 promotes the
  lone node to the next level without pairing it. Our scheme duplicates
  it instead. This is the classic "CVE-2012-2459 shape":

  ```
  Root([x])           == Root([x, x])
  Root([a, b, c])     == Root([a, b, c, c])
  ```

  A prover can rewrite the leaf-count claim without changing the root.
  Callers that anchor on `(root, leaf_count)` are safe; callers that
  anchor only on `root` must be aware of this.

Both KATs (`TestMerkleProvenance_SingleLeaf_DupSelfPadding`,
`TestMerkleProvenance_ThreeLeaves_DupPadding_KAT`) pin these deviations so
a future refactor cannot silently break them.

## Adversarial coverage

The provenance test file also proves the verifier rejects:

- a valid path presented at the **wrong index**,
- a **tampered sibling** in the path,
- a valid path against a **tampered root**,
- a valid path **replayed against a different tree's root**,
- **malformed hex** in the path or wrong-size siblings.

## When to reach for a stronger scheme

- **RFC 6962** — when interoperating with Certificate Transparency, Sigstore
  Rekor, or any external log that expects the CT-style domain-separated
  hashing. Would need a `BuildTreeCT` variant.
- **Sparse Merkle Tree** — when leaves are addressed by key rather than
  ordered by index (state trees, revocation sets).
- **Bulletproofs-style commitment tree** — when path length must be `O(log n)`
  and each proof kept short *and* small. Beyond the current scope.

These are follow-ups; the current scheme is sufficient for the on-chain
anchor + off-chain audit-log workflow it is used for.
