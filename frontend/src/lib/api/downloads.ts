// The job engine (Go's tasks.Engine): live snapshot, 24-hour history and controls.
import { bridged, previewStandIn } from '../bridge';
import * as Go from '../wailsjs/go/api/Downloads';
import { MockTasks } from './mock/downloads';
import type { Job, TaskSnapshot } from './types';

const mock = previewStandIn(() => new MockTasks());

export const downloadsApi = {
  snapshot: bridged(Go.Snapshot, (): TaskSnapshot => mock.snapshot()),
  history: bridged(Go.History, (): Job[] => mock.history),
  pause: bridged(Go.Pause, mock.pause),
  resume: bridged(Go.Resume, mock.resume),
  cancel: bridged(Go.Cancel, mock.cancel),
  retry: bridged(Go.Retry, mock.retry),
  pauseAll: bridged(Go.PauseAll, mock.pauseAll),
  resumeAll: bridged(Go.ResumeAll, mock.resumeAll),
  cancelAll: bridged(Go.CancelAll, mock.cancelAll),
  retryAllFailed: bridged(Go.RetryAllFailed, mock.retryAllFailed),
  clearFinished: bridged(Go.ClearFinished, mock.clearFinished),
};
