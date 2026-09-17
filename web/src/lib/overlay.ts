import { useEffect, useState } from 'react';

/**
 * Drives the enter transition for an overlay that mounts *already open*.
 *
 * CSS can only animate a change: if the surface is born with
 * `data-state="open"` there is nothing to transition from and it snaps in.
 * The open flag is flipped on the next animation frame — the display-synced
 * clock for exactly this job — and the returned state is *derived*, so
 * deactivating the layer needs no state write at all.
 *
 * Layers that stay mounted and take an `open` prop don't need this: bind
 * `data-state` straight to the prop so the transition also runs on the way
 * out, and a re-trigger mid-flight reverses from wherever it is.
 */
export function useEnterState(active = true): 'closed' | 'open' {
  const [flipped, setFlipped] = useState(false);

  useEffect(() => {
    if (!active) return;
    const raf = requestAnimationFrame(() => setFlipped(true));
    return () => cancelAnimationFrame(raf);
  }, [active]);

  return active && flipped ? 'open' : 'closed';
}
