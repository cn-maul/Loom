import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import { ErrorNote, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { organizationApi, personApi } from '../api/client';
import type { Event, Organization, PersonListParams, PersonWithActivity } from '../api/types';
import { fullDate, relativeTime } from '../format';

const NO_ORG = '__none__';
const PAGE_SIZE = 30;

type PersonSort = NonNullable<PersonListParams['sort']>;

const SORT_OPTIONS: { value: PersonSort; label: string }[] = [
  { value: 'recent', label: '最近记录' },
  { value: 'importance', label: '重要程度' },
  { value: 'name', label: '姓名' },
  { value: 'created', label: '新建时间' },
];

export default function Home() {
  const navigate = useNavigate();
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selected, setSelected] = useState<string>('');
  const [events, setEvents] = useState<Event[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadingTimeline, setLoadingTimeline] = useState(false);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [orgFilter, setOrgFilter] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<PersonSort>('recent');
  const [hasMore, setHasMore] = useState(false);
  const [draft, setDraft] = useState({ name: '', relation: '朋友', position: '', org_id: '' });

  // Search is server-side, so the free-text box is debounced before it turns
  // into a request.
  useEffect(() => {
    const timer = setTimeout(() => setSearch(searchInput.trim()), 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  const queryParams = useMemo((): PersonListParams => {
    const params: PersonListParams = {};
    if (search) params.q = search;
    if (orgFilter === NO_ORG) params.org_id = 'none';
    else if (orgFilter) params.org_id = orgFilter;
    if (sort !== 'recent') params.sort = sort;
    return params;
  }, [search, orgFilter, sort]);

  const loadPersons = useCallback(
    async (offset: number, append: boolean) => {
      try {
        // One extra row beyond the page reveals whether a next page exists,
        // without a separate count request.
        const data = await personApi.list({ ...queryParams, limit: PAGE_SIZE + 1, offset });
        const page = data.slice(0, PAGE_SIZE);
        setHasMore(data.length > PAGE_SIZE);
        setPersons((current) => (append ? [...current, ...page] : page));
        if (!append) setSelected((current) => current || page[0]?.id || '');
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [queryParams],
  );

  // First load and every filter change restart from the first page.
  useEffect(() => {
    setLoadingList(true);
    void loadPersons(0, false).finally(() => setLoadingList(false));
  }, [loadPersons]);

  useEffect(() => {
    organizationApi
      .list()
      .then(setOrganizations)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  useEffect(() => {
    if (persons.length > 0 && !persons.some((person) => person.id === selected)) {
      setSelected(persons[0].id);
    }
    if (persons.length === 0) setSelected('');
  }, [persons, selected]);

  useEffect(() => {
    if (!selected) {
      setEvents([]);
      return;
    }
    let cancelled = false;
    setLoadingTimeline(true);
    personApi
      .events(selected)
      .then((data) => {
        if (!cancelled) setEvents(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setLoadingTimeline(false);
      });
    return () => {
      cancelled = true;
    };
  }, [selected]);

  const handleCreated = async (event: Event) => {
    setEvents((current) => [event, ...current]);
    await loadPersons(0, false);
  };

  const createPerson = async () => {
    const name = draft.name.trim();
    if (!name || creating) return;
    setCreating(true);
    setError('');
    try {
      const person = await personApi.create({
        name,
        relation: draft.relation,
        importance: 3,
        position: draft.position.trim(),
        org_id: draft.org_id,
      });
      setDraft({ name: '', relation: '朋友', position: '', org_id: '' });
      await loadPersons(0, false);
      setSelected(person.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const loadMore = async () => {
    setLoadingMore(true);
    setError('');
    try {
      await loadPersons(persons.length, true);
    } finally {
      setLoadingMore(false);
    }
  };

  const current = persons.find((person) => person.id === selected) ?? null;

  return (
    <div>
      {error ? (
        <div className="mb-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="w-full shrink-0 rounded-xl border border-border bg-card p-4 shadow-sm lg:w-80">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">人物</h2>
            <select
              value={sort}
              onChange={(e) => setSort(e.target.value as PersonSort)}
              className={`${controlClass} h-auto w-auto px-2 py-1 text-xs text-muted-foreground`}
              title="排序方式"
            >
              {SORT_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>

          <div className="mb-4 space-y-2">
            <input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="搜索姓名、关系、职位、备注…"
              className={controlClass}
            />
            <select
              value={orgFilter}
              onChange={(e) => setOrgFilter(e.target.value)}
              className={`${controlClass} text-muted-foreground`}
              title="按组织筛选"
            >
              <option value="">全部组织</option>
              <option value={NO_ORG}>未归属</option>
              {organizations.map((org) => (
                <option key={org.id} value={org.id}>
                  {org.name}
                </option>
              ))}
            </select>
            <div className="flex gap-2">
              <select
                value={draft.relation}
                onChange={(e) => setDraft({ ...draft, relation: e.target.value })}
                className={`${controlClass} min-w-0 flex-1 text-muted-foreground`}
              >
                {['朋友', '同事', '家人', '上级', '下属', '客户', '其他'].map((relation) => (
                  <option key={relation} value={relation}>
                    {relation}
                  </option>
                ))}
              </select>
              <input
                value={draft.position}
                onChange={(e) => setDraft({ ...draft, position: e.target.value })}
                placeholder="职位（可选）"
                className={`${controlClass} min-w-0 flex-1`}
              />
            </div>
            <div className="flex gap-2">
              <select
                value={draft.org_id}
                onChange={(e) => setDraft({ ...draft, org_id: e.target.value })}
                className={`${controlClass} min-w-0 flex-1 text-muted-foreground`}
              >
                <option value="">无组织</option>
                {organizations.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
              <Button onClick={createPerson} disabled={creating || !draft.name.trim()} variant="outline">
                新建
              </Button>
            </div>
          </div>

          {loadingList ? (
            <Spinner label="载入人物…" />
          ) : persons.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {search || orgFilter ? '没有匹配的人物，换个条件试试。' : '还没有人物，先建一个。'}
            </p>
          ) : (
            <>
              <ul className="space-y-1">
                {persons.map((person) => (
                  <li key={person.id}>
                    <button
                      onClick={() => setSelected(person.id)}
                      className={`w-full rounded-lg px-3 py-2 text-left transition-colors ${
                        selected === person.id ? 'bg-primary/10 ring-1 ring-primary' : 'hover:bg-muted'
                      }`}
                    >
                      <div className="flex items-baseline justify-between gap-2">
                        <span className="truncate text-sm font-medium text-foreground">{person.name}</span>
                        {/* 最近记录是最后一次有记录的日期；没有记录就明说，不拿创建时间冒充联系时间。 */}
                        <span
                          className="shrink-0 text-xs text-muted-foreground"
                          title={person.last_event_date ? `最近记录 ${fullDate(person.last_event_date)}` : undefined}
                        >
                          {person.last_event_date ? relativeTime(person.last_event_date) : '尚无记录'}
                        </span>
                      </div>
                      <div className="truncate text-xs text-muted-foreground">
                        {[person.relation || '未分类', [person.org_name, person.position].filter(Boolean).join('·') || '未归属'].join(
                          ' · ',
                        )}{' '}
                        · {person.event_count} 条记录
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
              {hasMore ? (
                <Button onClick={loadMore} disabled={loadingMore} variant="outline" className="mt-3 w-full">
                  {loadingMore ? '载入中…' : '加载更多'}
                </Button>
              ) : null}
            </>
          )}
        </aside>

        <section className="min-w-0 flex-1 space-y-4">
          {!current ? (
            <div className="rounded-xl border border-dashed border-border bg-card p-16 text-center text-sm text-muted-foreground">
              选择或新建一个人物开始记录。
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm text-muted-foreground">
                  <Link to={`/persons/${current.id}`} className="text-base font-semibold text-foreground hover:text-primary">
                    {current.name}
                  </Link>
                  <span className="ml-2">
                    {[current.org_name, current.position].filter(Boolean).join(' · ') || current.relation} · 上次联系{' '}
                    {current.last_event_date ? fullDate(current.last_event_date) : '暂无'}
                  </span>
                </div>
                <Link
                  to="/advice"
                  state={{ personId: current.id }}
                  className="rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-primary hover:text-primary"
                >
                  去提问
                </Link>
              </div>

              <QuickRecord
                personId={current.id}
                onRecorded={(event) => void handleCreated(event)}
                onAsk={() => navigate('/advice', { state: { personId: current.id } })}
              />

              <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-foreground">时间线</h3>
                  {loadingTimeline ? <Spinner /> : null}
                </div>
                <EventTimeline events={events} />
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
