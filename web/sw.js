const CACHE = 'zenith-erp-pro-v2';
self.addEventListener('install', e => e.waitUntil(caches.open(CACHE).then(c => c.addAll(['/', '/styles.css', '/app.js', '/manifest.json', '/icon.svg', '/assets/logo.png']))));
self.addEventListener('fetch', e => {
  const url = new URL(e.request.url);
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/document/') || url.pathname.startsWith('/verify/')) return;
  e.respondWith(caches.match(e.request).then(r => r || fetch(e.request)));
});
