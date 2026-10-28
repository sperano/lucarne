// The browser side of the suite: one Chromium per test file, and pages
// watched for what must never happen (a console error, an uncaught
// exception, a dialog, a request leaving the server, a line of a run
// Lucarne must not read).
import { chromium, type Browser, type Locator, type Page, type Response } from 'playwright';
import { expect } from 'vitest';
import { baseURL } from './control';

/** How long the UI may take to show what a test waits for. */
export const UI_TIMEOUT_MS = 10_000;
/** How often a polled condition is checked. */
export const POLL_INTERVAL_MS = 50;

/** EventSource.readyState once closed. */
const EVENT_SOURCE_CLOSED = 2;

// The lines of the runs Lucarne must never read, as in
// internal/e2e/scenario/runs.go: no page may ever show them.
export const FOREIGN_MARKER = 'FOREIGN-POD-LINE';
export const PRIVATE_MARKER = 'PRIVATE-NAMESPACE-LINE';
const UNREADABLE_MARKERS = [FOREIGN_MARKER, PRIVATE_MARKER];

/** The console error Chromium logs for an answer with an error status. */
export const FAILED_RESOURCE =
  /^Failed to load resource: the server responded with a status of 40[04]/;

declare global {
  interface Window {
    /** Every EventSource the page opened, recorded by the init script. */
    __e2eStreams?: EventSource[];
  }
}

/** A page and what it did that a test may assert never happened. */
export interface Watched {
  readonly page: Page;
  /** The answer to the document request. */
  readonly response: Response | null;
  /** Console errors and uncaught exceptions. */
  readonly errors: string[];
  /** The paths of the page's requests to the server, in order. */
  readonly requests: string[];
  /** Requests to another origin than the server's. */
  readonly offOrigin: string[];
  readonly dialogs: string[];
}

export function launch(): Promise<Browser> {
  return chromium.launch();
}

/** Records the page's EventSources, so a test can tell whether a stream is still open. */
function recordStreams(): void {
  const Native = window.EventSource;
  const opened: EventSource[] = [];
  window.__e2eStreams = opened;
  window.EventSource = class extends Native {
    constructor(url: string | URL, init?: EventSourceInit) {
      super(url, init);
      opened.push(this);
    }
  };
}

/** A new page in its own context, watched, at path. */
export async function open(browser: Browser, path: string): Promise<Watched> {
  const context = await browser.newContext();
  await context.addInitScript(recordStreams);
  const page = await context.newPage();
  const errors: string[] = [];
  const requests: string[] = [];
  const offOrigin: string[] = [];
  const dialogs: string[] = [];
  const origin = new URL(baseURL()).origin;
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text());
  });
  page.on('pageerror', (err) => errors.push(`uncaught: ${err.message}`));
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.origin === origin) requests.push(url.pathname);
    else offOrigin.push(request.url());
  });
  page.on('dialog', (dialog) => {
    dialogs.push(`${dialog.type()}: ${dialog.message()}`);
    void dialog.dismiss();
  });
  const response = await page.goto(`${baseURL()}${path}`);
  return { page, response, errors, requests, offOrigin, dialogs };
}

/**
 * Checks what must hold of every page, then closes it: no error but those
 * allowed, no dialog, no request off the server, no unreadable line.
 */
export async function close(watched: Watched, allowedErrors?: RegExp): Promise<void> {
  const { page } = watched;
  const text = await page.locator('body').innerText();
  for (const marker of UNREADABLE_MARKERS) expect(text).not.toContain(marker);
  expect(await page.content()).not.toMatch(new RegExp(UNREADABLE_MARKERS.join('|')));
  expect(
    watched.errors.filter((e) => allowedErrors === undefined || !allowedErrors.test(e)),
  ).toEqual([]);
  expect(watched.dialogs).toEqual([]);
  expect(watched.offOrigin).toEqual([]);
  await page.context().close();
}

/** Waits for the first element of locator to be visible. */
export async function visible(locator: Locator): Promise<void> {
  await locator.first().waitFor({ state: 'visible', timeout: UI_TIMEOUT_MS });
}

/** Waits for locator to match nothing visible. */
export async function hidden(locator: Locator): Promise<void> {
  await locator.first().waitFor({ state: 'hidden', timeout: UI_TIMEOUT_MS });
}

/** Polls value until it passes check, within UI_TIMEOUT_MS. */
export function eventually<T>(value: () => Promise<T> | T) {
  return expect.poll(value, { timeout: UI_TIMEOUT_MS, interval: POLL_INTERVAL_MS });
}

/** The transcript of the conversation view. */
export function transcript(page: Page): Locator {
  return page.getByRole('region', { name: 'Transcript' });
}

/** The items the transcript renders now. */
export function renderedItems(page: Page): Locator {
  return transcript(page).locator(':scope > ol > li');
}

/** The assistant messages the transcript renders. */
export function assistantMessages(page: Page): Locator {
  return transcript(page).getByRole('article', { name: 'Assistant message' });
}

/** The role label a message starts with. */
const ASSISTANT_LABEL = /^Assistant\s*/;

/** The text of the rendered assistant messages, in order, without their role label. */
export async function messageTexts(page: Page): Promise<string[]> {
  const texts = await assistantMessages(page).allInnerTexts();
  return texts.map((t) => t.replace(ASSISTANT_LABEL, '').trim());
}

/** The live stream status line. */
export function streamStatus(page: Page): Locator {
  return page.getByRole('status');
}

/** How many of the page's EventSources are not closed. */
export function openStreams(page: Page): Promise<number> {
  return page.evaluate(
    (closed) => (window.__e2eStreams ?? []).filter((s) => s.readyState !== closed).length,
    EVENT_SOURCE_CLOSED,
  );
}

/** Whether locator's first element is at least partly inside the viewport. */
export function inViewport(locator: Locator): Promise<boolean> {
  return locator.first().evaluate((element) => {
    const box = element.getBoundingClientRect();
    return box.bottom > 0 && box.top < window.innerHeight && box.height > 0;
  });
}

/** The query that opens a run in the inspector. */
export const INSPECTOR_QUERY = 'view=inspector';

/** The inspector's list of the attempt's events. */
export function eventList(page: Page): Locator {
  return page.getByRole('listbox', { name: 'Events of this attempt' });
}

/** The inspector's row of the event id. */
export function eventRow(page: Page, id: string): Locator {
  return eventList(page).locator(`[data-event="${id}"]`);
}

/** The inspector's detail of the selected event. */
export function eventDetail(page: Page): Locator {
  return page.getByRole('region', { name: 'Event detail' });
}

/** The link of the view switch to the view called name. */
export function viewLink(page: Page, name: 'Conversation' | 'Inspector'): Locator {
  return page.getByRole('navigation', { name: 'View' }).getByRole('link', { name });
}

/** Whether locator's first element has the focus. */
export function focused(locator: Locator): Promise<boolean> {
  return locator.first().evaluate((element) => element === document.activeElement);
}
