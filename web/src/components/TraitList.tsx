import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { traitApi } from '../api/client';
import type { Trait } from '../api/types';
import { EmptyState, ErrorNote } from './ui';
import { Badge } from './ui/badge';

interface Props {
  traits: Trait[];
  /** Reports the verdict that succeeded so the owner can mirror it in its own state. */
  onVerified: (id: string, verified: number) => void;
  /** Event id → human label (usually its date), so evidence links are readable. */
  eventLabels?: Record<string, string>;
  /** Stale notes point at a record that changed; this links straight to it so
   *  the user can re-extract and have the note re-derived. */
  staleAction?: (trait: Trait) => ReactNode;
}

/** The user's verdict is the product's correction channel: ✓ strengthens a profile
 *  note, ✗ hides it and keeps it out of future prompts. */
export default function TraitList({ traits, onVerified, eventLabels, staleAction }: Props) {
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState('');

  const verify = async (trait: Trait, verified: number) => {
    setPending(trait.id);
    setError('');
    try {
      await traitApi.verify(trait.id, verified);
      onVerified(trait.id, verified);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setPending(null);
    }
  };

  if (traits.length === 0) {
    return <EmptyState>还没有画像。记录几件事之后 AI 会自动总结这个人的特点。</EmptyState>;
  }

  return (
    <div className="space-y-3">
      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {traits.map((trait) => (
        <div
          key={trait.id}
          className={`rounded-xl border bg-card p-3 shadow-sm transition-opacity ${
            trait.source_stale ? 'border-amber-300/70 dark:border-amber-500/40' : 'border-border'
          }`}
          style={{ opacity: pending === trait.id ? 0.5 : 1 }}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-foreground">{trait.trait_key}</span>
                <span className="text-xs text-muted-foreground">{Math.round(trait.confidence * 100)}%</span>
                {trait.verified === 1 ? (
                  <Badge variant="secondary" className="border-transparent bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-400">
                    已确认
                  </Badge>
                ) : null}
                {trait.source_stale ? (
                  <Badge
                    variant="secondary"
                    className="border-transparent bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300"
                    title={trait.source_stale_reason}
                  >
                    依据已变
                  </Badge>
                ) : null}
              </div>
              <p className="mt-1 text-sm leading-6 text-foreground">{trait.trait_value}</p>
              {trait.source_stale ? (
                <p className="mt-1 text-xs leading-5 text-amber-700 dark:text-amber-400">
                  {trait.source_stale_reason || '来源记录已变化'}
                  {staleAction ? <> · {staleAction(trait)}</> : null}
                </p>
              ) : null}
              {trait.source_event_ids.length > 0 ? (
                <div className="mt-1 flex flex-wrap gap-2 text-xs">
                  {trait.source_event_ids.map((id) => (
                    <Link key={id} to={`/events/${id}`} className="text-muted-foreground hover:text-primary">
                      依据 · {eventLabels?.[id] ?? `#${id.slice(0, 8)}`}
                    </Link>
                  ))}
                </div>
              ) : null}
            </div>

            <div className="flex shrink-0 gap-1">
              <button
                onClick={() => verify(trait, 1)}
                disabled={pending === trait.id}
                title="准确"
                className={`h-8 w-8 rounded-lg border text-sm transition-colors ${
                  trait.verified === 1
                    ? 'border-green-300 bg-green-50 text-green-700 dark:bg-green-900/40 dark:text-green-400'
                    : 'border-border text-muted-foreground hover:border-green-300 hover:text-green-600'
                }`}
              >
                ✓
              </button>
              <button
                onClick={() => verify(trait, -1)}
                disabled={pending === trait.id}
                title="不准确"
                className="h-8 w-8 rounded-lg border border-border text-sm text-muted-foreground hover:border-red-300 hover:text-red-600"
              >
                ✗
              </button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
