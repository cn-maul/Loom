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
 *  note, ✗ hides it and keeps it out of future prompts.
 *
 *  All the notes live on one panel separated by hairlines — a porfolio of
 *  notes is a list, not a set of islands. */
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
    <div>
      {error ? (
        <div className="mb-3">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      <ul className="panel-rows">
        {traits.map((trait) => (
          <li
            key={trait.id}
            className="px-5 py-3.5 transition-opacity"
            style={{ opacity: pending === trait.id ? 0.5 : 1 }}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-[13.5px] font-semibold tracking-[-0.01em] text-foreground">
                    {trait.trait_key}
                  </span>
                  <span className="text-[11.5px] tabular-nums text-ink-4">
                    {Math.round(trait.confidence * 100)}%
                  </span>
                  {trait.verified === 1 ? (
                    <Badge className="bg-live-bg text-live-text">已确认</Badge>
                  ) : null}
                  {trait.source_stale ? (
                    <Badge className="bg-heat-bg text-heat-text" title={trait.source_stale_reason}>
                      依据已变
                    </Badge>
                  ) : null}
                </div>
                <p className="mt-1.5 text-[13.5px] leading-[1.75] text-foreground">{trait.trait_value}</p>
                {trait.source_stale ? (
                  <p className="mt-1.5 text-[12px] leading-[1.65] text-heat-text">
                    {trait.source_stale_reason || '来源记录已变化'}
                    {staleAction ? <> · {staleAction(trait)}</> : null}
                  </p>
                ) : null}
                {trait.source_event_ids.length > 0 ? (
                  <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-[12px]">
                    {trait.source_event_ids.map((id) => (
                      <Link key={id} to={`/events/${id}`} className="text-ink-3 hover:text-primary">
                        依据 · {eventLabels?.[id] ?? `#${id.slice(0, 8)}`}
                      </Link>
                    ))}
                  </div>
                ) : null}
              </div>

              <div className="flex shrink-0 gap-1.5">
                <button
                  onClick={() => verify(trait, 1)}
                  disabled={pending === trait.id}
                  title="准确"
                  aria-label="准确"
                  className={`flex size-8 items-center justify-center rounded-full text-[13px] transition-colors ${
                    trait.verified === 1
                      ? 'bg-live-bg text-live-text'
                      : 'text-ink-3 hover:bg-fill hover:text-foreground'
                  }`}
                >
                  ✓
                </button>
                <button
                  onClick={() => verify(trait, -1)}
                  disabled={pending === trait.id}
                  title="不准确"
                  aria-label="不准确"
                  className="flex size-8 items-center justify-center rounded-full text-[13px] text-ink-3 transition-colors hover:bg-fill hover:text-destructive"
                >
                  ✗
                </button>
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
