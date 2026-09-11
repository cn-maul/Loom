import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { eventApi } from '../api/client';
import type { AdviceResponse, Event } from '../api/types';
import { eventHeadline, quoteScript, shortDate } from '../format';
import { Badge } from './ui/badge';
import { Spinner } from './ui';

interface Props {
  advice: AdviceResponse;
  personName: string;
}

export default function AdvicePanel({ advice, personName }: Props) {
  const [evidence, setEvidence] = useState<Event[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (advice.evidence_event_ids.length === 0) {
      setEvidence([]);
      return;
    }
    // An evidence id whose event has since been deleted must simply drop out.
    Promise.all(advice.evidence_event_ids.map((id) => eventApi.get(id).catch(() => null))).then((list) => {
      if (!cancelled) setEvidence(list.filter((event): event is Event => event !== null));
    });
    return () => {
      cancelled = true;
    };
  }, [advice]);

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold text-foreground">情况判断</h3>
          <Badge variant="secondary">{advice.vector_used ? '语义检索 + 近期记录' : '仅近期记录（向量检索未参与）'}</Badge>
        </div>
        <p className="text-sm leading-7 text-foreground">{advice.situation}</p>
        {advice.other_perspective ? (
          <p className="mt-2 text-sm leading-7 text-muted-foreground">
            <span className="font-medium text-foreground">对方的视角：</span>
            {advice.other_perspective}
          </p>
        ) : null}
      </div>

      {advice.risks.length > 0 ? (
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <h3 className="mb-2 text-sm font-semibold text-foreground">风险</h3>
          <ul className="list-disc space-y-1 pl-5 text-sm leading-6 text-muted-foreground">
            {advice.risks.map((risk) => (
              <li key={risk}>{risk}</li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="grid gap-3 md:grid-cols-2">
        {advice.strategies.map((strategy) => (
          <div key={strategy.name} className="rounded-xl border border-border bg-card p-4 shadow-sm">
            <h3 className="text-sm font-semibold text-primary">{strategy.name}</h3>
            {strategy.script ? (
              <p className="mt-2 rounded-lg bg-muted px-3 py-2 text-sm leading-7 text-foreground">
                {quoteScript(strategy.script)}
              </p>
            ) : null}
            <div className="mt-2 space-y-1 text-xs leading-5">
              {strategy.pros ? <p className="text-green-700 dark:text-green-400">优点：{strategy.pros}</p> : null}
              {strategy.cons ? <p className="text-muted-foreground">缺点：{strategy.cons}</p> : null}
            </div>
          </div>
        ))}
      </div>

      {advice.follow_up ? (
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <h3 className="mb-1 text-sm font-semibold text-foreground">后续跟进</h3>
          <p className="text-sm leading-7 text-muted-foreground">{advice.follow_up}</p>
        </div>
      ) : null}

      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <h3 className="mb-2 text-sm font-semibold text-foreground">依据的记录</h3>
        {evidence === null ? (
          <Spinner label="载入依据…" />
        ) : evidence.length === 0 ? (
          <p className="text-sm text-muted-foreground">这次回答没有引用具体记录。</p>
        ) : (
          <ul className="space-y-2">
            {evidence.map((event) => (
              <li key={event.id} className="text-sm">
                <Link to={`/events/${event.id}`} className="leading-6 text-foreground hover:text-primary">
                  <span className="mr-2 font-mono text-xs text-muted-foreground">{shortDate(event.event_date)}</span>
                  {eventHeadline(event.summary, event.raw_text)}
                </Link>
                <span className="ml-2 text-xs text-muted-foreground">{personName}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
