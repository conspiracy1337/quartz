// The add-account dialog's view of Go's accounts:signin events, as a pure reducer.
import type { SignInEvent, SignInProblem } from './api/types';

export interface SignInState {
  /** The sign-in this dialog follows; events of older ones are ignored. */
  flow: number;
  step: string;
  code: string;
  url: string;
  qr: boolean[][];
  expiresAt: number;
  error: SignInProblem | null;
  /** The account signed in, once done. */
  accountId: string;
}

export const idle = (flow = 0): SignInState => ({ flow, step: '', code: '', url: '', qr: [], expiresAt: 0, error: null, accountId: '' });

/** Applies one event. A newer flow starts from scratch; an older one is dropped. */
export function reduce(s: SignInState, e: SignInEvent): SignInState {
  if (e.flow < s.flow) return s;
  const base = e.flow > s.flow ? idle(e.flow) : s;
  return {
    ...base,
    step: e.step ?? base.step,
    code: e.code ?? base.code,
    url: e.url ?? base.url,
    qr: e.qr ?? base.qr,
    expiresAt: e.expiresAt ?? base.expiresAt,
    error: e.error ?? (e.done ? null : base.error),
    accountId: e.accountId ?? base.accountId,
  };
}

/** "14:32" until the code expires. */
export function countdown(expiresAt: number, now: number): string {
  const s = Math.max(0, Math.round((expiresAt - now) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}
