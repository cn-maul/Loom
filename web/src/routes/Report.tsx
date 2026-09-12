import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import PersonPicker from '../components/PersonPicker';
import { EmptyState, ErrorNote, Spinner } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { reportApi, personApi } from '../api/client';
import { PageHeader, SectionCard } from '../components/layout';
import type { PersonWithActivity, Report, ReportFollowUpRef, ReportSnapshot } from '../api/types';
import { fullDate, relativeTime } from '../format';

// Bucket order mirrors the report's own structure: late work first, outcomes
// last, so the reader works top to bottom instead of hunting.
type BucketKey = 'overdue' | 'due_soon' | 'waiting' | 'carried_over' | 'upcoming' | 'completed';

const BUCKETS: { key: BucketKey; title: string; tone: 'danger' | 'warn' | 'calm' | 'done' }[] = [
  { key: 'overdue', title: '逾期事项', tone: 'danger' },
  { key: 'due_soon', title: '本期到期', tone: 'warn' },
  { key: 'waiting', title: '等待对方回复', tone: 'calm' },
  { key: 'carried_over', title: '跨期未完成', tone: 'warn' },
  { key: 'upcoming', title: '下一步可执行', tone: 'calm' },
  { key: 'completed', title: '本期完成', tone: 'done' },
];

const BUCKET_STYLES: Record<string, string> = {
  danger: 'text-destructive',
  warn: 'text-amber-600 dark:text-amber-400',
  calm: 'text-foreground',
  done: 'text-muted-foreground',
};

function FollowUpList({ items }: { items: ReportFollowUpRef[] }) {
  return (
    <ul className="space-y-1.5 text-sm leading-6">
      {items.map((item) => (
        <li key={item.id} className="flex flex-wrap items-baseline gap-x-2">
          <Link to={`/persons/${item.person_id}`} className="font-medium text-primary hover:underline">
            {item.person_name || '未知人物'}
          </Link>
          <span className="text-foreground">{item.title}</span>
          {item.days_overdue > 0 ? <Badge variant="destructive">逾期 {item.days_overdue} 天</Badge> : null}
          {item.due_date ? <span className="text-xs text-muted-foreground">{item.due_date} 到期</span> : null}
          {!item.due_date && item.due_text ? <span className="text-xs text-muted-foreground">{item.due_text}</span> : null}
          {item.owner ? <span className="text-xs text-muted-foreground">负责人 {item.owner}</span> : null}
        </li>
      ))}
    </ul>
  );
}

export default function Report() {
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [personId, setPersonId] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [report, setReport] = useState<Report | null>(null);
  const [history, setHistory] = useState<ReportSnapshot[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    personApi.list().then(setPersons).catch(() => undefined);
  }, []);

  const loadHistory = useCallback(async (id: string) => {
    try {
      setHistory(await reportApi.list(id || undefined));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    // Switching the person scope must not leave the previous scope's report on
    // screen: its numbers would quietly describe someone else.
    setReport(null);
    void loadHistory(personId);
  }, [personId, loadHistory]);

  const generate = async (weekOf?: string) => {
    setBusy(true);
    setError('');
    setReport(null);
    try {
      const data = await reportApi.generate({
        person_id: personId || undefined,
        start: weekOf ? undefined : start || undefined,
        end: weekOf ? undefined : end || undefined,
        week_of: weekOf,
      });
      setReport(data);
      void loadHistory(personId);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const open = async (id: string) => {
    setError('');
    try {
      setReport(await reportApi.get(id));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const remove = async (id: string) => {
    if (!window.confirm('删除这份报告快照？')) return;
    setError('');
    try {
      await reportApi.remove(id);
      if (report?.id === id) setReport(null);
      void loadHistory(personId);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const lastWeekOf = () => {
    const date = new Date();
    date.setDate(date.getDate() - 7);
    return date.toISOString().slice(0, 10);
  };

  return (
    <div>
      <PageHeader
        title="周报"
        description="先汇总结构化事实，再让模型写叙述；模型不可用时保留大纲，不假装有结论"
      />
      <SectionCard className="mb-5">
        <div className="flex flex-wrap items-center gap-3">
          <PersonPicker persons={persons} value={personId} onChange={setPersonId} placeholder="全部人物" />
          <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} className="w-40" />
          <span className="text-xs text-muted-foreground">至</span>
          <Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} className="w-40" />
          <Button onClick={() => void generate()} disabled={busy}>
            生成本期报告
          </Button>
          <Button variant="outline" onClick={() => void generate(lastWeekOf())} disabled={busy}>
            上一周
          </Button>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          不填日期则默认最近七天；报告生成后会保存为快照，可随时回看。
        </p>
      </SectionCard>

      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {busy ? <Spinner label="正在汇总记录并生成报告…" /> : null}

      {report ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-semibold text-foreground">
                {fullDate(report.start)} ~ {fullDate(report.end)}（{report.days} 天）
              </h2>
              {report.person_name ? <Badge variant="secondary">{report.person_name}</Badge> : null}
              {report.status === 'failed' ? (
                <Badge variant="destructive" title={report.failure_reason}>
                  叙述生成失败，已保留结构化汇总
                </Badge>
              ) : null}
              <span className="text-xs text-muted-foreground">
                {report.event_count} 条记录 · {report.open_count} 项未完成 · 由 {report.generated_by} 生成
              </span>
            </div>
            <p className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground">{report.summary}</p>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            {BUCKETS.map(({ key, title, tone }) => {
              const items = report[key];
              if (items.length === 0) return null;
              return (
                <div key={key} className="rounded-xl border border-border bg-card p-5 shadow-sm">
                  <h3 className={`mb-2 text-sm font-semibold ${BUCKET_STYLES[tone]}`}>
                    {title}（{items.length}）
                  </h3>
                  <FollowUpList items={items} />
                </div>
              );
            })}
          </div>

          {report.changes.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h3 className="mb-2 text-sm font-semibold text-foreground">关系与任职变化（{report.changes.length}）</h3>
              <ul className="space-y-1.5 text-sm leading-6 text-muted-foreground">
                {report.changes.map((change) => (
                  <li key={`${change.kind}-${change.id}-${change.date}`}>
                    <span className="text-foreground">{change.person_name || '某人'}</span> 于 {change.date}
                    {{ relationship_started: '建立关系', relationship_ended: '结束关系', position_started: '开始任职', position_ended: '结束任职' }[change.kind] ?? change.kind}
                    ：{change.description || '身份'}
                    {change.counterpart_name ? `（对方：${change.counterpart_name}）` : ''}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {report.promises.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h3 className="mb-2 text-sm font-semibold text-foreground">本期承诺（{report.promises.length}）</h3>
              <ul className="space-y-1.5 text-sm leading-6 text-muted-foreground">
                {report.promises.map((promise, index) => (
                  <li key={`${promise.event_id}-${index}`}>
                    {promise.event_date} · <span className="text-foreground">{promise.person_name}</span>
                    ：{promise.who} 说「{promise.what}」
                    {promise.deadline ? `（${promise.deadline}）` : ''}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {report.events.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h3 className="mb-2 text-sm font-semibold text-foreground">本期记录（{report.events.length}）</h3>
              <ul className="space-y-1.5 text-sm leading-6 text-muted-foreground">
                {report.events.map((event) => (
                  <li key={event.id}>
                    {event.event_date} ·{' '}
                    <Link to={`/persons/${event.person_id}`} className="text-primary hover:underline">
                      {event.person_name}
                    </Link>
                    ：{event.summary}
                    {event.extraction_status === 'failed' ? <Badge variant="destructive">提取失败</Badge> : null}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {report.persons.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h3 className="mb-2 text-sm font-semibold text-foreground">按人物汇总</h3>
              <ul className="space-y-1.5 text-sm leading-6 text-muted-foreground">
                {report.persons.map((person) => (
                  <li key={person.person_id}>
                    <Link to={`/persons/${person.person_id}`} className="text-primary hover:underline">
                      {person.person_name || '未知人物'}
                    </Link>
                    ：{person.event_count} 条记录，最近 {person.last_event_date || '—'}
                    {person.open_follow_ups > 0 ? `，${person.open_follow_ups} 项未完成` : ''}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}

      {history.length > 0 ? (
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <h3 className="mb-2 text-sm font-semibold text-foreground">报告历史</h3>
          <ul className="divide-y divide-border">
            {history.map((item) => (
              <li key={item.id} className="flex items-start justify-between gap-3 py-2">
                <button
                  type="button"
                  onClick={() => void open(item.id)}
                  className={`flex-1 text-left text-sm leading-6 hover:text-primary ${
                    report?.id === item.id ? 'text-primary' : 'text-foreground'
                  }`}
                >
                  <span className="mr-2 font-mono text-xs text-muted-foreground">{relativeTime(item.generated_at)}</span>
                  {fullDate(item.start)} ~ {fullDate(item.end)}
                  <span className="ml-2 inline-flex flex-wrap items-center gap-1 align-middle">
                    {item.person_name ? <Badge variant="secondary">{item.person_name}</Badge> : null}
                    {item.status === 'failed' ? <Badge variant="destructive">叙述失败</Badge> : null}
                    <Badge variant="outline">{item.event_count} 条记录</Badge>
                  </span>
                </button>
                <Button variant="ghost" size="sm" onClick={() => void remove(item.id)}>
                  删除
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <EmptyState>还没有报告。选择时间范围后点「生成本期报告」。</EmptyState>
      )}
    </div>
  );
}
