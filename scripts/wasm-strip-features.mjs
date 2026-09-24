#!/usr/bin/env node
// Post-process a Wasm binary to remove custom `target_features` and the
// `DataCount` (section id 12) preamble that Casper Condor 2.x's WASM
// pre-processor rejects. This does NOT change any function bytecode; it
// only strips two sections that some rustc versions emit even when the
// underlying opcodes do not use bulk-memory/reference-types/etc.
//
// Usage: node scripts/wasm-strip-features.mjs <in.wasm> <out.wasm>
//
// Refs:
//   * https://webassembly.github.io/spec/core/binary/modules.html#binary-datacountsec
//   * "target_features" is a Custom section documented in the LLVM lld source.
//
// This is a small binary rewrite so we can audit it easily.

import fs from "node:fs";

if (process.argv.length !== 4) {
  console.error("usage: wasm-strip-features.mjs <in.wasm> <out.wasm>");
  process.exit(2);
}
const [, , inPath, outPath] = process.argv;
const buf = fs.readFileSync(inPath);
if (buf.readUInt32LE(0) !== 0x6d736100) {
  console.error("not a wasm file: " + inPath);
  process.exit(1);
}
if (buf.readUInt32LE(4) !== 1) {
  console.error("unexpected wasm version");
  process.exit(1);
}

function readULEB128(offset) {
  let result = 0, shift = 0, byte;
  do {
    byte = buf[offset++];
    result |= (byte & 0x7f) << shift;
    shift += 7;
  } while (byte & 0x80);
  return { value: result, next: offset };
}

const chunks = [buf.slice(0, 8)];
let off = 8;
let stripped = [];
while (off < buf.length) {
  const secId = buf[off];
  const sizeRes = readULEB128(off + 1);
  const secSize = sizeRes.value;
  const bodyStart = sizeRes.next;
  const bodyEnd = bodyStart + secSize;

  let keep = true;
  let sectionName = "";
  if (secId === 12) {
    // DataCount section — strip.
    keep = false;
    sectionName = "DataCount";
  } else if (secId === 0) {
    // Custom section — check its name.
    const nameLenRes = readULEB128(bodyStart);
    const nameStart = nameLenRes.next;
    const nameEnd = nameStart + nameLenRes.value;
    const name = buf.slice(nameStart, nameEnd).toString("utf8");
    sectionName = "Custom:" + name;
    if (name === "target_features") {
      keep = false;
    }
  }

  if (keep) {
    chunks.push(buf.slice(off, bodyEnd));
  } else {
    stripped.push({ id: secId, name: sectionName, size: bodyEnd - off });
  }
  off = bodyEnd;
}

const out = Buffer.concat(chunks);
fs.writeFileSync(outPath, out);
console.log(`wrote ${outPath}  ${out.length} bytes (was ${buf.length})`);
for (const s of stripped) {
  console.log(`  stripped section id=${s.id} name=${s.name} size=${s.size}`);
}
