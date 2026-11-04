// Service worker Aldebaran Joueur : met l'appli en cache pour qu'elle s'ouvre hors ligne.
const CACHE='aldebaran-joueur-v40';
const FILES=['./','./index.html','./manifest.webmanifest','./icon-192.png','./icon-512.png','./icon-maskable-512.png','./apple-touch-icon.png','./favicon.png'];
self.addEventListener('install',e=>{ e.waitUntil(caches.open(CACHE).then(c=>c.addAll(FILES)).then(()=>self.skipWaiting())); });
self.addEventListener('activate',e=>{ e.waitUntil(caches.keys().then(k=>Promise.all(k.filter(n=>n!==CACHE).map(n=>caches.delete(n)))).then(()=>self.clients.claim())); });
// Réseau d'abord (pour recevoir les mises à jour), cache en secours hors ligne.
self.addEventListener('fetch',e=>{
  if(e.request.method!=='GET') return;
  const same=new URL(e.request.url).origin===location.origin;
  // « no-cache » : on revalide toujours auprès du serveur, pour ne jamais rester sur une ancienne version
  e.respondWith(fetch(e.request,same?{cache:'no-cache'}:{}).then(r=>{ if(r.ok&&new URL(e.request.url).origin===location.origin){ const c=r.clone(); caches.open(CACHE).then(x=>x.put(e.request,c)); } return r; }).catch(()=>caches.match(e.request)));
});
