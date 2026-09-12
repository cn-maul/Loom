import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { adviceApi, eventApi } from '../api/client';
import { RETRIEVAL_LABEL, type AdviceSession, type Event } from '../api/types';
import { eventHeadline, quoteScript, shortDate } from '../format';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { ErrorNote, Notice, Spinner } from './ui';

interface Props {
  advice: AdviceSession;
  personName: string;
  /** Lets the page keep its history list in step after a strategy is adopted. */
  onAdopted: (session: AdviceSession) => void;
}

export default function AdvicePanel({ advice, personName, onAdopted }: Props) {
  const [events, setEvents] = useState<Record<string, Event> | null>(null);
  const [followUpIds, setFollowUpIds] = useState<string[]>(advice.follow_up_ids);
  const [adopting, setAdopting] = useState<number | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    setFollowUpIds(advice.follow_up_ids);
  }, [advice]);

  useEffect(() => {
    let cancelled = false;
    const ids = advice.evidence_event_ids;
    if (ids.length === 0) {
      setEvents({});
      return;
    }
    // A cited record that has since been deleted simply drops out of the map,
    // and the reference renders as "记录已删除" rather than as a dead link.
    Promise.all(ids.map((id) => eventApi.get(id).catch(() => null))).then((list) => {
      if (cancelled) return;
      const found: Record<string, Event> = {};
      list.forEach((event) => {
        if (event) found[event.id] = event;
      });
      setEvents(found);
    });
    return () => {
      cancelled = true;
    };
  }, [advice]);

  const adopt = async (index: number) => {
    if (adopting !== null) return;
    setAdopting(index);
    setError('');
    try {
      const updated = await adviceApi.adopt(advice.id, { strategy_index: index });
      setFollowUpIds(updated.follow_up_ids);
      onAdopted(updated);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setAdopting(null);
    }
  };

  const evidenceFor = (ids: string[]) => <Evidence ids={ids} events={events} />;

  return (
    <div className="space-y-4">
      {advice.source_stale === 1 ? (
        <Notice>
          这份建议的依据已经变了：{advice.source_stale_reason || '来源记录已修改或删除'}。下面引用的内容不再代表现状。
        </Notice>
      ) : null}

      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold text-foreground">情况判断</h3>
          <Badge variant="secondary">{RETRIEVAL_LABEL[advice.retrieval_status] ?? advice.retrieval_status}</Badge>
        </div>
        <p className="text-sm leading-7 text-foreground">{advice.situation.text}</p>
        {evidenceFor(advice.situation.evidence_event_ids)}
        {advice.other_perspective.text ? (
          <div className="mt-3 border-t border-border pt-3">
            <p className="text-sm leading-7 text-muted-foreground">
              <span className="font-medium text-foreground">对方的视角：</span>
              {advice.other_perspective.text}
            </p>
            {evidenceFor(advice.other_perspective.evidence_event_ids)}
          </div>
        ) : null}
      </div>

      {advice.risks.length > 0 ? (
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <h3 className="mb-2 text-sm font-semibold text-foreground">风险</h3>
          <ul className="space-y-2 text-sm leading-6">
            {advice.risks.map((risk, index) => (
              <li key={`${index}-${risk.text}`} className="text-muted-foreground">
                <span className="mr-1 text-foreground">·</span>
                {risk.text}
                {evidenceFor(risk.evidence_event_ids)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="grid gap-3 md:grid-cols-2">
        {advice.strategies.map((strategy, index) => {
          const adopted = advice.adopted_strategy_index === index;
          return (
            <div key={`${index}-${strategy.name}`} className="flex flex-col rounded-xl border border-border bg-card p-4 shadow-sm">
              <div className="flex items-start justify-between gap-2">
                <h3 className="text-sm font-semibold text-primary">{strategy.name}</h3>
                {adopted ? <Badge variant="secondary">已采纳</Badge> : null}
              </div>
              {strategy.script ? (
                <p className="mt-2 rounded-lg bg-muted px-3 py-2 text-sm leading-7 text-foreground">
                  {quoteScript(strategy.script)}
                </p>
              ) : null}
              <div className="mt-2 space-y-1 text-xs leading-5">
                {strategy.pros ? <p className="text-green-700 dark:text-green-400">优点：{strategy.pros}</p> : null}
                {strategy.cons ? <p className="text-muted-foreground">缺点：{strategy.cons}</p> : null}
              </div>
              {evidenceFor(strategy.evidence_event_ids)}
              <div className="mt-3">
                <Button variant="outline" size="sm" disabled={adopting !== null} onClick={() => void adopt(index)}>
                  {adopting === index ? '转出中…' : adopted ? '再转一次跟进事项' : '采纳并转为跟进事项'}
                </Button>
              </div>
            </div>
          );
        })}
      </div>

      {followUpIds.length > 0 ? (
        <p className="text-xs text-muted-foreground">已从这份建议转出 {followUpIds.length} 条跟进事项。</p>
      ) : null}
      {error ? <ErrorNote>{error}</ErrorNote> : null}

      {advice.follow_up.text ? (
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <h3 className="mb-1 text-sm font-semibold text-foreground">后续跟进</h3>
          <p className="text-sm leading-7 text-muted-foreground">{advice.follow_up.text}</p>
          {evidenceFor(advice.follow_up.evidence_event_ids)}
        </div>
      ) : null}

      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <h3 className="mb-2 text-sm font-semibold text-foreground">依据的记录</h3>
        {events === null ? (
          <Spinner label="载入依据…" />
        ) : advice.evidence_event_ids.length === 0 ? (
          <p className="text-sm text-muted-foreground">这次回答没有引用具体记录，结论来自画像和常理推断。</p>
        ) : (
          <ul className="space-y-2">
            {advice.evidence_event_ids.map((id) => {
              const event = events[id];
              if (!event) {
                return (
                  <li key={id} className="text-sm text-muted-foreground">
                    记录已删除（{id.slice(0, 8)}…）
                  </li>
                );
              }
              return (
                <li key={id} className="text-sm">
                  <Link to={`/events/${event.id}`} className="leading-6 text-foreground hover:text-primary">
                    <span className="mr-2 font-mono text-xs text-muted-foreground">{shortDate(event.event_date)}</span>
                    {eventHeadline(event.summary, event.raw_text)}
                  </Link>
                  <span className="ml-2 text-xs text-muted-foreground">{personName}</span>
                </li>
              );
            })}
          </ul>
        )}
        {advice.model ? <p className="mt-3 text-xs text-muted-foreground">由 {advice.model} 生成</p> : null}
      </div>
    </div>
  );
}

/** Renders the records one conclusion rests on, or says plainly that it has none. */
function Evidence({ ids, events }: { ids: string[]; events: Record<string, Event> | null }) {
  if (ids.length === 0) {
    return <p className="mt-1 text-xs text-muted-foreground">无直接记录依据</p>;
  }
  if (events === null) {
    return <p className="mt-1 text-xs text-muted-foreground">载入依据…</p>;
  }
  return (
    <p className="mt-1 text-xs leading-5 text-muted-foreground">
      依据：
      {ids.map((id, index) => {
        const event = events[id];
        return (
          <span key={id}>
            {index > 0 ? '、' : ''}
            {event ? (
              <Link to={`/events/${event.id}`} className="hover:text-primary hover:underline">
                {shortDate(event.event_date)} {eventHeadline(event.summary, event.raw_text)}
              </Link>
            ) : (
              <span>记录已删除</span>
            )}
          </span>
        );
      })}
    </p>
  );
}
