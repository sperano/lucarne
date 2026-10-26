// Every runner fixture's run, read through the real source, stream and
// server, renders its conversation and its end state.
import type { Browser, Locator, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Snapshot } from '../src/lib/contract';
import { END_REASON_LABELS, RUN_PHASE_LABELS } from '../src/lib/labels';
import { runs, snapshot, type RunInfo } from './control';
import { close, launch, open, streamStatus, transcript, visible } from './harness';

const FIXTURE_PREFIX = 'fixture/';
/** Text that Markdown may render differently than it is written. */
const MARKDOWN_SYNTAX = /[`*_[\]<>#|~\\]|^\s*([-+]|\d+\.)\s/;

/** Something of a run that its conversation must show, and how to find it. */
interface Expected {
  readonly what: string;
  readonly find: (page: Page) => Locator;
}

/** Derives what the page must show from the run's own events. */
function expectedOf(snap: Snapshot): Expected {
  for (const e of snap.events) {
    if (e.kind !== 'message' || e.message.role !== 'assistant' || e.message.type === 'reasoning') {
      continue;
    }
    const line = e.message.text.split('\n')[0]?.trim() ?? '';
    if (line === '' || MARKDOWN_SYNTAX.test(line)) continue;
    return {
      what: `the assistant message "${line}"`,
      find: (page) =>
        transcript(page)
          .getByRole('article', { name: 'Assistant message' })
          .filter({ hasText: line }),
    };
  }
  for (const e of snap.events) {
    if (e.kind === 'error') {
      const { message } = e.error;
      return {
        what: `the runner error "${message}"`,
        find: (page) =>
          transcript(page).getByRole('note', { name: 'Runner error' }).filter({ hasText: message }),
      };
    }
    if (e.kind === 'tool_call') {
      const { name } = e.toolCall;
      return {
        what: `the tool call ${name}`,
        find: (page) => transcript(page).getByRole('article', { name: `Tool call ${name}` }),
      };
    }
  }
  throw new Error(`run ${snap.run.taskName} has no message, error or tool call to look for`);
}

const fixtures: RunInfo[] = Object.values(await runs()).filter((r) =>
  r.name.startsWith(FIXTURE_PREFIX),
);

describe('fixture runs', () => {
  let browser: Browser;
  const phases = new Set<string>();

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
    // The fixtures hold both outcomes, so both states were checked.
    expect([...phases].sort()).toEqual(['failed', 'succeeded']);
  });

  it('lists a run for every runner', () => {
    const runners = new Set(fixtures.map((r) => r.name.split('/')[1]));
    expect([...runners].sort()).toEqual(['claude-code', 'codex', 'opencode']);
  });

  it.each(fixtures.map((r) => [r.name, r] as const))(
    '%s renders its conversation',
    async (_, r) => {
      const snap = await snapshot(r);
      const { phase } = snap.run;
      const exitCode = snap.attempts[0]?.container.exitCode;
      // The Task's phase agrees with how its agent exited.
      expect(phase).toBe(exitCode === 0 ? 'succeeded' : 'failed');
      phases.add(phase);
      const expected = expectedOf(snap);

      const watched = await open(browser, r.link);
      const { page } = watched;
      await visible(expected.find(page)).catch((err: unknown) => {
        throw new Error(`${r.name} does not show ${expected.what}`, { cause: err });
      });
      const label = RUN_PHASE_LABELS[phase];
      await visible(page.locator('.run .phase').filter({ hasText: label }));
      await visible(page.getByText(`exit code ${String(exitCode)}`));
      await visible(
        streamStatus(page).filter({
          hasText: `${END_REASON_LABELS.run_finished} Task phase: ${label}.`,
        }),
      );
      await close(watched);
    },
  );
});
