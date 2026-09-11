import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { RefreshCw } from 'lucide-react';
import { controlClass, EmptyState, ErrorNote, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { EventStatusBadge } from '../components/EventStatus';
import { eventApi, personApi } from '../api/client';
import type { Event, EventListParams } from '../api/types';
import { eventHeadline, fullDate, relativeTime } from '../format';
import type { PersonWithActivity } from '../api/types';

const PAGE_SIZE = 20;

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'succeeded', label: '已提取' },
  { value: 'pending', label: '待提取' },
  { value: 'failed', label: '提取失败' },
];

export default function Events() {
  const [events, setEvents] = useState<Event[]>([]);
  const [total, setTotal] = useState(0);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);

  const [searchInput, setSearchInput] = useState('');
  const [filters, setFilters] = useState<EventListParams>({});
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [retrying, setRetrying] = useState('');

  // The search box debounces into the filter; every other control applies at once.
  const debounced = useRef<number | undefined>(undefined);
  useEffect(() => {
    window.clearTimeout(debounced.current);
    debounced.current = window.setTimeout(() => {
      setFilters((prev) => ({ ...prev, q: searchInput.trim() }));
    }, 300);
    return () => window.clearTimeout(debounced.current);
  }, [searchInput]);

  const load = useCallback(async (offset: number) => {
    const appending = offset > 0;
    if (appending) setLoadingMore(true);
    else setLoading(true);
    try {
      const { data, total: matched } = await eventApi.list({ ...filters, limit: PAGE_SIZE, offset });
      setEvents((prev) => (appending ? [...prev, ...data] : data));
      setTotal(matched);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, [filters]);

  useEffect(() => {
    void load(0);
  }, [load]);

  useEffect(() => {
    personApi
      .list()
      .then(setPersons)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const retry = async (event: Event) => {
    // A hand-curated result must not be replaced by a click that looked like a
    // plain retry, so the confirmation is what turns force on.
    const force = event.manually_edited === 1;
    if (force && !window.confirm('这条记录的提取结果你手动改过，重新提取会覆盖你的修改。继续？')) return;
    setRetrying(event.id);
    try {
      const result = await eventApi.retryExtract(event.id, force);
      setEvents((prev) => prev.map((item) => (item.id === event.id ? result.event : item)));
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setRetrying('');
    }
  };

  const hasMore = events.length < total;
  const filtered = Object.values(filters).some((value) => value !== undefined && value !== '');

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-lg font-semibold text-foreground">记录</h1>
        {!loading && total > 0 ? (
          <span className="text-xs text-muted-foreground">共 {total} 条，已显示 {events.length} 条</span>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <input
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="搜索原文或摘要…"
          className={`${controlClass} w-full sm:w-64`}
        />
        <select
          value={filters.person_id ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, person_id: e.target.value }))}
          className={`${controlClass} w-auto text-muted-foreground`}
          title="按人物筛选（含参与人）"
        >
          <option value="">全部人物</option>
          {persons.map((person) => (
            <option key={person.id} value={person.id}>
              {person.name}
            </option>
          ))}
        </select>
        <select
          value={filters.status ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, status: e.target.value }))}
          className={`${controlClass} w-auto text-muted-foreground`}
          title="按提取状态筛选"
        >
          {STATUS_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1">
          <input
            type="date"
            value={filters.from ?? ''}
            onChange={(e) => setFilters((prev) => ({ ...prev, from: e.target.value }))}
            className={`${controlClass} h-9 w-36`}
            title="起始日期"
          />
          <span className="text-xs text-muted-foreground">至</span>
          <input
            type="date"
            value={filters.to ?? ''}
            onChange={(e) => setFilters((prev) => ({ ...prev, to: e.target.value }))}
            className={`${controlClass} h-9 w-36`}
            title="结束日期"
          />
        </div>
        {filtered ? (
          <Button
            variant="ghost"
            onClick={() => {
              setSearchInput('');
              setFilters({});
            }}
            className="h-9 text-muted-foreground"
          >
            清空筛选
          </Button>
        ) : null}
      </div>

      {error ? <ErrorNote>{error}</ErrorNote> : null}

      {loading ? (
        <Spinner label="载入记录…" />
      ) : events.length === 0 ? (
        <EmptyState>{filtered ? '没有匹配的记录，换个条件试试。' : '还没有记录，在首页或人物页写一条。'}</EmptyState>
      ) : (
        <>
          <ul className="space-y-2">
            {events.map((event) => (
              <li
                key={event.id}
                className="rounded-xl border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary/50"
              >
                <div className="flex items-start justify-between gap-3">
                  <Link to={`/events/${event.id}`} className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="font-mono">{fullDate(event.event_date)}</span>
                      <span>{relativeTime(event.event_date)}</span>
                      {event.record_type ? <span>· {event.record_type}</span> : null}
                      {event.channel ? <span>· {event.channel}</span> : null}
                    </div>
                    <p className="mt-1 truncate text-sm text-foreground">
                      {eventHeadline(event.summary, event.raw_text)}
                    </p>
                    {event.participants.length > 0 ? (
                      <p className="mt-1 truncate text-xs text-muted-foreground">
                        参与：
                        {event.participants
                          .map((p) => (p.role && p.role !== 'primary' ? `${p.person_name ?? ''}（${p.role}）` : (p.person_name ?? '')))
                          .join('、')}
                      </p>
                    ) : null}
                    {event.extraction_status === 'failed' && event.extraction_error ? (
                      <p className="mt-1 truncate text-xs text-red-600 dark:text-red-400">{event.extraction_error}</p>
                    ) : null}
                  </Link>

                  <div className="flex shrink-0 flex-col items-end gap-2">
                    <EventStatusBadge event={event} />
                    <Button
                      variant="outline"
                      className="h-7 px-2 text-xs text-muted-foreground"
                      disabled={retrying === event.id}
                      onClick={() => void retry(event)}
                      title={event.extraction_status === 'succeeded' ? '重新跑一次提取' : '重试提取'}
                    >
                      <RefreshCw className={`mr-1 size-3 ${retrying === event.id ? 'animate-spin' : ''}`} />
                      重新提取
                    </Button>
                  </div>
                </div>
              </li>
            ))}
          </ul>

          {hasMore ? (
            <Button variant="outline" onClick={() => void load(events.length)} disabled={loadingMore} className="w-full">
              {loadingMore ? '载入中…' : `加载更多（还有 ${total - events.length} 条）`}
            </Button>
          ) : null}
        </>
      )}
    </div>
  );
}
