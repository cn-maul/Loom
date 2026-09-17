import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { adviceApi, eventApi } from '../api/client';
import { RETRIEVAL_LABEL, type AdviceSession, type Event } from '../api/types';
import { eventHeadline, quoteScript, shortDate } from '../format';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { ErrorNote, Notice, Spinner } from './ui';
import { SectionCard } from './layout';

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
    <div className="space-y-5">
      {advice.source_stale === 1 ? (
        <Notice>
          这份建议的依据已经变了：{advice.source_stale_reason || '来源记录已修改或删除'}。下面引用的内容不再代表现状。
        </Notice>
      ) : null}

      <SectionCard
        title="情况判断"
        actions={<Badge variant="secondary">{RETRIEVAL_LABEL[advice.retrieval_status] ?? advice.retrieval_status}</Badge>}
      >
        <p className="text-[14px] leading-[1.85] text-ink-2">{advice.situation.text}</p>
        {evidenceFor(advice.situation.evidence_event_ids)}
        {advice.other_perspective.text ? (
          <div className="mt-3.5 border-t border-hairline pt-3.5">
            <p className="text-[14px] leading-[1.85] text-ink-2">
              <span className="font-medium text-foreground">对方的视角：</span>
              {advice.other_perspective.text}
            </p>
            {evidenceFor(advice.other_perspective.evidence_event_ids)}
          </div>
        ) : null}
      </SectionCard>

      {advice.risks.length > 0 ? (
        <SectionCard title="风险">
          <ul className="space-y-2.5">
            {advice.risks.map((risk, index) => (
              <li key={`${index}-${risk.text}`} className="text-[13.5px] leading-[1.7] text-ink-2">
                <span className="mr-1.5 text-ink-4">·</span>
                {risk.text}
                {evidenceFor(risk.evidence_event_ids)}
              </li>
            ))}
          </ul>
        </SectionCard>
      ) : null}

      {/* Strategies are sibling options, so they share ONE panel separated by
          hairlines — a grid of individually-bordered cards would read as a bag
          of islands. The accent lives on the button, not on the heading. */}
      {advice.strategies.length > 0 ? (
        <section className="panel">
          <div className="border-b border-hairline px-5 py-[13px]">
            <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">可选做法</h2>
          </div>
          <ul className="panel-rows">
            {advice.strategies.map((strategy, index) => {
              const adopted = advice.adopted_strategy_index === index;
              return (
                <li key={`${index}-${strategy.name}`} className="px-5 py-4">
                  <div className="flex items-start justify-between gap-3">
                    <h3 className="text-[14px] font-semibold tracking-[-0.01em] text-foreground">{strategy.name}</h3>
                    {adopted ? <Badge className="bg-live-bg text-live-text">已采纳</Badge> : null}
                  </div>

                  {strategy.script ? (
                    <p className="mt-2.5 rounded-md bg-fill px-3.5 py-3 text-[13.5px] leading-[1.85] text-ink-2">
                      {quoteScript(strategy.script)}
                    </p>
                  ) : null}

                  <div className="mt-2.5 space-y-1 text-[12.5px] leading-[1.65]">
                    {strategy.pros ? <p className="text-live-text">优点：{strategy.pros}</p> : null}
                    {strategy.cons ? <p className="text-ink-3">缺点：{strategy.cons}</p> : null}
                  </div>

                  {evidenceFor(strategy.evidence_event_ids)}

                  <div className="mt-3.5">
                    <Button variant="outline" size="sm" disabled={adopting !== null} onClick={() => void adopt(index)}>
                      {adopting === index ? '转出中…' : adopted ? '再转一次跟进事项' : '采纳并转为跟进事项'}
                    </Button>
                  </div>
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}

      {followUpIds.length > 0 ? (
        <p className="text-[12px] text-ink-3">已从这份建议转出 {followUpIds.length} 条跟进事项。</p>
      ) : null}

      {error ? <ErrorNote>{error}</ErrorNote> : null}

      {advice.follow_up.text ? (
        <SectionCard title="后续跟进">
          <p className="text-[13.5px] leading-[1.85] text-ink-2">{advice.follow_up.text}</p>
          {evidenceFor(advice.follow_up.evidence_event_ids)}
        </SectionCard>
      ) : null}

      <SectionCard title="依据的记录">
        {events === null ? (
          <Spinner label="载入依据…" />
        ) : advice.evidence_event_ids.length === 0 ? (
          <p className="text-[13px] leading-[1.7] text-muted-foreground">
            这次回答没有引用具体记录，结论来自画像和常理推断。
          </p>
        ) : (
          <ul className="panel-rows">
            {advice.evidence_event_ids.map((id) => {
              const event = events[id];
              if (!event) {
                return (
                  <li key={id} className="py-2 text-[13px] text-muted-foreground">
                    记录已删除（{id.slice(0, 8)}…）
                  </li>
                );
              }
              return (
                <li key={id} className="flex flex-wrap items-baseline gap-x-2 py-2">
                  <Link
                    to={`/events/${event.id}`}
                    className="text-[13.5px] leading-[1.6] text-foreground hover:text-primary"
                  >
                    <span className="mr-2 tabular-nums text-[12px] text-ink-3">{shortDate(event.event_date)}</span>
                    {eventHeadline(event.summary, event.raw_text)}
                  </Link>
                  <span className="text-[12px] text-ink-3">{personName}</span>
                </li>
              );
            })}
          </ul>
        )}
        {advice.model ? <p className="mt-3.5 text-[12px] text-ink-3">由 {advice.model} 生成</p> : null}
      </SectionCard>
    </div>
  );
}

/** Renders the records one conclusion rests on, or says plainly that it has none. */
function Evidence({ ids, events }: { ids: string[]; events: Record<string, Event> | null }) {
  if (ids.length === 0) {
    return <p className="mt-1.5 text-[12px] text-ink-4">无直接记录依据</p>;
  }
  if (events === null) {
    return <p className="mt-1.5 text-[12px] text-ink-4">载入依据…</p>;
  }
  return (
    <p className="mt-1.5 text-[12px] leading-[1.7] text-ink-3">
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
