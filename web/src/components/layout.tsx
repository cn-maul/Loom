/**
 * Shared page furniture.
 *
 * Every screen used to hand-roll its own headings, cards and empty states,
 * which is why spacing and typography drifted between pages. These primitives
 * fix one consistent rhythm: a page opens with a `PageHeader`, groups content
 * into `SectionCard` (one white panel, not a pile of cards), and explains
 * nothing-to-show with an `EmptyState`.
 *
 * Visual language: Apple — ground #f5f5f7, white panels, hairline dividers,
 * grayscale hierarchy. Colour is reserved for the one accent in a view.
 */
import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

/** Page title with an optional one-line explanation and right-aligned actions. */
export function PageHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('mb-8 flex flex-wrap items-start justify-between gap-3', className)}>
      <div className="min-w-0">
        <h1 className="text-[clamp(22px,2.6vw,28px)] font-bold leading-[1.15] tracking-[-0.03em] text-foreground">
          {title}
        </h1>
        {description ? (
          <p className="mt-2 max-w-[62ch] text-[13.5px] leading-[1.6] text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
}

/**
 * A titled panel. One continuous white surface with a hairline under the
 * header — the header only renders when there is something to put in it.
 */
export function SectionCard({
  title,
  description,
  actions,
  children,
  className,
  bodyClassName,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  const hasHeader = Boolean(title || actions || description);
  return (
    <section className={cn('panel', className)}>
      {hasHeader ? (
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline px-5 py-3.5">
          <div className="min-w-0">
            {title ? (
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">{title}</h2>
            ) : null}
            {description ? <p className="mt-0.5 text-[12.5px] text-muted-foreground">{description}</p> : null}
          </div>
          {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      <div className={cn('p-5', bodyClassName)}>{children}</div>
    </section>
  );
}

/**
 * Nothing to show. No dashed box, no tinted plate — just quiet type on the
 * page ground with the one action that resolves the emptiness.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn('flex flex-col items-center justify-center gap-2 px-6 py-12 text-center', className)}
    >
      {icon ? <div className="mb-1 text-ink-4">{icon}</div> : null}
      <p className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">{title}</p>
      {description ? (
        <p className="max-w-[36ch] text-[13px] leading-[1.65] text-muted-foreground">{description}</p>
      ) : null}
      {action ? <div className="mt-3">{action}</div> : null}
    </div>
  );
}

/**
 * One number with a label, used in the summary strip at the top of a page.
 * Only numbers that carry meaning — no stat padding.
 */
export function StatTile({
  label,
  value,
  hint,
  icon,
  tone = 'default',
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  icon?: ReactNode;
  tone?: 'default' | 'warn' | 'danger';
}) {
  return (
    <div className="rounded-xl bg-card p-[15px] shadow-card">
      <div className="flex items-center gap-1.5 text-[12px] text-muted-foreground">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <div
        className={cn(
          'mt-2 text-[26px] font-semibold leading-none tracking-[-0.02em] [font-variant-numeric:tabular-nums]',
          tone === 'warn' && 'text-heat-text',
          tone === 'danger' && 'text-destructive',
          tone === 'default' && 'text-foreground',
        )}
      >
        {value}
      </div>
      {hint ? <div className="mt-1.5 text-[12px] text-muted-foreground">{hint}</div> : null}
    </div>
  );
}

/** Filter row: controls wrap instead of overflowing on narrow screens. */
export function Toolbar({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('flex flex-wrap items-center gap-2.5', className)}>{children}</div>;
}

/**
 * Initial-based monogram.
 *
 * Deliberately grayscale. This used to hash the name into one of eight pastel
 * colours, which made every list read as a bag of coloured pills — colour as
 * decoration. Identity here comes from the name, which is right there; the
 * palette stays reserved for the one accent in a view.
 *
 * `id` is still accepted so existing call sites keep compiling now that the
 * tone hash is gone.
 */
export function Avatar({
  name,
  size = 'md',
  className,
}: {
  name: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
  id?: string;
}) {
  const trimmed = name.trim();
  // Chinese names read better with the last two characters; Latin ones with initials.
  const label = /[\u4e00-\u9fa5]/.test(trimmed)
    ? trimmed.slice(-2)
    : trimmed
        .split(/\s+/)
        .slice(0, 2)
        .map((part) => part[0]?.toUpperCase() ?? '')
        .join('');

  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full bg-track font-semibold tracking-[-0.01em] text-ink-2',
        size === 'sm' && 'size-7 text-[11px]',
        size === 'md' && 'size-9 text-xs',
        size === 'lg' && 'size-12 text-sm',
        className,
      )}
    >
      {label || '?'}
    </span>
  );
}

/** Label + control + hint, so forms line up within and across pages. */
export function Field({
  label,
  hint,
  children,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={cn('block', className)}>
      <span className="mb-1.5 block text-[12px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="mt-1.5 block text-[12px] text-muted-foreground">{hint}</span> : null}
    </label>
  );
}
