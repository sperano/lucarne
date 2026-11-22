// The reader's choice to open every collapsible item of the conversation, kept on the device.

/** The localStorage key that holds the choice. */
export const EXPAND_ALL_KEY = 'lucarne.expandAll';
const ON = 'true';

/** The browser's localStorage, or undefined where the browser refuses it. */
export function deviceStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

/** Whether the reader chose to expand every item; off when nothing was saved or storage fails. */
export function loadExpandAll(storage: Storage | undefined): boolean {
  try {
    return storage?.getItem(EXPAND_ALL_KEY) === ON;
  } catch {
    return false;
  }
}

/** Saves the choice; a storage that refuses it (quota, privacy mode) leaves it for this page. */
export function saveExpandAll(storage: Storage | undefined, on: boolean): void {
  try {
    if (on) storage?.setItem(EXPAND_ALL_KEY, ON);
    else storage?.removeItem(EXPAND_ALL_KEY);
  } catch {
    // Kept for this page only.
  }
}
