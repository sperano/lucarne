// The source-to-browser suite: real Chromium against lucarne-e2e, the real
// server over a fake cluster (internal/e2e/cmd/lucarne-e2e). Run with
// `npm run test:e2e` after `npm run build`, whose UI the server embeds;
// `npm test` does not include it.
import { defineConfig } from 'vitest/config';

/** How long one test may take: the long session loads 20 000 events. */
const TEST_TIMEOUT_MS = 60_000;
/** How long a hook may take: launching Chromium. */
const HOOK_TIMEOUT_MS = 30_000;

export default defineConfig({
  test: {
    include: ['e2e/**/*.test.ts'],
    environment: 'node',
    globalSetup: ['e2e/global-setup.ts'],
    // One server for every file; the files run one after the other so that
    // the stream cuts of one cannot reach the pages of another.
    fileParallelism: false,
    testTimeout: TEST_TIMEOUT_MS,
    hookTimeout: HOOK_TIMEOUT_MS,
  },
});
