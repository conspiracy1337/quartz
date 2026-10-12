import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
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
