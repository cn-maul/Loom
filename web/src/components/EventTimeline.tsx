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

/**
 * A person's history as one continuous surface: hairline-separated rows on a
 * single panel. It used to be a stack of individually bordered cards with a
 * timeline rail, which read as a bag of islands — same information, more noise.
 */
export default function EventTimeline({ events, personNames, showPerson, emptyText }: Props) {
  if (events.length === 0) {
    return <EmptyState title={emptyText ?? '还没有记录'} description="在上方写一笔，之后会自动提取摘要与承诺。" />;
  }

  return (
    <ol className="panel-rows">
      {events.map((event) => {
        const headline = eventHeadline(event.summary, event.raw_text);
        return (
          <li key={event.id} className="transition-colors hover:bg-hover">
            {/* The person link sits outside the card link: nesting anchors is invalid HTML. */}
            {showPerson && personNames?.[event.person_id] ? (
              <Link
                to={`/persons/${event.person_id}`}
                className="mb-1 inline-block text-[12px] font-medium text-primary hover:underline"
              >
                {personNames[event.person_id]}
              </Link>
            ) : null}
            <Link to={`/events/${event.id}`} className="block px-5 py-3.5">
              <div className="flex flex-wrap items-center gap-2 text-[12px] text-ink-3">
                <span className="tabular-nums">{shortDate(event.event_date)}</span>
                {event.record_type ? (
                  <span className="rounded-full bg-fill px-2 py-[3px] text-[11px] text-ink-2">
                    {event.record_type}
                  </span>
                ) : null}
                {event.channel ? <span>{event.channel}</span> : null}
                {event.extraction_status === 'failed' ? (
                  <span className="font-medium text-destructive">提取失败</span>
                ) : null}
              </div>

              <p className="mt-1.5 line-clamp-2 text-[13.5px] leading-[1.75] text-foreground">
                {headline || '（空记录）'}
              </p>

              {event.my_feeling || event.their_reaction ? (
                <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-ink-3">
                  {event.my_feeling ? <span>我的感受：{event.my_feeling}</span> : null}
                  {event.their_reaction ? <span>对方反应：{event.their_reaction}</span> : null}
                </div>
              ) : null}

              {event.promises.length > 0 ? (
                <ul className="mt-2 flex flex-wrap gap-1.5">
                  {event.promises.map((promise, index) => (
                    <li
                      key={`${event.id}-promise-${index}`}
                      className="rounded-full bg-fill px-2.5 py-[3px] text-[11.5px] text-ink-2"
                    >
                      <span className="font-medium text-foreground">承诺</span> · {promise.who || '对方'}：
                      {promise.what}
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
