import { Link } from 'react-router-dom';
import type { Event } from '../api/types';
import { eventHeadline, shortDate } from '../format';
import { EmptyState } from './layout';

interface Props {
  events: Event[];
  /** Needed when one timeline mixes persons, e.g. the weekly view. */
  personNames?: Record<string, string>;
  showPerson?: boolean;
  emptyText?: string;
}

export default function EventTimeline({ events, personNames, showPerson, emptyText }: Props) {
  if (events.length === 0) {
    return <EmptyState title={emptyText ?? '还没有记录'} description="在上方写一笔，之后会自动提取摘要与承诺。" />;
  }

  return (
    <ol className="relative space-y-3 border-l border-border pl-5">
      {events.map((event) => {
        const headline = eventHeadline(event.summary, event.raw_text);
        return (
          <li key={event.id} className="relative">
            <span
              aria-hidden
              className="absolute -left-[25px] top-3 h-2.5 w-2.5 rounded-full bg-primary/70 ring-4 ring-card"
            />
            {/* The person link sits outside the card link: nesting anchors is invalid HTML. */}
            {showPerson && personNames?.[event.person_id] ? (
              <Link
                to={`/persons/${event.person_id}`}
                className="mb-1 inline-block text-xs font-medium text-primary hover:underline"
              >
                {personNames[event.person_id]}
              </Link>
            ) : null}
            <Link
              to={`/events/${event.id}`}
              className="block rounded-lg border border-border bg-muted/30 px-3 py-2.5 transition-colors hover:border-primary/40 hover:bg-muted/60"
            >
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <span className="font-mono">{shortDate(event.event_date)}</span>
                {event.record_type ? (
                  <span className="rounded-full bg-secondary px-2 py-0.5 text-secondary-foreground">{event.record_type}</span>
                ) : null}
                {event.channel ? <span>{event.channel}</span> : null}
                {event.extraction_status === 'failed' ? (
                  <span className="text-red-600 dark:text-red-400">提取失败</span>
                ) : null}
              </div>

              <p className="mt-1 line-clamp-2 text-sm leading-6 text-foreground">{headline || '（空记录）'}</p>

              {event.my_feeling || event.their_reaction ? (
                <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  {event.my_feeling ? <span>我的感受：{event.my_feeling}</span> : null}
                  {event.their_reaction ? <span>对方反应：{event.their_reaction}</span> : null}
                </div>
              ) : null}

              {event.promises.length > 0 ? (
                <ul className="mt-2 flex flex-wrap gap-2">
                  {event.promises.map((promise, index) => (
                    <li
                      key={`${event.id}-promise-${index}`}
                      className="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary"
                    >
                      承诺 · {promise.who || '对方'}：{promise.what}
                      {promise.deadline ? `（${promise.deadline}）` : ''}
                    </li>
                  ))}
                </ul>
              ) : null}
            </Link>
          </li>
        );
      })}
    </ol>
  );
}
