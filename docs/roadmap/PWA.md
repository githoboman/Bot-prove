# Installable PWA / Offline Shell — Design

Ref: `handoff/CP_FINAL_TASKS_V2.md` §D.

## Problem

The current frontend is a plain SPA. Design partners have asked for an
offline-capable installable shell so field reviewers can browse the last
known state of a decision receipt when connectivity drops. **Constraint:**
offline mode must NEVER pretend a write went through.

## Design overview

- Turn the existing Vite/React app into a PWA with a service worker.
- Cache the static shell and a bounded window of GET responses.
- Explicitly badge stale / offline state everywhere it matters.
- Refuse all mutating requests when offline; do NOT queue them.

## Caching policy

- **App shell** (HTML, JS bundle, CSS, fonts): cache-first with
  network-revalidate.
- **API GETs**:
  - `/v1/proofs/:id`, `/v1/receipts/:id`: cache with 5-min TTL, always
    display `stale_at` timestamp.
  - `/v1/registry/*` snapshot: cache with 1-hour TTL, display "cached at
    <ts>, may be behind chain state" badge.
  - `/v1/health`: never cached, offline shows a red offline banner.
- **API mutations (POST/PUT/DELETE)**: never cached, never queued. When
  offline, the client returns a synchronous `OFFLINE` error and the UI
  shows a "you are offline — actions disabled" state.

## Stale/offline UI conventions

Every cached view MUST show, prominently and above the fold:

- A "cached at HH:MM UTC" timestamp.
- An "offline" badge when the network is down.
- A "stale (older than 5 min)" badge when the cache TTL is exceeded but no
  fresh response has arrived.

Never render a decision as "approved" or "denied" without the cache-age
badge. A judge reading the app while offline must see they are looking at
a snapshot, not the live chain state.

## Service worker

- Written in TypeScript alongside the app; built with `vite-plugin-pwa`
  in `injectManifest` mode so we own the SW code.
- Versioned: the SW carries an `APP_VERSION` string; on activate, cached
  responses tagged with an older version are purged.
- `updatefound` handler prompts the user to reload; never applies a new
  SW silently over a page mid-render.

## Data budget

Bounded cache:

- Max 200 receipts.
- Max 50 proofs.
- Max 10 MB total.
- LRU eviction.

The bound is enforced client-side; on overflow, oldest entries are
purged and a debug log is emitted.

## Milestones

1. **Vite PWA plugin + SW skeleton (3 days).** `vite-plugin-pwa` in
   injectManifest mode; SW registers on production build only.
2. **Cache strategy + badges (5 days).** GET caching, stale badges, offline
   banner. Every view audited by hand for stale-safety.
3. **Mutation refusal (2 days).** All POST paths short-circuit in the SW
   when offline; UI shows the disabled state.
4. **QA pass (2 days).** Chrome DevTools offline mode + Lighthouse PWA
   audit ≥ 90.

## Non-goals

- Offline queueing of mutating requests. Explicitly out of scope — this
  would create a class of hard-to-debug consistency bugs.
- Push notifications. Roadmap.
- Background sync of the receipt log. Roadmap.

## Acceptance criteria

- [ ] `frontend/` builds a valid PWA manifest and SW.
- [ ] Every cached view shows a `cached at` timestamp and an offline badge
      when appropriate.
- [ ] Attempted mutations while offline show a disabled state with a
      clear error, no queued write.
- [ ] Lighthouse PWA score ≥ 90.
- [ ] `docs/roadmap/PWA.md` cross-linked from `30-DAY.md`.
