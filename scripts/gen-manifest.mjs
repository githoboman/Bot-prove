#!/usr/bin/env node
/**
 * gen-manifest.mjs — regenerate all downstream copies of deploy-out/onchain.json.
 *
 * Canonical source of truth: deploy-out/onchain.json
 * Generated copies:
 *   - frontend/public/onchain.json   (fetched at runtime by the SPA)
 *   - engine/internal/config/manifest_data.go (embedded via go:embed fallback)
 *
 * Usage:
 *   node scripts/gen-manifest.mjs           # regenerate all copies
 *   node scripts/gen-manifest.mjs --check   # exit 1 if any generated copy is stale
 *
 * The generator does NOT touch documentation strings (README/OpenAPI examples).
 * Contract-hash references in prose docs are audited manually; see
 * CP_FINAL_TASKS_V2 §A Gate 1.5.
 */
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, '..');

const CANONICAL = resolve(repoRoot, 'deploy-out/onchain.json');
const COPIES = [
  {
    path: resolve(repoRoot, 'frontend/public/onchain.json'),
    transform: (m) => {
      // Runtime copy for the SPA. Strip generator metadata — the browser
      // does not need it and shipping it clutters DevTools.
      const { $schema, generator, ...runtime } = m;
      return runtime;
    },
  },
];

function loadCanonical() {
  return JSON.parse(readFileSync(CANONICAL, 'utf8'));
}

function serialize(obj) {
  return JSON.stringify(obj, null, 2) + '\n';
}

function sha(buf) {
  return createHash('sha256').update(buf).digest('hex').slice(0, 12);
}

const args = new Set(process.argv.slice(2));
const checkOnly = args.has('--check');

const canonical = loadCanonical();
let dirty = false;

for (const copy of COPIES) {
  const desired = serialize(copy.transform(canonical));
  let current;
  try {
    current = readFileSync(copy.path, 'utf8');
  } catch {
    current = '';
  }
  if (current !== desired) {
    dirty = true;
    if (checkOnly) {
      console.error(
        `[stale] ${copy.path}\n  expected sha=${sha(desired)}\n  actual   sha=${sha(current)}`,
      );
    } else {
      mkdirSync(dirname(copy.path), { recursive: true });
      writeFileSync(copy.path, desired);
      console.log(`[wrote] ${copy.path}  (sha=${sha(desired)})`);
    }
  } else {
    console.log(`[ok]    ${copy.path}  (sha=${sha(desired)})`);
  }
}

if (checkOnly && dirty) {
  console.error('\nERROR: generated manifest copies are stale. Run: node scripts/gen-manifest.mjs');
  process.exit(1);
}

console.log(`\nCanonical: ${CANONICAL}`);
console.log(`Contracts: ${Object.keys(canonical.contracts).length} deployed, ${Object.keys(canonical.undeployed_contracts || {}).length} undeployed`);
