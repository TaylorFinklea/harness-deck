// harness-deck service worker.
//
// Three responsibilities:
//   1. Cache the app shell so the dashboard loads instantly even on a
//      flaky tailnet (the live data still goes to the network).
//   2. Receive Web Push events and surface them as system notifications.
//   3. Open the relevant report when the user taps a notification.

// Bump CACHE name whenever the precached shell list changes — old SWs
// will purge their prior cache on activate.
const CACHE = 'harness-deck-v2';
const SHELL = ['/', '/manifest.webmanifest', '/icon.svg', '/icon-192.png', '/icon-512.png'];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE).then((c) => c.addAll(SHELL)).catch(() => {})
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

// Network-first for everything else: we want the dashboard to reflect
// the harness's latest report.json as soon as the network can deliver it.
// Cache is a fallback when offline so the shell still loads.
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  // Don't try to cache SSE — it's an open stream, not a resource.
  if (req.headers.get('accept') === 'text/event-stream') return;

  event.respondWith(
    fetch(req)
      .then((resp) => {
        // Only cache same-origin successful HTML/CSS/JS/icon responses.
        if (resp.ok && new URL(req.url).origin === self.location.origin) {
          const copy = resp.clone();
          caches.open(CACHE).then((c) => c.put(req, copy)).catch(() => {});
        }
        return resp;
      })
      .catch(() => caches.match(req).then((hit) => hit || caches.match('/')))
  );
});

// Push notification. Payload is the JSON object Sender.Send delivered:
// {title, body, tag, url, project, run}. Tag dedupes repeated pushes for
// the same ask id so re-running the watcher doesn't stack notifications.
self.addEventListener('push', (event) => {
  if (!event.data) return;
  let payload;
  try {
    payload = event.data.json();
  } catch (e) {
    payload = { title: 'harness-deck', body: event.data.text() };
  }
  event.waitUntil(
    self.registration.showNotification(payload.title || 'harness-deck', {
      body: payload.body || '',
      tag: payload.tag,
      icon: '/icon-192.png',
      badge: '/icon-192.png',
      data: { url: payload.url || '/' },
    })
  );
});

// Tapping a notification opens (or focuses) the deep-linked report.
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientsArr) => {
      for (const c of clientsArr) {
        if ('focus' in c) {
          c.navigate(url).catch(() => {});
          return c.focus();
        }
      }
      if (self.clients.openWindow) return self.clients.openWindow(url);
    })
  );
});
