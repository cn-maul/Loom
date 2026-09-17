import type { Event } from '../api/types';
import { cn } from '../lib/utils';

/** The three states the extraction pipeline reports, as the list and the detail
 *  page both show them. A missing status means the row predates the column and
 *  is treated as pending, which is what the repository does too.
 *
 *  Tint plates, not outlined chips: a status is information, so it gets a
 *  translucent Apple system tint that reads on the white panel and on the dark
 *  ground alike. */
const STATUS_META: Record<string, { label: string; className: string }> = {
  succeeded: {
    label: '已提取',
    className: 'bg-live-bg text-live-text',
  },
  succeeded_with_warnings: {
    label: '已提取 · 有警告',
    className: 'bg-heat-bg text-heat-text',
  },
  pending: {
    label: '待提取',
    className: 'bg-fill text-ink-2',
  },
  failed: {
    label: '提取失败',
    className: 'bg-destructive/10 text-destructive',
  },
};

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
      className={cn(
        'shrink-0 rounded-full px-2.5 py-[3px] text-[11.5px] font-medium',
        shown.className,
        className,
      )}
      title={tip || undefined}
    >
      {event.manually_edited === 1 ? `${shown.label} · 已人工修订` : shown.label}
    </span>
  );
}
