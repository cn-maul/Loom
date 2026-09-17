import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { NotebookPen } from 'lucide-react';
import { QUICK_RECORD_EVENT, eventApi, personApi } from '../api/client';
import type { Event, PersonWithActivity } from '../api/types';
import { Avatar, EmptyState } from '../components/layout';
import { ErrorNote, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { eventHeadline, relativeTime, todayISO } from '../format';
import { cn } from '../lib/utils';

/** A person whose last record is older than this reads as "fading". */
const STALE_DAYS = 30;

const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六'];

function daysSince(date: string): number | null {
  if (!date) return null;
  const last = new Date(`${date.slice(0, 10)}T00:00:00`);
  if (Number.isNaN(last.getTime())) return null;
  const now = new Date(`${todayISO()}T00:00:00`);
  return Math.floor((now.getTime() - last.getTime()) / 86400000);
}

/** "今天 / 昨天 / 9月10日 周四" — the natural-language day header. */
function dayLabel(date: string): string {
  const diff = daysSince(date);
  if (diff === 0) return '今天';
  if (diff === 1) return '昨天';
  const d = new Date(`${date.slice(0, 10)}T00:00:00`);
  if (Number.isNaN(d.getTime())) return date;
  return `${d.getMonth() + 1}月${d.getDate()}日 周${WEEKDAYS[d.getDay()]}`;
}

function groupByDay(events: Event[]): { date: string; items: Event[] }[] {
  const groups: { date: string; items: Event[] }[] = [];
  for (const event of events) {
    const day = event.event_date.slice(0, 10);
    const last = groups[groups.length - 1];
    if (last && last.date === day) last.items.push(event);
    else groups.push({ date: day, items: [event] });
  }
  return groups;
}

/** One record row on the shared panel. Person first, what happened second;
 *  the extracted fields are quiet tags, not a second layer of colour. */
function RecordRow({ event, persons }: { event: Event; persons: PersonWithActivity[] }) {
  const person = persons.find((p) => p.id === event.person_id);
  const headline = eventHeadline(event.summary, event.raw_text);
  return (
    <li className="group px-5 py-4 transition-colors hover:bg-hover">
      <div className="flex gap-3.5">
        {person ? (
          <Link to={`/persons/${person.id}`} className="shrink-0" title={person.name}>
            <Avatar name={person.name} id={person.id} size="md" />
          </Link>
        ) : (
          <span className="grid size-9 shrink-0 place-items-center rounded-full bg-track text-ink-3">
            <NotebookPen className="size-4" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            {person ? (
              <Link
                to={`/persons/${person.id}`}
                className="text-[14px] font-semibold tracking-[-0.01em] text-foreground hover:text-primary"
              >
                {person.name}
              </Link>
            ) : (
              <span className="text-[14px] font-medium text-ink-3">未关联人物</span>
            )}
            <span className="truncate text-[12.5px] text-ink-3">
              {[person?.relation, person?.org_name].filter(Boolean).join(' · ')}
            </span>
            {event.record_type ? (
              <span className="rounded-full bg-fill px-2 py-[3px] text-[11px] leading-4 text-ink-2">
                {event.record_type}
              </span>
            ) : null}
            {event.extraction_status === 'pending' ? (
              <span className="text-[11px] text-ink-4">提取中…</span>
            ) : null}
            {event.extraction_status === 'failed' ? (
              <span className="text-[11px] font-medium text-destructive">提取失败</span>
            ) : null}
          </div>
          <Link to={`/events/${event.id}`} className="mt-1.5 block">
            <p className="line-clamp-2 text-[13.5px] leading-[1.75] text-ink-2 transition-colors group-hover:text-foreground">
              {headline || '（原文待提取）'}
            </p>
          </Link>
          {event.promises.length > 0 || event.my_feeling ? (
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1.5">
              {event.promises.map((promise, index) => (
                <span
                  key={index}
                  className="inline-flex items-center gap-1.5 rounded-full bg-fill px-2.5 py-[3px] text-[11.5px] text-ink-2"
                >
                  <span className="font-medium text-foreground">承诺</span>
                  {promise.who || '对方'}：{promise.what}
                  {promise.deadline ? `（${promise.deadline}）` : ''}
                </span>
              ))}
              {event.my_feeling ? (
                <span className="text-[11.5px] text-ink-3">感受：{event.my_feeling}</span>
              ) : null}
            </div>
          ) : null}
        </div>
        <span className="shrink-0 pt-0.5 text-[11.5px] tabular-nums text-ink-4">
          {event.event_date.slice(5, 10).replace('-', '/')}
        </span>
      </div>
    </li>
  );
}

/** Compact rail row with a status dot: green = recently in touch, orange = fading. */
function PersonRow({ person }: { person: PersonWithActivity }) {
  const days = daysSince(person.last_event_date);
  const stale = days === null || days >= STALE_DAYS;
  return (
    <li>
      <Link
        to={`/persons/${person.id}`}
        className="flex items-center gap-3 rounded-md px-3 py-2.5 transition-colors hover:bg-hover"
      >
        <span className="relative">
          <Avatar name={person.name} id={person.id} size="sm" />
          <span
            aria-hidden
            className={cn(
              'absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full ring-2 ring-card',
              stale ? 'bg-heat' : 'bg-live',
            )}
          />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] font-medium text-foreground">{person.name}</span>
          <span className="block truncate text-[12px] text-ink-3">
            {[person.relation, person.org_name].filter(Boolean).join(' · ') || '未分类'}
          </span>
        </span>
        <span
          className={cn(
            'shrink-0 text-[11.5px]',
            stale ? 'font-medium text-heat-text' : 'text-ink-3',
          )}
        >
          {days === null ? '从未记录' : days === 0 ? '今天' : days === 1 ? '昨天' : `${days} 天前`}
        </span>
      </Link>
    </li>
  );
}

/** The landing page: a quiet hero, a day-grouped story of recent records on one
 *  continuous surface, and a contacts rail that puts "people you are losing"
 *  before "people you just saw". No cumulative counters — they answer nobody's
 *  question. */
export default function Home() {
  const [events, setEvents] = useState<Event[]>([]);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [weekTotal, setWeekTotal] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let alive = true;
    const weekAgo = (() => {
      const d = new Date();
      d.setDate(d.getDate() - 7);
      return d.toISOString().slice(0, 10);
    })();
    // The third call only exists for its total: "how much happened this week"
    // without shipping a second list of rows.
    Promise.all([eventApi.list({ limit: 12 }), personApi.list(), eventApi.list({ from: weekAgo, limit: 1 })])
      .then(([eventPage, personList, weekPage]) => {
        if (!alive) return;
        setEvents(eventPage.data);
        setPersons(personList);
        setWeekTotal(weekPage.total);
      })
      .catch((e: unknown) => {
        if (alive) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, []);

  const groups = useMemo(() => groupByDay(events), [events]);
  const { fading, recent } = useMemo(() => {
    const fading: PersonWithActivity[] = [];
    const recent: PersonWithActivity[] = [];
    for (const person of persons) {
      const days = daysSince(person.last_event_date);
      (days === null || days >= STALE_DAYS ? fading : recent).push(person);
    }
    return { fading, recent };
  }, [persons]);
  const newcomers = useMemo(
    () => [...persons].sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? '')).slice(0, 3),
    [persons],
  );

  if (loading) {
    return <Spinner label="汇总最近动态…" />;
  }
  if (error && events.length === 0 && persons.length === 0) {
    return <ErrorNote>{error}</ErrorNote>;
  }

  const now = new Date();
  const openRecord = () => window.dispatchEvent(new CustomEvent(QUICK_RECORD_EVENT));

  return (
    <div>
      {/* Hero: the date, the one number that matters, one action. No card, no
          gradient — the type carries it. */}
      <section className="mb-10">
        <p className="text-[11.5px] font-semibold uppercase tracking-[0.14em] text-ink-3">
          {now.getMonth() + 1} 月 {now.getDate()} 日 · 周{WEEKDAYS[now.getDay()]}
        </p>
        <h1 className="mt-4 text-[clamp(28px,5vw,42px)] font-bold leading-[1.08] tracking-[-0.03em] text-foreground">
          今天
        </h1>
        <p className="mt-3.5 max-w-[52ch] text-[15px] leading-[1.7] text-muted-foreground">
          {weekTotal === null
            ? '看看最近发生了什么'
            : weekTotal > 0
              ? `本周记了 ${weekTotal} 条 · 联系 ${recent.filter((p) => {
                  const days = daysSince(p.last_event_date);
                  return days !== null && days < 7;
                }).length} 个人`
              : '本周还没有记录，从一个瞬间开始'}
        </p>
        <div className="mt-6">
          <Button size="lg" onClick={openRecord}>
            <NotebookPen className="size-4" />
            记一笔
          </Button>
        </div>
      </section>

      {error ? (
        <div className="mb-6">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      <div className="grid gap-9 lg:grid-cols-[minmax(0,1fr)_290px]">
        {/* Recent records as a day-grouped story on one continuous surface. */}
        <div>
          {events.length === 0 ? (
            <EmptyState
              icon={<NotebookPen className="size-6" />}
              title="还没有记录"
              description="记下今天和某个人的一个瞬间，AI 会帮你沉淀成画像。"
              action={
                <Button onClick={openRecord}>
                  <NotebookPen className="size-4" />
                  记第一笔
                </Button>
              }
            />
          ) : (
            groups.map((group) => (
              <section key={group.date} className="mb-7">
                <h2 className="mb-2.5 px-[18px] text-[11.5px] font-semibold uppercase tracking-[0.14em] text-ink-3">
                  {dayLabel(group.date)}
                </h2>
                <div className="panel">
                  <ul className="panel-rows">
                    {group.items.map((event) => (
                      <RecordRow key={event.id} event={event} persons={persons} />
                    ))}
                  </ul>
                </div>
              </section>
            ))
          )}
          {events.length > 0 ? (
            <Link
              to="/events"
              className="inline-flex items-center gap-1 text-[13.5px] text-ink-3 transition-colors hover:text-primary"
            >
              查看全部记录 →
            </Link>
          ) : null}
        </div>

        {/* Contacts rail: the actionable half first. */}
        <aside className="space-y-9">
          {fading.length > 0 ? (
            <section>
              <h2 className="mb-2.5 px-[18px] text-[11.5px] font-semibold uppercase tracking-[0.14em] text-heat-text">
                该联系了
              </h2>
              <div className="panel p-2">
                <ul>
                  {fading.slice(0, 5).map((person) => (
                    <PersonRow key={person.id} person={person} />
                  ))}
                </ul>
              </div>
            </section>
          ) : null}

          <section>
            <h2 className="mb-2.5 px-[18px] text-[11.5px] font-semibold uppercase tracking-[0.14em] text-ink-3">
              最近联系
            </h2>
            {recent.length === 0 ? (
              <EmptyState
                title="还没有人物"
                description="先去建一位人物，记录才有归属。"
                action={
                  <Link
                    to="/organizations"
                    className="inline-flex h-9 items-center rounded-full border border-hairline bg-card px-4 text-[13px] font-medium text-foreground shadow-xs transition-colors hover:bg-hover"
                  >
                    新建人物
                  </Link>
                }
              />
            ) : (
              <div className="panel p-2">
                <ul>
                  {recent.slice(0, 5).map((person) => (
                    <PersonRow key={person.id} person={person} />
                  ))}
                </ul>
              </div>
            )}
          </section>

          {newcomers.length > 0 ? (
            <section>
              <h2 className="mb-2.5 px-[18px] text-[11.5px] font-semibold uppercase tracking-[0.14em] text-ink-3">
                新面孔
              </h2>
              <div className="panel p-2">
                <ul>
                  {newcomers.map((person) => (
                    <li key={person.id}>
                      <Link
                        to={`/persons/${person.id}`}
                        className="flex items-center gap-2.5 rounded-md px-3 py-2 transition-colors hover:bg-hover"
                      >
                        <Avatar name={person.name} id={person.id} size="sm" className="size-6 text-[10px]" />
                        <span className="min-w-0 flex-1 truncate text-[13.5px] text-foreground">
                          {person.name}
                        </span>
                        <span className="shrink-0 text-[11px] text-ink-4">
                          {person.created_at ? relativeTime(person.created_at.slice(0, 10)) : ''}
                        </span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            </section>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
