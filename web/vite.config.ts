import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { playwright } from '@vitest/browser-playwright';
import { defineConfig, type Plugin } from 'vitest/config';

// The Go server, which serves the API while `vite` serves the UI.
const DEV_API_TARGET = 'http://localhost:8080';

const outDir = resolve(import.meta.dirname, 'dist');

// Tests of Svelte components (Foo.svelte.test.ts) run in Chromium, the rest in Node.
const COMPONENT_TESTS = 'src/**/*.svelte.test.ts';

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
  // Only the app: fixtures.html is the development preview, served by `vite` alone.
  build: {
    outDir,
    emptyOutDir: true,
    // dist/.vite/license.md: the licenses of the dependencies in the bundle,
    // which the image ships under /licenses. The server serves only
    // index.html and assets/, so not this file.
    license: true,
    rolldownOptions: { input: resolve(import.meta.dirname, 'index.html') },
  },
  server: {
    // The shared contract and the adapters' golden files live outside web/.
    fs: {
      allow: [
        import.meta.dirname,
        resolve(import.meta.dirname, '../contract'),
        resolve(import.meta.dirname, '../internal/adapter'),
      ],
    },
    proxy: {
      '/api': { target: DEV_API_TARGET },
      '/healthz': { target: DEV_API_TARGET },
    },
  },
  test: {
    projects: [
      {
        extends: true,
        test: {
          name: 'unit',
          include: ['src/**/*.test.ts'],
          exclude: [COMPONENT_TESTS],
          environment: 'node',
        },
      },
      {
        extends: true,
        test: {
          name: 'browser',
          include: [COMPONENT_TESTS],
          setupFiles: ['src/browser-setup.ts'],
          browser: {
            enabled: true,
            headless: true,
            provider: playwright(),
            instances: [{ browser: 'chromium' }],
          },
        },
      },
    ],
  },
});
