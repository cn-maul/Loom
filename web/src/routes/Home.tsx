import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { NotebookPen, Sparkles, Users } from 'lucide-react';
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

/** One record row. Person first, what happened second, AI extraction as quiet
 *  chips — the text is the protagonist, not the metadata. */
function RecordRow({ event, persons }: { event: Event; persons: PersonWithActivity[] }) {
  const person = persons.find((p) => p.id === event.person_id);
  const headline = eventHeadline(event.summary, event.raw_text);
  return (
    <li className="group relative py-3.5">
      <div className="flex gap-3.5">
        {person ? (
          <Link to={`/persons/${person.id}`} className="shrink-0" title={person.name}>
            <Avatar name={person.name} id={person.id} size="md" />
          </Link>
        ) : (
          <span className="grid size-9 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground">
            <NotebookPen className="size-4" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            {person ? (
              <Link to={`/persons/${person.id}`} className="text-sm font-medium text-foreground hover:text-primary">
                {person.name}
              </Link>
            ) : (
              <span className="text-sm font-medium text-muted-foreground">未关联人物</span>
            )}
            <span className="truncate text-xs text-muted-foreground">
              {[person?.relation, person?.org_name].filter(Boolean).join(' · ')}
            </span>
            {event.record_type ? (
              <span className="rounded-full bg-secondary px-2 py-0.5 text-[11px] leading-4 text-secondary-foreground">{event.record_type}</span>
            ) : null}
            {event.extraction_status === 'pending' ? (
              <span className="text-[11px] text-muted-foreground/70">提取中…</span>
            ) : null}
            {event.extraction_status === 'failed' ? (
              <span className="text-[11px] text-red-600 dark:text-red-400">提取失败</span>
            ) : null}
          </div>
          <Link to={`/events/${event.id}`} className="mt-1 block">
            <p className="line-clamp-2 text-[13.5px] leading-6 text-foreground/90 transition-colors group-hover:text-primary">
              {headline || '（原文待提取）'}
            </p>
          </Link>
          {(event.promises.length > 0 || event.my_feeling) && (
            <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
              {event.promises.map((promise, index) => (
                <span key={index} className="inline-flex items-center gap-1 text-[11px] text-primary">
                  <span className="size-1 rounded-full bg-primary/70" />
                  {promise.who || '对方'}：{promise.what}
                  {promise.deadline ? `（${promise.deadline}）` : ''}
                </span>
              ))}
              {event.my_feeling ? <span className="text-[11px] text-muted-foreground">感受：{event.my_feeling}</span> : null}
            </div>
          )}
        </div>
        <span className="shrink-0 pt-1 text-[11px] tabular-nums text-muted-foreground/70">
          {event.event_date.slice(5, 10).replace('-', '/')}
        </span>
      </div>
    </li>
  );
}

/** Right-rail person row with a status dot: green = recent, amber = fading. */
function PersonRow({ person }: { person: PersonWithActivity }) {
  const days = daysSince(person.last_event_date);
  const stale = days === null || days >= STALE_DAYS;
  return (
    <li>
      <Link to={`/persons/${person.id}`} className="flex items-center gap-3 rounded-xl px-2.5 py-2 transition-colors hover:bg-muted">
        <span className="relative">
          <Avatar name={person.name} id={person.id} size="sm" />
          <span
            aria-hidden
            className={cn(
              'absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full ring-2 ring-card',
              stale ? 'bg-amber-400' : 'bg-emerald-400',
            )}
          />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium text-foreground">{person.name}</span>
          <span className="block truncate text-xs text-muted-foreground">
            {[person.relation, person.org_name].filter(Boolean).join(' · ') || '未分类'}
          </span>
        </span>
        <span className={cn('shrink-0 text-[11px]', stale ? 'font-medium text-amber-600 dark:text-amber-400' : 'text-muted-foreground')}>
          {days === null ? '从未记录' : days === 0 ? '今天' : days === 1 ? '昨天' : `${days} 天前`}
        </span>
      </Link>
    </li>
  );
}

/** The landing page: a quiet hero, a day-grouped story of recent records, and
 *  a contacts rail that puts "people you are losing" before "people you just
 *  saw". No cumulative counters — they answer nobody's question. */
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
    <div className="mx-auto max-w-5xl">
      {/* Hero: greeting, the one number that matters, one action. */}
      <div className="mb-8 overflow-hidden rounded-2xl border border-border bg-gradient-to-br from-primary/10 via-card to-card p-7 shadow-sm">
        <p className="text-xs font-medium uppercase tracking-widest text-primary/80">
          {now.getMonth() + 1} 月 {now.getDate()} 日 · 周{WEEKDAYS[now.getDay()]}
        </p>
        <div className="mt-2 flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">今天</h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              {weekTotal === null
                ? '看看最近发生了什么'
                : weekTotal > 0
                  ? `本周记了 ${weekTotal} 条 · 联系 ${recent.filter((p) => {
                      const days = daysSince(p.last_event_date);
                      return days !== null && days < 7;
                    }).length} 个人`
                  : '本周还没有记录，从一个瞬间开始'}
            </p>
          </div>
          <Button size="lg" onClick={openRecord} className="shadow-md shadow-primary/20">
            <NotebookPen className="size-4" />
            记一笔
          </Button>
        </div>
      </div>

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      <div className="grid gap-8 lg:grid-cols-[1fr_300px]">
        {/* Recent records as a day-grouped story. */}
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
              <section key={group.date} className="mb-6">
                <h2 className="mb-1 flex items-center gap-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {dayLabel(group.date)}
                  <span className="h-px flex-1 bg-border" />
                </h2>
                <ul className="divide-y divide-border/60">
                  {group.items.map((event) => (
                    <RecordRow key={event.id} event={event} persons={persons} />
                  ))}
                </ul>
              </section>
            ))
          )}
          {events.length > 0 ? (
            <Link
              to="/events"
              className="mt-2 inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-primary"
            >
              查看全部记录 →
            </Link>
          ) : null}
        </div>

        {/* Contacts rail: the actionable half first. */}
        <aside className="space-y-6">
          {fading.length > 0 ? (
            <div>
              <h2 className="mb-1.5 px-1 text-xs font-semibold uppercase tracking-wider text-amber-600 dark:text-amber-400">
                该联系了
              </h2>
              <ul className="-mx-1">
                {fading.slice(0, 5).map((person) => (
                  <PersonRow key={person.id} person={person} />
                ))}
              </ul>
            </div>
          ) : null}

          <div>
            <h2 className="mb-1.5 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              最近联系
            </h2>
            {recent.length === 0 ? (
              <EmptyState
                icon={<Users className="size-5" />}
                title="还没有人物"
                description="先去建一位人物，记录才有归属。"
                action={
                  <Link
                    to="/organizations"
                    className="rounded-lg border border-border px-3 py-2 text-sm text-foreground transition-colors hover:border-primary hover:text-primary"
                  >
                    新建人物
                  </Link>
                }
              />
            ) : (
              <ul className="-mx-1">
                {recent.slice(0, 5).map((person) => (
                  <PersonRow key={person.id} person={person} />
                ))}
              </ul>
            )}
          </div>

          {newcomers.length > 0 ? (
            <div className="rounded-xl border border-dashed border-border p-3">
              <h2 className="mb-1 flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <Sparkles className="size-3" />
                新面孔
              </h2>
              <ul className="-mx-1">
                {newcomers.map((person) => (
                  <li key={person.id}>
                    <Link to={`/persons/${person.id}`} className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 transition-colors hover:bg-muted">
                      <Avatar name={person.name} id={person.id} size="sm" className="size-6 text-[10px]" />
                      <span className="min-w-0 flex-1 truncate text-sm text-foreground">{person.name}</span>
                      <span className="shrink-0 text-[11px] text-muted-foreground">
                        {person.created_at ? relativeTime(person.created_at.slice(0, 10)) : ''}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
