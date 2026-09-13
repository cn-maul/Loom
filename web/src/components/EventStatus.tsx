import type { Event } from '../api/types';
import { cn } from '../lib/utils';

/** The three states the extraction pipeline reports, as the list and the detail
 *  page both show them. A missing status means the row predates the column and
 *  is treated as pending, which is what the repository does too. */
const STATUS_META: Record<string, { label: string; className: string }> = {
  succeeded: {
    label: '已提取',
    className: 'border-emerald-300/60 bg-emerald-50 text-emerald-700 dark:border-emerald-500/40 dark:bg-emerald-950/40 dark:text-emerald-300',
  },
  succeeded_with_warnings: {
    label: '已提取 · 有警告',
    className: 'border-amber-300/60 bg-amber-50 text-amber-700 dark:border-amber-500/40 dark:bg-amber-950/40 dark:text-amber-300',
  },
  pending: {
    label: '待提取',
    className: 'border-border bg-muted text-muted-foreground',
  },
  failed: {
    label: '提取失败',
    className: 'border-red-300/60 bg-red-50 text-red-700 dark:border-red-500/40 dark:bg-red-950/40 dark:text-red-300',
  },
};

export function statusLabel(status: string): string {
  return STATUS_META[status]?.label ?? STATUS_META.pending.label;
}

export function EventStatusBadge({ event, className }: { event: Event; className?: string }) {
  const status = event.extraction_status || 'pending';
  const meta = STATUS_META[status] ?? STATUS_META.pending;
  // "Succeeded with warnings" is not the same as a clean success: indexing or
  // profile refresh may have been skipped, and the user deserves to see that.
  const warn = status === 'succeeded' && event.pipeline_warnings.length > 0;
  const warnMeta = STATUS_META['succeeded_with_warnings'];
  const shown = warn ? warnMeta : meta;
  const tip = warn ? event.pipeline_warnings.join('\n') : event.extraction_error;
  return (
    <span
      className={cn('shrink-0 rounded-full border px-2 py-0.5 text-xs', shown.className, className)}
      title={tip || undefined}
    >
      {event.manually_edited === 1 ? `${shown.label} · 已人工修订` : shown.label}
    </span>
  );
}
