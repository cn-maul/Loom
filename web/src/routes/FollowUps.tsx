import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { AlertTriangle, CalendarClock, CheckCircle2, ChevronDown, ChevronRight, Hourglass, Inbox, ListTodo } from 'lucide-react';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { Avatar, EmptyState, Field, PageHeader, SectionCard, StatTile } from '../components/layout';
import { followUpApi, personApi } from '../api/client';
import { FOLLOW_UP_STATUS_LABEL, type FollowUp, type FollowUpPostponement, type PersonWithActivity } from '../api/types';
import { fullDate, todayISO } from '../format';

const STATUS_OPTIONS = ['pending', 'waiting', 'completed', 'cancelled'] as const;

function statusBadgeClass(status: FollowUp['status']): string {
  switch (status) {
    case 'pending':
      return 'border-blue-300/60 bg-blue-50 text-blue-700 dark:border-blue-500/40 dark:bg-blue-950/40 dark:text-blue-300';
    case 'waiting':
      return 'border-amber-300/60 bg-amber-50 text-amber-800 dark:border-amber-500/40 dark:bg-amber-950/40 dark:text-amber-300';
    case 'completed':
      return 'border-emerald-300/60 bg-emerald-50 text-emerald-700 dark:border-emerald-500/40 dark:bg-emerald-950/40 dark:text-emerald-300';
    default:
      return 'border-border bg-muted text-muted-foreground';
  }
}

const OWNER_OPTIONS = ['我', '对方'];

function isoDate(offsetDays: number) {
  const date = new Date();
  date.setDate(date.getDate() + offsetDays);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

export default function FollowUps() {
  const location = useLocation();
  const initialPerson = (location.state as { personId?: string } | null)?.personId ?? '';

  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [personFilter, setPersonFilter] = useState(initialPerson);
  const [statusFilter, setStatusFilter] = useState('');
  const [items, setItems] = useState<FollowUp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');
  const [busyId, setBusyId] = useState('');

  // Per-item UI state: one open panel at a time is enough — these actions are
  // quick, and stacking four expansion areas on one card gets unreadable.
  const [panel, setPanel] = useState<{ id: string; kind: 'complete' | 'postpone' } | null>(null);
  const [historyFor, setHistoryFor] = useState('');
  const [history, setHistory] = useState<FollowUpPostponement[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const [note, setNote] = useState('');
  const [newDue, setNewDue] = useState('');
  const [reason, setReason] = useState('');

  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [draft, setDraft] = useState({ person_id: initialPerson, title: '', description: '', owner: '我', due_text: '', due_date: '' });

  useEffect(() => {
    personApi
      .list()
      .then((data) => {
        setPersons(data);
        // A stale person id (cleared state, deleted person) must not filter the list to nothing.
        setPersonFilter((current) => (current && data.some((p) => p.id === current) ? current : ''));
        setDraft((d) => ({ ...d, person_id: d.person_id && data.some((p) => p.id === d.person_id) ? d.person_id : data[0]?.id ?? '' }));
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const data = await followUpApi.list(personFilter || undefined, statusFilter || undefined);
      setItems(data);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [personFilter, statusFilter]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const run = async (id: string, action: () => Promise<FollowUp>) => {
    setBusyId(id);
    setActionError('');
    try {
      const updated = await action();
      setItems((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setPanel(null);
      setNote('');
      setNewDue('');
      setReason('');
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusyId('');
    }
  };

  const loadHistory = async (id: string) => {
    if (historyFor === id) {
      setHistoryFor('');
      return;
    }
    setHistoryFor(id);
    setHistoryLoading(true);
    try {
      const rows = await followUpApi.postponements(id);
      setHistory(rows);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setHistoryLoading(false);
    }
  };

  const create = async () => {
    const title = draft.title.trim();
    if (!title || !draft.person_id || creating) return;
    setCreating(true);
    setActionError('');
    try {
      const created = await followUpApi.create({
        person_id: draft.person_id,
        title,
        description: draft.description.trim(),
        owner: draft.owner,
        due_text: draft.due_text.trim(),
        due_date: draft.due_date,
      });
      setDraft({ ...draft, title: '', description: '', due_text: '', due_date: '' });
      setShowCreate(false);
      // A hand-typed item may fall outside the current filter; showing it
      // beats silently swallowing it.
      if (
        (personFilter && created.person_id !== personFilter) ||
        (statusFilter && created.status !== statusFilter)
      ) {
        setPersonFilter('');
        setStatusFilter('');
      } else {
        setItems((current) => [created, ...current]);
      }
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const personName = (id: string) => persons.find((p) => p.id === id)?.name ?? '未知人物';
  const today = todayISO();

  /**
   * Sort the flat list into the buckets that decide what to touch next:
   * overdue, due within a week, waiting on someone, later, and closed.
   * Items without a date sink to the bottom of their bucket rather than
   * pretending to be urgent.
   */
  const buckets = useMemo(() => {
    const soon = isoDate(7);
    const overdue: FollowUp[] = [];
    const dueSoon: FollowUp[] = [];
    const waiting: FollowUp[] = [];
    const upcoming: FollowUp[] = [];
    const closed: FollowUp[] = [];

    for (const item of items) {
      if (item.status === 'completed' || item.status === 'cancelled') {
        closed.push(item);
        continue;
      }
      if (item.due_date && item.due_date < today) {
        overdue.push(item);
        continue;
      }
      if (item.status === 'waiting') {
        waiting.push(item);
        continue;
      }
      if (item.due_date && item.due_date <= soon) {
        dueSoon.push(item);
        continue;
      }
      upcoming.push(item);
    }

    const byDue = (a: FollowUp, b: FollowUp) => (a.due_date || '9999').localeCompare(b.due_date || '9999');
    return [
      { key: 'overdue', label: '已逾期', icon: AlertTriangle, items: overdue.sort(byDue), tone: 'text-red-600 dark:text-red-400' },
      { key: 'soon', label: '七天内到期', icon: CalendarClock, items: dueSoon.sort(byDue), tone: 'text-amber-600 dark:text-amber-400' },
      { key: 'waiting', label: '等待对方', icon: Hourglass, items: waiting.sort(byDue), tone: 'text-amber-700 dark:text-amber-300' },
      { key: 'upcoming', label: '之后', icon: Inbox, items: upcoming.sort(byDue), tone: 'text-muted-foreground' },
      { key: 'closed', label: '已完成 / 已取消', icon: CheckCircle2, items: closed, tone: 'text-muted-foreground' },
    ].filter((bucket) => bucket.items.length > 0);
  }, [items, today]);

  const openCount = items.filter((item) => item.status === 'pending' || item.status === 'waiting').length;
  const overdueCount = items.filter(
    (item) => item.due_date && item.due_date < today && (item.status === 'pending' || item.status === 'waiting'),
  ).length;

  const renderItem = (item: FollowUp) => {
    const overdue = item.due_date && item.due_date < today && (item.status === 'pending' || item.status === 'waiting');
    const open = panel?.id === item.id ? panel.kind : null;
    return (
      <li key={item.id} className="lift rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-1 gap-3">
            <Avatar name={personName(item.person_id)} id={item.person_id} size="sm" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className={`rounded-full border px-2 py-0.5 text-xs ${statusBadgeClass(item.status)}`}>
                  {FOLLOW_UP_STATUS_LABEL[item.status]}
                </span>
                <span className="truncate text-sm font-medium text-foreground">{item.title}</span>
                {item.owner ? (
                  <span className="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">
                    {item.owner}的球
                  </span>
                ) : null}
                {overdue ? (
                  <span className="rounded-full border border-red-300/60 bg-red-50 px-2 py-0.5 text-xs text-red-700 dark:border-red-500/40 dark:bg-red-950/40 dark:text-red-300">
                    已逾期 {fullDate(item.due_date)}
                  </span>
                ) : item.due_date ? (
                  <span className="text-xs text-muted-foreground">期限 {fullDate(item.due_date)}</span>
                ) : null}
              </div>
              {item.due_text ? <p className="mt-1 text-xs text-muted-foreground">原话：{item.due_text}</p> : null}
              {item.description ? <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.description}</p> : null}
              {item.status === 'completed' && item.completion_note ? (
                <p className="mt-1 text-sm text-emerald-700 dark:text-emerald-300">结果：{item.completion_note}</p>
              ) : null}
              <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                <Link to={`/persons/${item.person_id}`} className="hover:text-primary">
                  {personName(item.person_id)}
                </Link>
                {item.source_event_id ? (
                  <Link to={`/events/${item.source_event_id}`} className="hover:text-primary">
                    来源记录
                  </Link>
                ) : null}
                {item.source_advice_id ? (
                  <Link to="/advice" state={{ personId: item.person_id }} className="hover:text-primary">
                    来自建议
                  </Link>
                ) : null}
                <button onClick={() => void loadHistory(item.id)} className="hover:text-primary">
                  延期历史
                </button>
              </div>
              {item.source_advice_stale ? (
                <div className="mt-2">
                  <Notice>这条事项依据的建议已被删除：{item.source_advice_stale_reason || '来源建议不存在了'}</Notice>
                </div>
              ) : null}
            </div>
          </div>

          {item.status === 'pending' || item.status === 'waiting' ? (
            <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
              <Button
                size="sm"
                variant={open === 'complete' ? 'outline' : 'default'}
                onClick={() => {
                  setPanel(open === 'complete' ? null : { id: item.id, kind: 'complete' });
                  setNote('');
                }}
              >
                {open === 'complete' ? '收起' : '完成'}
              </Button>
              <Button
                size="sm"
                variant={open === 'postpone' ? 'default' : 'outline'}
                onClick={() => {
                  setPanel(open === 'postpone' ? null : { id: item.id, kind: 'postpone' });
                  setNewDue(item.due_date || today);
                  setReason('');
                }}
              >
                延期
              </Button>
              {item.status === 'pending' ? (
                <Button size="sm" variant="ghost" disabled={busyId === item.id} onClick={() => void run(item.id, () => followUpApi.wait(item.id))}>
                  等待对方
                </Button>
              ) : null}
              <Button
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-red-600"
                disabled={busyId === item.id}
                onClick={() => {
                  if (window.confirm('确认取消这条事项？')) void run(item.id, () => followUpApi.cancel(item.id));
                }}
              >
                取消
              </Button>
            </div>
          ) : null}
        </div>

        {open === 'complete' ? (
          <div className="mt-3 rounded-lg border border-border bg-muted/40 p-3">
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={2}
              placeholder="结果（可选）：实际发生了什么？"
              className={`${controlClass} h-auto resize-y`}
            />
            <div className="mt-2 flex justify-end">
              <Button size="sm" disabled={busyId === item.id} onClick={() => void run(item.id, () => followUpApi.complete(item.id, note.trim() || undefined))}>
                {busyId === item.id ? '保存中…' : '标记完成'}
              </Button>
            </div>
          </div>
        ) : null}

        {open === 'postpone' ? (
          <div className="mt-3 rounded-lg border border-border bg-muted/40 p-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="text-sm text-muted-foreground">
                新期限
                <input type="date" value={newDue} onChange={(e) => setNewDue(e.target.value)} className={`${controlClass} mt-1`} />
              </label>
              <label className="text-sm text-muted-foreground">
                原因（可选）
                <input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="如：对方出差"
                  className={`${controlClass} mt-1`}
                />
              </label>
            </div>
            <div className="mt-2 flex justify-end">
              <Button
                size="sm"
                disabled={busyId === item.id || !newDue}
                onClick={() => void run(item.id, () => followUpApi.postpone(item.id, newDue, reason.trim() || undefined))}
              >
                {busyId === item.id ? '保存中…' : '确认延期'}
              </Button>
            </div>
          </div>
        ) : null}

        {historyFor === item.id ? (
          <div className="mt-3 rounded-lg border border-border bg-muted/40 p-3">
            <div className="mb-1 flex items-center gap-1 text-xs font-medium text-foreground">
              {historyLoading ? (
                <Spinner label="载入延期历史…" />
              ) : (
                <>
                  {history.length > 0 ? <ChevronRight className="size-3.5" /> : <ChevronDown className="size-3.5" />}
                  延期历史（{history.length}）
                </>
              )}
            </div>
            {!historyLoading && history.length === 0 ? (
              <p className="text-xs text-muted-foreground">从未延期过。</p>
            ) : (
              <ul className="space-y-1">
                {history.map((h) => (
                  <li key={h.id} className="text-xs text-muted-foreground">
                    {fullDate(h.old_due_date) || '（无期限）'} → {fullDate(h.new_due_date)}
                    {h.reason ? ` · ${h.reason}` : ''} · {fullDate(h.created_at)}
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </li>
    );
  };

  return (
    <div>
      <PageHeader
        title="跟进事项"
        description="谁的球、什么期限、结果如何——记录与建议转出的行动都在这里收口"
        actions={
          <>
            <select
              value={personFilter}
              onChange={(e) => setPersonFilter(e.target.value)}
              className={`${controlClass} h-9 w-auto min-w-32 text-muted-foreground`}
              title="按人物筛选"
            >
              <option value="">全部人物</option>
              {persons.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className={`${controlClass} h-9 w-auto text-muted-foreground`}
              title="按状态筛选"
            >
              <option value="">全部状态</option>
              {STATUS_OPTIONS.map((s) => (
                <option key={s} value={s}>
                  {FOLLOW_UP_STATUS_LABEL[s]}
                </option>
              ))}
            </select>
            <Button variant="outline" onClick={() => setShowCreate((v) => !v)}>
              {showCreate ? '收起' : '手动新建'}
            </Button>
          </>
        }
      />

      {error ? (
        <div className="mb-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {actionError ? (
        <div className="mb-4">
          <ErrorNote>{actionError}</ErrorNote>
        </div>
      ) : null}

      {!loading && items.length > 0 ? (
        <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
          <StatTile label="未完成" value={openCount} icon={<ListTodo className="size-3.5" />} />
          <StatTile
            label="已逾期"
            value={overdueCount}
            icon={<AlertTriangle className="size-3.5" />}
            tone={overdueCount > 0 ? 'danger' : 'default'}
            hint={overdueCount > 0 ? '今天先处理这些' : '没有拖欠'}
          />
          <StatTile
            label="等待对方"
            value={buckets.find((b) => b.key === 'waiting')?.items.length ?? 0}
            icon={<Hourglass className="size-3.5" />}
            hint="球在别人手上"
          />
          <StatTile
            label="已完成"
            value={items.filter((item) => item.status === 'completed').length}
            icon={<CheckCircle2 className="size-3.5" />}
          />
        </div>
      ) : null}

      {showCreate ? (
        <SectionCard className="mb-5" title="新建事项">
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="人物">
              <select
                value={draft.person_id}
                onChange={(e) => setDraft({ ...draft, person_id: e.target.value })}
                className={`${controlClass} h-9 text-muted-foreground`}
              >
                {persons.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="标题">
              <input
                value={draft.title}
                onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                placeholder="如：确认项目排期"
                className={`${controlClass} h-9`}
              />
            </Field>
            <Field label="说明">
              <input
                value={draft.description}
                onChange={(e) => setDraft({ ...draft, description: e.target.value })}
                placeholder="可选"
                className={`${controlClass} h-9`}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="谁的球">
                <select
                  value={draft.owner}
                  onChange={(e) => setDraft({ ...draft, owner: e.target.value })}
                  className={`${controlClass} h-9 text-muted-foreground`}
                >
                  {OWNER_OPTIONS.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="期限日期">
                <input
                  type="date"
                  value={draft.due_date}
                  onChange={(e) => setDraft({ ...draft, due_date: e.target.value })}
                  className={`${controlClass} h-9`}
                />
              </Field>
            </div>
            <div className="md:col-span-2">
              <Field label="期限原文" hint="与日期并存，保留当时的说法">
                <input
                  value={draft.due_text}
                  onChange={(e) => setDraft({ ...draft, due_text: e.target.value })}
                  placeholder="如：下周三前（可选）"
                  className={`${controlClass} h-9`}
                />
              </Field>
            </div>
          </div>
          <div className="mt-3 flex justify-end">
            <Button onClick={() => void create()} disabled={creating || !draft.title.trim() || !draft.person_id}>
              {creating ? '创建中…' : '创建'}
            </Button>
          </div>
        </SectionCard>
      ) : null}

      {loading ? (
        <Spinner label="载入事项…" />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<ListTodo className="size-6" />}
          title={personFilter || statusFilter ? '当前筛选下没有事项' : '还没有跟进事项'}
          description={
            personFilter || statusFilter
              ? '换个人物或状态看看。'
              : '记录里出现承诺时会自动产生，也可以手动新建一条。'
          }
          action={
            personFilter || statusFilter ? null : (
              <Button size="sm" onClick={() => setShowCreate(true)}>
                手动新建
              </Button>
            )
          }
        />
      ) : (
        <div className="space-y-6">
          {buckets.map((bucket) => {
            const Icon = bucket.icon;
            return (
              <section key={bucket.key}>
                <div className="mb-2 flex items-center gap-2">
                  <Icon className={`size-4 ${bucket.tone}`} />
                  <h2 className="text-sm font-semibold text-foreground">{bucket.label}</h2>
                  <span className="text-xs text-muted-foreground">{bucket.items.length}</span>
                  <span className="h-px flex-1 bg-border" />
                </div>
                <ul className="space-y-2">{bucket.items.map(renderItem)}</ul>
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}
