// Builds lucarne-e2e once (or takes the binary LUCARNE_E2E_SERVER names),
// starts it, waits for its "listening <base URL>" line and provides the
// base URL to the tests as inject('baseURL'); stops it on teardown.
import { execFile, spawn, type ChildProcess } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { promisify } from 'node:util';
import type { TestProject } from 'vitest/node';

declare module 'vitest' {
  export interface ProvidedContext {
    /** The lucarne-e2e server, as http://127.0.0.1:<port>. */
    baseURL: string;
  }
}

const REPO_ROOT = resolve(import.meta.dirname, '../..');
const INDEX_HTML = resolve(import.meta.dirname, '../dist/index.html');
const SERVER_PACKAGE = './internal/e2e/cmd/lucarne-e2e';
const SERVER_ENV = 'LUCARNE_E2E_SERVER';
const LISTENING_PREFIX = 'listening ';
/** How long the server may take to print its address. */
const START_TIMEOUT_MS = 30_000;
/** How long a first `go build`, without a build cache, may take. */
const BUILD_TIMEOUT_MS = 300_000;
/** The status the server answers UI requests with when built without web/dist. */
const UI_NOT_BUILT = 503;

const run = promisify(execFile);

export default async function setup(project: TestProject): Promise<() => Promise<void>> {
  try {
    return await start(project);
  } catch (err) {
    report(err);
    throw err;
  }
}

/**
 * Writes why the setup failed to stderr: Vitest 5 fails to print an error
 * thrown from a globalSetup (its source map lookup throws a TypeError of
 * its own instead), and a failed `go build` says why only on its stderr.
 */
function report(err: unknown): void {
  const message = err instanceof Error ? err.message : String(err);
  const stderr =
    typeof err === 'object' && err !== null && 'stderr' in err ? String(err.stderr) : '';
  process.stderr.write(`\nlucarne e2e setup failed: ${message}\n${stderr}\n`);
}

async function start(project: TestProject): Promise<() => Promise<void>> {
  let buildDir: string | undefined;
  let binary = process.env[SERVER_ENV];
  if (binary === undefined || binary === '') {
    if (!existsSync(INDEX_HTML)) {
      throw new Error(
        `${INDEX_HTML} is missing: run \`npm run build\` before \`npm run test:e2e\`, the server embeds it`,
      );
    }
    buildDir = mkdtempSync(join(tmpdir(), 'lucarne-e2e-'));
    binary = join(buildDir, 'lucarne-e2e');
    await run('go', ['build', '-o', binary, SERVER_PACKAGE], {
      cwd: REPO_ROOT,
      timeout: BUILD_TIMEOUT_MS,
    });
  }
  const server = spawn(binary, [], { stdio: ['ignore', 'pipe', 'inherit'] });
  const stop = async (): Promise<void> => {
    await kill(server);
    if (buildDir !== undefined) rmSync(buildDir, { recursive: true, force: true });
  };
  try {
    const baseURL = await listening(server);
    await checkUI(baseURL);
    project.provide('baseURL', baseURL);
  } catch (err) {
    await stop();
    throw err;
  }
  return stop;
}

/** The base URL the server prints once it serves. */
function listening(server: ChildProcess): Promise<string> {
  const { stdout } = server;
  if (stdout === null) return Promise.reject(new Error('lucarne-e2e has no stdout pipe'));
  return new Promise((resolveURL, reject) => {
    const timer = setTimeout(() => {
      reject(
        new Error(
          `lucarne-e2e printed no "${LISTENING_PREFIX}" line in ${String(START_TIMEOUT_MS)} ms`,
        ),
      );
    }, START_TIMEOUT_MS);
    const done = (): void => {
      clearTimeout(timer);
    };
    server.once('error', (err) => {
      done();
      reject(err);
    });
    server.once('exit', (code) => {
      done();
      reject(new Error(`lucarne-e2e exited with ${String(code)} before it served`));
    });
    const lines = createInterface({ input: stdout });
    lines.on('line', (line) => {
      if (!line.startsWith(LISTENING_PREFIX)) return;
      done();
      resolveURL(line.slice(LISTENING_PREFIX.length).trim());
    });
  });
}

/** Fails when the binary was built before the UI it should embed. */
async function checkUI(baseURL: string): Promise<void> {
  const response = await fetch(baseURL);
  if (response.status === UI_NOT_BUILT) {
    throw new Error(
      `lucarne-e2e serves no UI: build it after \`npm run build\` (${SERVER_ENV} names a stale binary?)`,
    );
  }
}

function kill(server: ChildProcess): Promise<void> {
  if (server.exitCode !== null || server.signalCode !== null) return Promise.resolve();
  return new Promise((resolveExit) => {
    server.once('exit', () => {
      resolveExit();
    });
    server.kill('SIGTERM');
  });
}
