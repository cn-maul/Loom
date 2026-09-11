import { Link } from 'react-router-dom';
import type { Event } from '../api/types';
import { eventHeadline, shortDate } from '../format';
import { EmptyState } from './ui';

interface Props {
  events: Event[];
  /** Needed when one timeline mixes persons, e.g. the weekly view. */
  personNames?: Record<string, string>;
  showPerson?: boolean;
  emptyText?: string;
}

export default function EventTimeline({ events, personNames, showPerson, emptyText }: Props) {
  if (events.length === 0) {
    return <EmptyState>{emptyText ?? '还没有记录。在上方写一笔。'}</EmptyState>;
  }

  return (
    <ol className="relative space-y-5 border-l border-border pl-5">
      {events.map((event) => {
        const headline = eventHeadline(event.summary, event.raw_text);
        return (
          <li key={event.id} className="relative">
            <span
              aria-hidden
              className="absolute -left-[25px] top-1.5 h-2.5 w-2.5 rounded-full bg-primary/70 ring-4 ring-card"
            />
            <div className="flex flex-wrap items-baseline gap-2 text-xs text-muted-foreground">
              <span className="font-mono">{shortDate(event.event_date)}</span>
              {showPerson && personNames?.[event.person_id] ? (
                <Link to={`/persons/${event.person_id}`} className="hover:text-primary">
                  {personNames[event.person_id]}
                </Link>
              ) : null}
            </div>

            <Link to={`/events/${event.id}`} className="text-sm leading-6 text-foreground hover:text-primary">
              {headline || '（空记录）'}
            </Link>

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
          </li>
        );
      })}
    </ol>
  );
}
