import { reportApi } from '../api/client';
import type { Report } from '../api/types';

/**
 * Portrait generation lives at module level, outside the component tree, so a
 * user who navigates away mid-generation keeps it running and finds the result
 * here when they come back. The backend also detaches generation from the
 * request, so even a full page reload only loses the spinner — the snapshot
 * still lands in the report history.
 */
export type PortraitState =
  | { status: 'idle' }
  | { status: 'running' }
  | { status: 'done'; snapshot: Report }
  | { status: 'error'; message: string };

const IDLE: PortraitState = { status: 'idle' };

const states = new Map<string, PortraitState>();
const listeners = new Map<string, Set<() => void>>();

function notify(personId: string) {
  listeners.get(personId)?.forEach((cb) => cb());
}

export function getPortraitState(personId: string): PortraitState {
  return states.get(personId) ?? IDLE;
}

export function subscribePortrait(personId: string, cb: () => void): () => void {
  let set = listeners.get(personId);
  if (!set) {
    set = new Set();
    listeners.set(personId, set);
  }
  set.add(cb);
  return () => {
    const current = listeners.get(personId);
    if (!current) return;
    current.delete(cb);
    if (current.size === 0) listeners.delete(personId);
  };
}

/** Kick off generation for one person. A second click while running is a no-op. */
export function startPortrait(personId: string, req: { person_id: string; start: string; end: string }): void {
  if (states.get(personId)?.status === 'running') return;
  states.set(personId, { status: 'running' });
  notify(personId);
  reportApi
    .generate(req)
    .then((report) => {
      states.set(personId, { status: 'done', snapshot: report });
    })
    .catch((e: unknown) => {
      states.set(personId, { status: 'error', message: e instanceof Error ? e.message : String(e) });
    })
    .finally(() => notify(personId));
}
