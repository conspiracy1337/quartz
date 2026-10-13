// Copying text: Wails' clipboard inside the app (the webview may refuse the
// browser API), the browser's clipboard in the preview.
import { hasBridge } from './bridge';
import { ClipboardSetText } from './wailsjs/runtime/runtime';

/** Puts text on the clipboard; resolves false when the system refused. */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (hasBridge()) return await ClipboardSetText(text);
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
