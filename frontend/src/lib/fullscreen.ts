// Fullscreen on the monitor: Wails' window inside the app (the webview has no
// Fullscreen API there), the browser's element fullscreen in the preview.
import { hasBridge } from './bridge';
import { WindowFullscreen, WindowIsFullscreen, WindowUnfullscreen } from './wailsjs/runtime/runtime';

/** Makes the window fullscreen; the returned function undoes it, unless it already was. */
export async function enterFullscreen(): Promise<() => void> {
  try {
    if (hasBridge()) {
      if (await WindowIsFullscreen()) return () => {};
      WindowFullscreen();
      return () => WindowUnfullscreen();
    }
    if (document.fullscreenElement) return () => {};
    await document.documentElement.requestFullscreen();
    return () => void (document.fullscreenElement && document.exitFullscreen().catch(() => {}));
  } catch {
    // Refused: the view still covers the whole window.
    return () => {};
  }
}
