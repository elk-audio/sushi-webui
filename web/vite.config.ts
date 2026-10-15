import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: { outDir: 'dist', emptyOutDir: true, target: 'es2020' },
  server: {
    proxy: { '/ws': { target: 'http://localhost:8080', ws: true } },
  },
});
