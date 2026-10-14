// What the launcher is asked to open from outside (links on web pages, files the OS
// opens, the command line, a second launch) is held in Go until the UI collects it:
// once at boot, then whenever app:open says there is more.
import { app, type OpenTarget } from './api/system';
import { onEvent } from './bridge';
import { go } from './router.svelte';
import { session } from './session.svelte';

/** Shows one target: the Import preview, a project page, an instance, or an error toast. */
export function show(t: OpenTarget) {
  switch (t.kind) {
    case 'import':
      if (t.openId) go({ name: 'import', open: t.openId });
      break;
    case 'project':
      if (t.project) go({ name: 'project', id: t.project });
      break;
    case 'instance':
      if (t.instance) go({ name: 'instance', id: t.instance, tab: 'overview' });
      break;
    case 'notice':
      session.notify({ subject: t.subject ?? '', text: t.text ?? '' });
      break;
    case 'error':
      session.notify({ tone: 'error', subject: t.subject ?? "Couldn't open that", text: t.text ?? '' });
      break;
    case 'quit-blocked':
      // Closing waits for downloads; closing again while this shows quits too.
      session.notify({ subject: t.subject ?? '', text: t.text ?? '', action: { label: 'Quit anyway', run: () => void app.quit() } });
      break;
  }
}

async function take() {
  for (const t of await app.pending()) show(t);
}

export function listenOpens(): () => void {
  void take();
  return onEvent('app:open', () => void take());
}
