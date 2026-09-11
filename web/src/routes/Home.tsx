import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { AlertTriangle, CalendarClock, Inbox, NotebookPen, Star, UserPlus, Users } from 'lucide-react';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import { Avatar, EmptyState, Field, PageHeader, SectionCard, StatTile, Toolbar } from '../components/layout';
import { ErrorNote, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { eventApi, followUpApi, organizationApi, personApi } from '../api/client';
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

const RELATIONS = ['朋友', '同事', '家人', '上级', '下属', '客户', '其他'];

function today() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
}

function daysAgo(days: number) {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

/** Importance is 1–5; show it as a compact star cluster instead of a raw number. */
function ImportanceDots({ value }: { value: number }) {
  if (value <= 0) return null;
  const filled = Math.max(1, Math.min(5, Math.round(value)));
  return (
    <span className="inline-flex items-center gap-px" title={`重要程度 ${filled}/5`}>
      {Array.from({ length: filled }, (_, index) => (
        <Star key={index} className="size-3 fill-amber-400 text-amber-400" />
      ))}
    </span>
  );
}

export default function Home() {
  const navigate = useNavigate();
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [total, setTotal] = useState(0);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [selected, setSelected] = useState<string>('');
  const [events, setEvents] = useState<Event[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadingTimeline, setLoadingTimeline] = useState(false);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [orgFilter, setOrgFilter] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<PersonSort>('recent');
  const [hasMore, setHasMore] = useState(false);
  const [draft, setDraft] = useState({ name: '', relation: '朋友', position: '', org_id: '' });
  const [stats, setStats] = useState({ weekEvents: 0, openFollowUps: 0, overdue: 0 });

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
        const result = await personApi.listPaged({ ...queryParams, limit: PAGE_SIZE + 1, offset });
        const page = result.data.slice(0, PAGE_SIZE);
        setHasMore(result.data.length > PAGE_SIZE);
        // The count is the filtered match count, not the size of the page.
        if (result.total > 0) setTotal(result.total);
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

  // Summary strip: three counts that change the plan for the day. Cheap
  // queries, and a failure here must not blank the page.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [recent, followUps] = await Promise.all([
          eventApi.list({ from: daysAgo(7), limit: 1 }),
          followUpApi.list(),
        ]);
        if (cancelled) return;
        const open = followUps.filter((item) => item.status === 'pending');
        const now = today();
        setStats({
          weekEvents: recent.total,
          openFollowUps: open.length,
          overdue: open.filter((item) => Boolean(item.due_date) && item.due_date < now).length,
        });
      } catch {
        // Stats are advisory; the page still works without them.
      }
    })();
    return () => {
      cancelled = true;
    };
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
      setShowCreate(false);
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
      <PageHeader
        title="人物"
        description={total > 0 ? `共 ${total} 位` : '按最近记录排序，先记一笔再回来看'}
        actions={
          <Button variant={showCreate ? 'secondary' : 'outline'} onClick={() => setShowCreate((v) => !v)}>
            <UserPlus className="size-4" />
            新建人物
          </Button>
        }
      />

      {error ? (
        <div className="mb-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      {/* The four numbers that decide what to do today. */}
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile label="人物" value={total} icon={<Users className="size-3.5" />} />
        <StatTile
          label="近 7 天记录"
          value={stats.weekEvents}
          icon={<NotebookPen className="size-3.5" />}
          hint="新写的记录条数"
        />
        <StatTile
          label="待跟进"
          value={stats.openFollowUps}
          icon={<Inbox className="size-3.5" />}
          hint="未完成的事项"
        />
        <StatTile
          label="已逾期"
          value={stats.overdue}
          icon={stats.overdue > 0 ? <AlertTriangle className="size-3.5" /> : <CalendarClock className="size-3.5" />}
          tone={stats.overdue > 0 ? 'danger' : 'default'}
          hint={stats.overdue > 0 ? '该联系了' : '没有拖欠'}
        />
      </div>

      <div className="flex flex-col gap-5 lg:flex-row">
        <aside className="w-full shrink-0 lg:w-[19rem]">
          <SectionCard
            title="人物列表"
            bodyClassName="p-3"
            actions={
              <select
                value={sort}
                onChange={(e) => setSort(e.target.value as PersonSort)}
                className={`${controlClass} h-8 w-auto py-0 text-xs text-muted-foreground`}
                title="排序方式"
              >
                {SORT_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            }
          >
            <Toolbar className="mb-3">
              <input
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                placeholder="搜索姓名、关系、职位、备注…"
                className={`${controlClass} h-9 min-w-0 flex-1 basis-full`}
              />
              <select
                value={orgFilter}
                onChange={(e) => setOrgFilter(e.target.value)}
                className={`${controlClass} h-9 min-w-0 flex-1 text-muted-foreground`}
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
            </Toolbar>

            {showCreate ? (
              <div className="mb-3 space-y-2 rounded-lg border border-dashed border-border bg-muted/40 p-3">
                <Field label="姓名">
                  <input
                    autoFocus
                    value={draft.name}
                    onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void createPerson();
                    }}
                    placeholder="必填"
                    className={`${controlClass} h-9`}
                  />
                </Field>
                <div className="grid grid-cols-2 gap-2">
                  <Field label="关系">
                    <select
                      value={draft.relation}
                      onChange={(e) => setDraft({ ...draft, relation: e.target.value })}
                      className={`${controlClass} h-9 text-muted-foreground`}
                    >
                      {RELATIONS.map((relation) => (
                        <option key={relation} value={relation}>
                          {relation}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <Field label="职位">
                    <input
                      value={draft.position}
                      onChange={(e) => setDraft({ ...draft, position: e.target.value })}
                      placeholder="可选"
                      className={`${controlClass} h-9`}
                    />
                  </Field>
                </div>
                <Field label="组织">
                  <select
                    value={draft.org_id}
                    onChange={(e) => setDraft({ ...draft, org_id: e.target.value })}
                    className={`${controlClass} h-9 text-muted-foreground`}
                  >
                    <option value="">无组织</option>
                    {organizations.map((org) => (
                      <option key={org.id} value={org.id}>
                        {org.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <div className="flex justify-end gap-2 pt-1">
                  <Button variant="ghost" size="sm" onClick={() => setShowCreate(false)}>
                    取消
                  </Button>
                  <Button size="sm" onClick={() => void createPerson()} disabled={creating || !draft.name.trim()}>
                    创建
                  </Button>
                </div>
              </div>
            ) : null}

            {loadingList ? (
              <Spinner label="载入人物…" />
            ) : persons.length === 0 ? (
              <EmptyState
                icon={<Users className="size-6" />}
                title={search || orgFilter ? '没有匹配的人物' : '还没有人物'}
                description={
                  search || orgFilter ? '换个关键词或组织试试。' : '新建一位人物，之后就能为 TA 记下每次接触。'
                }
                action={
                  search || orgFilter ? null : (
                    <Button size="sm" onClick={() => setShowCreate(true)}>
                      新建人物
                    </Button>
                  )
                }
              />
            ) : (
              <>
                <ul className="space-y-0.5">
                  {persons.map((person) => (
                    <li key={person.id}>
                      <button
                        onClick={() => setSelected(person.id)}
                        className={`flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left transition-colors ${
                          selected === person.id ? 'bg-primary/10 ring-1 ring-primary/30' : 'hover:bg-muted'
                        }`}
                      >
                        <Avatar name={person.name} id={person.id} size="md" />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-1.5">
                            <span className="truncate text-sm font-medium text-foreground">{person.name}</span>
                            <ImportanceDots value={person.importance} />
                          </div>
                          <div className="truncate text-xs text-muted-foreground">
                            {[person.relation || '未分类', [person.org_name, person.position].filter(Boolean).join(' · ')]
                              .filter(Boolean)
                              .join(' · ') || '未归属'}
                          </div>
                        </div>
                        <div className="shrink-0 text-right">
                          {/* 最近记录是最后一次有记录的日期；没有记录就明说，不拿创建时间冒充联系时间。 */}
                          <div
                            className="text-xs text-muted-foreground"
                            title={person.last_event_date ? `最近记录 ${fullDate(person.last_event_date)}` : undefined}
                          >
                            {person.last_event_date ? relativeTime(person.last_event_date) : '尚无记录'}
                          </div>
                          <div className="text-[11px] text-muted-foreground/80">{person.event_count} 条</div>
                        </div>
                      </button>
                    </li>
                  ))}
                </ul>
                {hasMore ? (
                  <Button onClick={() => void loadMore()} disabled={loadingMore} variant="outline" className="mt-3 w-full">
                    {loadingMore ? '载入中…' : '加载更多'}
                  </Button>
                ) : null}
              </>
            )}
          </SectionCard>
        </aside>

        <section className="min-w-0 flex-1 space-y-5">
          {!current ? (
            <EmptyState
              className="py-20"
              icon={<Users className="size-7" />}
              title="选择或新建一位人物"
              description="选中的人会出现在这里，可以直接写记录、看时间线，或去提问该怎么相处。"
            />
          ) : (
            <>
              {/* Headline for the selected person: who they are and where to go next. */}
              <div className="flex flex-wrap items-center gap-4 rounded-xl border border-border bg-card p-4 shadow-sm">
                <Avatar name={current.name} id={current.id} size="lg" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Link
                      to={`/persons/${current.id}`}
                      className="truncate text-lg font-semibold text-foreground hover:text-primary"
                    >
                      {current.name}
                    </Link>
                    <ImportanceDots value={current.importance} />
                    {current.relation ? (
                      <span className="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">
                        {current.relation}
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-0.5 truncate text-sm text-muted-foreground">
                    {[current.org_name, current.position].filter(Boolean).join(' · ') || '未填写组织与职位'}
                  </p>
                </div>

                <div className="flex items-center gap-5 text-center">
                  <div>
                    <div className="text-xl font-semibold tabular-nums text-foreground">{current.event_count}</div>
                    <div className="text-xs text-muted-foreground">记录</div>
                  </div>
                  <div>
                    <div className="text-sm font-medium text-foreground">
                      {current.last_event_date ? relativeTime(current.last_event_date) : '—'}
                    </div>
                    <div className="text-xs text-muted-foreground">最近记录</div>
                  </div>
                </div>

                <div className="flex shrink-0 gap-2">
                  <Link
                    to={`/persons/${current.id}`}
                    className="rounded-lg border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-primary hover:text-primary"
                  >
                    详情
                  </Link>
                  <Button variant="outline" onClick={() => navigate('/advice', { state: { personId: current.id } })}>
                    去提问
                  </Button>
                </div>
              </div>

              <QuickRecord
                personId={current.id}
                onRecorded={(event) => void handleCreated(event)}
                onAsk={() => navigate('/advice', { state: { personId: current.id } })}
              />

              <SectionCard
                title="时间线"
                description="只显示以 TA 为主角或参与人的记录"
                actions={loadingTimeline ? <Spinner /> : null}
              >
                <EventTimeline events={events} />
              </SectionCard>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
