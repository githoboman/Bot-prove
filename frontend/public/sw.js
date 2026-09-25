/**
 * Bot Prove — Service Worker (backlog 9.3, 9.4).
 *
 * Goals:
 *   1. Cache-first for the shell (index.html + static assets) so
 *      /verify-offline works with no network.
 *   2. Network-first for /v1/... API — never serve stale API data.
 *   3. Bump CACHE_VERSION on every deploy; the SW purges old caches.
 *
 * We intentionally do NOT cache decision payloads or on-chain data.
 * Those change and must never be shown as fresh when they aren't.
 */

const CACHE_VERSION = 'bot-prove-v4'
const SHELL_ASSETS = [
  '/',
  '/index.html',
  '/manifest.webmanifest',
  '/onchain.json',
  '/favicon.ico',
  '/favicon-32x32.png',
  '/favicon-16x16.png',
  '/apple-touch-icon.png',
]

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_VERSION).then(cache => cache.addAll(SHELL_ASSETS))
  )
  self.skipWaiting()
})

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(keys.filter(k => k !== CACHE_VERSION).map(k => caches.delete(k)))
    )
  )
  self.clients.claim()
})

self.addEventListener('fetch', event => {
  const req = event.request
  const url = new URL(req.url)

  // Never cache API traffic — API calls must be authoritative.
  if (url.pathname.startsWith('/v1/') || url.pathname.startsWith('/api/')) {
    event.respondWith(fetch(req))
    return
  }

  // Cache-first for shell/static; fall back to cache if network fails.
  event.respondWith(
    caches.match(req).then(cached => {
      if (cached) return cached
      return fetch(req).then(res => {
        // Cache successful GETs for JS/CSS/HTML/JSON assets.
        if (req.method === 'GET' && res.status === 200 && res.type === 'basic') {
          const clone = res.clone()
          caches.open(CACHE_VERSION).then(c => c.put(req, clone))
        }
        return res
      }).catch(() => caches.match('/index.html'))
    })
  )
})
