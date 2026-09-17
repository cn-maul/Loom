import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { NotebookPen, RefreshCw } from 'lucide-react';
import { controlClass, ErrorNote, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { EmptyState, PageHeader, Toolbar } from '../components/layout';
import { eventApi, organizationApi, personApi } from '../api/client';
import type { Event, EventListParams, Organization, PersonWithActivity } from '../api/types';
import { eventHeadline, shortDate } from '../format';

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
  const [organizations, setOrganizations] = useState<Organization[]>([]);

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
    organizationApi
      .list()
      .then(setOrganizations)
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
  // 主人物不在 participants 里时按 id 回查名字（列表接口只回 person_id）。
  const personNames = new Map(persons.map((p) => [p.id, p.name]));
  const participantsLabel = (event: Event) => {
    const names = event.participants.map((p) => p.person_name ?? '').filter(Boolean);
    if (names.length > 0) return names.join('、');
    return personNames.get(event.person_id) ?? '';
  };

  const field = `${controlClass} h-9 text-[13px]`;

  return (
    <div>
      <PageHeader
        title={
          <span className="flex items-baseline gap-2.5">
            记录
            {!loading && total > 0 ? (
              <span className="whitespace-nowrap text-[13px] font-normal text-ink-3">
                共 {total} 条，已显示 {events.length} 条
              </span>
            ) : null}
          </span>
        }
      />

      {/* Filters on their own row: the header stays a header, and the controls
          wrap instead of fighting the title for width. */}
      <Toolbar className="mb-6">
        <input
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="搜索原文或摘要"
          className={`${field} w-44`}
        />
        <select
          value={filters.org_id ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, org_id: e.target.value }))}
          className={`${field} w-28 text-ink-2`}
          title="按组织筛选"
        >
          <option value="">全部组织</option>
          <option value="none">未归属</option>
          {organizations.map((org) => (
            <option key={org.id} value={org.id}>
              {org.name}
            </option>
          ))}
        </select>
        <select
          value={filters.person_id ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, person_id: e.target.value }))}
          className={`${field} w-28 text-ink-2`}
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
          className={`${field} w-24 text-ink-2`}
          title="按提取状态筛选"
        >
          {STATUS_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <input
          type="date"
          value={filters.from ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, from: e.target.value }))}
          className={`${field} w-36`}
          title="起始日期"
        />
        <span className="shrink-0 text-[13px] text-ink-3">至</span>
        <input
          type="date"
          value={filters.to ?? ''}
          onChange={(e) => setFilters((prev) => ({ ...prev, to: e.target.value }))}
          className={`${field} w-36`}
          title="结束日期"
        />
        {filtered ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSearchInput('');
              setFilters({});
            }}
          >
            清空
          </Button>
        ) : null}
      </Toolbar>

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      {loading ? (
        <Spinner label="载入记录…" />
      ) : events.length === 0 ? (
        <EmptyState
          icon={<NotebookPen className="size-6" />}
          title={filtered ? '没有匹配的记录' : '还没有记录'}
          description={filtered ? '换个关键词、人物或时间范围试试。' : '在首页或人物页写一条，之后可以在这里检索和修正。'}
          action={
            filtered ? null : (
              <Link to="/" className="text-[13.5px] text-primary hover:underline">
                去首页写一条
              </Link>
            )
          }
        />
      ) : (
        <div className="panel">
          <div className="scroll-slim overflow-x-auto">
            <table className="w-full table-fixed text-left text-[13.5px]">
              <thead>
                <tr className="border-b border-hairline text-[12px] text-ink-3">
                  <th className="w-20 py-3 pl-5 pr-3.5 font-medium">日期</th>
                  <th className="w-36 px-3.5 py-3 font-medium">参与人</th>
                  <th className="px-3.5 py-3 font-medium">摘要</th>
                  <th className="px-3.5 py-3 font-medium">原文</th>
                  <th className="w-28 px-3.5 py-3 font-medium">AI 处理</th>
                  <th className="w-32 py-3 pl-3.5 pr-5 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {events.map((event) => {
                  const status =
                    event.extraction_status === 'succeeded'
                      ? event.pipeline_warnings.length > 0
                        ? { label: '已处理 · 有警告', cls: 'bg-heat-bg text-heat-text', tip: event.pipeline_warnings.join('\n') }
                        : { label: '已处理', cls: 'bg-live-bg text-live-text', tip: '' }
                      : event.extraction_status === 'failed'
                        ? { label: '失败', cls: 'bg-destructive/10 text-destructive', tip: event.extraction_error }
                        : { label: '待处理', cls: 'bg-fill text-ink-2', tip: '' };
                  return (
                    <tr
                      key={event.id}
                      className="border-b border-hairline transition-colors last:border-0 hover:bg-hover"
                    >
                      <td className="whitespace-nowrap py-3 pl-5 pr-3.5 tabular-nums text-ink-3">
                        {shortDate(event.event_date)}
                      </td>
                      <td className="px-3.5 py-3">
                        <div className="max-w-[10rem] truncate" title={participantsLabel(event)}>
                          {participantsLabel(event) || '—'}
                        </div>
                      </td>
                      <td className="px-3.5 py-3">
                        <Link
                          to={`/events/${event.id}`}
                          className="block max-w-[18rem] truncate text-foreground hover:text-primary"
                          title={eventHeadline(event.summary, event.raw_text)}
                        >
                          {eventHeadline(event.summary, event.raw_text) || '（无摘要）'}
                        </Link>
                        {event.manually_edited === 1 ? (
                          <span className="text-[12px] text-ink-3">已人工修订</span>
                        ) : null}
                      </td>
                      <td className="px-3.5 py-3 text-ink-3">
                        <div className="max-w-[13rem] truncate" title={event.raw_text}>
                          {event.raw_text || '—'}
                        </div>
                      </td>
                      <td className="px-3.5 py-3">
                        <span
                          className={`inline-block whitespace-nowrap rounded-full px-2.5 py-[3px] text-[11.5px] font-medium ${status.cls}`}
                          title={status.tip || undefined}
                        >
                          {status.label}
                        </span>
                      </td>
                      <td className="py-3 pl-3.5 pr-5 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-ink-3"
                          disabled={retrying === event.id}
                          onClick={() => void retry(event)}
                          title={event.extraction_status === 'succeeded' ? '重新跑一次提取' : '重试提取'}
                        >
                          <RefreshCw className={`size-3.5 ${retrying === event.id ? 'animate-spin' : ''}`} />
                          重新提取
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {hasMore ? (
        <Button
          variant="outline"
          onClick={() => void load(events.length)}
          disabled={loadingMore}
          className="mt-5 w-full"
        >
          {loadingMore ? '载入中…' : `加载更多（还有 ${total - events.length} 条）`}
        </Button>
      ) : null}
    </div>
  );
}
