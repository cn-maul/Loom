import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

/** Shared field style for the handful of raw `<input>` / `<select>` sites. */
export const controlClass =
  'border-hairline bg-card h-11 w-full rounded-md border px-3.5 text-[14px] text-foreground shadow-xs outline-none transition-[border-color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25';

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-[13px] leading-[1.6] text-ink-3">
      <span
        aria-hidden
        className="h-4 w-4 animate-spin rounded-full border-2 border-track border-t-ink-3"
      />
      {label ? <span>{label}</span> : null}
    </span>
  );
}

/** Failure. Tinted, not boxed — a status colour is information, not decor. */
export function ErrorNote({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md bg-destructive/10 px-3.5 py-2.5 text-[13px] leading-6 text-destructive">
      {children}
    </div>
  );
}

/** Warning / needs-attention, in the one heat colour. */
export function Notice({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-md bg-heat-bg px-3.5 py-2.5 text-[13px] leading-6 text-heat-text">{children}</div>
  );
}

export function EmptyState({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('py-16 text-center text-[13.5px] text-muted-foreground', className)}>{children}</div>;
}
