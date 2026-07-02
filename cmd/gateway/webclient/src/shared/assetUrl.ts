// Resolves a runtime asset path (prefab JSON, texture PNG) against the Vite base URL.
//
// The client is served under a sub-path (e.g. /client3d/), but prefab/texture paths
// in code and JSON are written as root-absolute ("/m1.png", "prefubs/Ships/x.json").
// fetch()/TextureLoader don't know about Vite's `base`, so we prepend it here.
export function assetUrl(p: string | undefined): string {
  if (!p) return p ?? '';
  if (p.startsWith('blob:') || p.startsWith('data:') || p.startsWith('http://') || p.startsWith('https://')) {
    return p;
  }
  const base = import.meta.env.BASE_URL || '/'; // e.g. "/client3d/" (always ends with "/")
  return base + p.replace(/^\//, '');
}
