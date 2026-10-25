import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig, type Plugin } from 'vitest/config';

// The Go server, which serves the API while `vite` serves the UI.
const DEV_API_TARGET = 'http://localhost:8080';

const outDir = resolve(import.meta.dirname, 'dist');

// Go embeds dist/ and refuses an empty pattern, so the build keeps the
// committed placeholder that emptyOutDir removes.
function keepDistPlaceholder(): Plugin {
  return {
    name: 'keep-dist-placeholder',
    apply: 'build',
    closeBundle() {
      writeFileSync(resolve(outDir, '.gitkeep'), '');
    },
  };
}

export default defineConfig({
  plugins: [svelte(), keepDistPlaceholder()],
  build: { outDir, emptyOutDir: true },
  server: {
    // The shared contract lives outside web/.
    fs: { allow: [import.meta.dirname, resolve(import.meta.dirname, '../contract')] },
    proxy: {
      '/api': { target: DEV_API_TARGET },
      '/healthz': { target: DEV_API_TARGET },
    },
  },
  test: { include: ['src/**/*.test.ts'], environment: 'node' },
});
