import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Minecraft's default skins live once, embedded in Go. The preview's fixtures use them as
// textures: /tex/default/<key>.png, and /tex/skin/<key>.png for fixture library skins (real
// library skins are named by SHA-256, so they never match and still reach Go).
const DEFAULTS = '../internal/skins/defaults/';
// The launcher's pictures and fonts live in src/assets, shared with Go.
const ASSETS = '../assets/';
const DEFAULT_SKIN = /^\/tex\/(?:default|skin)\/(slim|wide)-(alex|ari|efe|kai|makena|noor|steve|sunny|zuri)\.png$/;

// Go serves pictures on /tex/ and /img/ (internal/imagesrv). Under `wails dev` the
// dev server answers first, and its page fallback would return index.html for a CSS
// background request; a 404 hands the request to Go's handler instead.
const goImages: Plugin = {
  name: 'go-images',
  configureServer(server) {
    server.middlewares.use((req, res, next) => {
      const url = (req as { url?: string }).url ?? '';
      const def = DEFAULT_SKIN.exec(url);
      if (def) {
        // Vite's own file serving answers /@fs/ (the folder is allowed below).
        (req as { url?: string }).url = '/@fs' + new URL(`${DEFAULTS}${def[1]}/${def[2]}.png`, import.meta.url).pathname;
        next();
        return;
      }
      if (url.startsWith('/tex/') || url.startsWith('/img/')) {
        res.statusCode = 404;
        res.end();
        return;
      }
      next();
    });
  },
};

export default defineConfig({
  plugins: [svelte(), goImages],
  resolve: { alias: { $lib: '/src/lib', $assets: new URL(ASSETS, import.meta.url).pathname } },
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    // One small bundle loads faster from the embedded asset server than many chunks.
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    // The preview's fixtures (lib/api/mock) only build values, so a production build,
    // which never takes a preview fallback, can drop them whole.
    rollupOptions: { treeshake: { moduleSideEffects: (id) => !id.includes('/src/lib/api/mock/') } },
  },
  server: { strictPort: true, port: 5173, fs: { allow: ['.', DEFAULTS, ASSETS] } },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
});
