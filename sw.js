// Service worker Aldebaran (Aventurier + Maître du destin) : l'appli s'ouvre hors ligne,
// mais va toujours chercher la dernière version sur le réseau quand il y en a un.
const CACHE='aldebaran-v77';
const FILES=['./','./index.html','./manifest.webmanifest','./icon-192.png','./icon-512.png','./icon-maskable-512.png','./apple-touch-icon.png','./favicon.png'];
self.addEventListener('install',e=>{ e.waitUntil(caches.open(CACHE).then(c=>c.addAll(FILES.map(u=>new Request(u,{cache:'reload'})))).then(()=>self.skipWaiting())); });
self.addEventListener('activate',e=>{ e.waitUntil(caches.keys().then(k=>Promise.all(k.filter(n=>n!==CACHE).map(n=>caches.delete(n)))).then(()=>self.clients.claim())); });
self.addEventListener('message',e=>{ if(e.data==='skipWaiting') self.skipWaiting(); });
// Réseau d'abord, sans cache HTTP (« no-store ») pour la page elle-même ; cache en secours hors ligne.
self.addEventListener('fetch',e=>{
  const req=e.request; if(req.method!=='GET') return;
  const url=new URL(req.url), same=url.origin===location.origin;
  if(!same) return;                                   // polices, PeerJS… : le navigateur s'en charge
  if(url.pathname.includes('/fonds/')||req.headers.has('range')) return;   // fonds de carte (vidéos lourdes) : chargés directement
  const nav=req.mode==='navigate';
  const net=fetch(nav?new Request(url.origin+url.pathname,{cache:'no-store',credentials:'same-origin'}):new Request(req.url,{cache:'no-cache'}));
  e.respondWith(net.then(r=>{ if(r.ok){ const c=r.clone(); caches.open(CACHE).then(x=>x.put(nav?url.origin+url.pathname:req,c)); } return r; })
    .catch(()=>caches.match(req,{ignoreSearch:true}).then(r=>r||(nav?caches.match('./index.html'):undefined))));
});
