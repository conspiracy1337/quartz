import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Go serves pictures on /tex/ and /img/ (internal/imagesrv). Under `wails dev` the
// dev server answers first, and its page fallback would return index.html for a CSS
// background request; a 404 hands the request to Go's handler instead.
const goImages: Plugin = {
  name: 'go-images',
  configureServer(server) {
    server.middlewares.use((req, res, next) => {
      const url = (req as { url?: string }).url ?? '';
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
  resolve: { alias: { $lib: '/src/lib' } },
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    // One small bundle loads faster from the embedded asset server than many chunks.
    cssCodeSplit: false,
    assetsInlineLimit: 0,
  },
  server: { strictPort: true, port: 5173 },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
});
