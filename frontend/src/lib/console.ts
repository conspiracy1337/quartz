// The live console shows one run. With several copies of an instance running, it
// follows the newest: the run that started last, or the first one it hears from.

/** Whether a batch of log lines from runId belongs on a console showing current. */
export const follows = (current: string, runId: string): boolean => !current || current === runId;
