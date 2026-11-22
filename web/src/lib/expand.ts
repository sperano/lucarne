// The reader's choice to open every collapsible item of the conversation, kept on the device.
// Expanded is the default; the key records only the choice to turn it off.

/** The localStorage key that holds the choice. */
export const EXPAND_ALL_KEY = 'lucarne.expandAll';
const OFF = 'false';

/** The browser's localStorage, or undefined where the browser refuses it. */
export function deviceStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

/** Whether the reader chose to expand every item; on when nothing was saved or storage fails. */
export function loadExpandAll(storage: Storage | undefined): boolean {
  try {
    return storage?.getItem(EXPAND_ALL_KEY) !== OFF;
  } catch {
    return true;
  }
}

/** Saves the choice; a storage that refuses it (quota, privacy mode) leaves it for this page. */
export function saveExpandAll(storage: Storage | undefined, on: boolean): void {
  try {
    if (on) storage?.removeItem(EXPAND_ALL_KEY);
    else storage?.setItem(EXPAND_ALL_KEY, OFF);
  } catch {
    // Kept for this page only.
  }
}
