import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import PersonPicker from '../components/PersonPicker';
import { ErrorNote, Spinner } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { reportApi, personApi } from '../api/client';
import { EmptyState, PageHeader, SectionCard } from '../components/layout';
import type { PersonWithActivity, Report, ReportFollowUpRef, ReportSnapshot } from '../api/types';
import { fullDate, relativeTime } from '../format';
import { cn } from '../lib/utils';

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

// Heat and danger are the only non-grey tones a bucket heading may take; a
// bucket that is merely "in progress" stays in the greyscale hierarchy.
const BUCKET_STYLES: Record<string, string> = {
  danger: 'text-destructive',
  warn: 'text-heat-text',
  calm: 'text-foreground',
  done: 'text-ink-3',
};

function FollowUpList({ items }: { items: ReportFollowUpRef[] }) {
  return (
    <ul className="mt-1.5 space-y-1.5">
      {items.map((item) => (
        <li key={item.id} className="flex flex-wrap items-baseline gap-x-2 text-[13.5px] leading-[1.7]">
          <Link to={`/persons/${item.person_id}`} className="font-medium text-primary hover:underline">
            {item.person_name || '未知人物'}
          </Link>
          <span className="text-foreground">{item.title}</span>
          {item.days_overdue > 0 ? (
            <Badge className="bg-destructive/10 text-destructive">逾期 {item.days_overdue} 天</Badge>
          ) : null}
          {item.due_date ? <span className="text-[12px] tabular-nums text-ink-3">{item.due_date} 到期</span> : null}
          {!item.due_date && item.due_text ? <span className="text-[12px] text-ink-3">{item.due_text}</span> : null}
          {item.owner ? <span className="text-[12px] text-ink-3">负责人 {item.owner}</span> : null}
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

  const field = 'h-9 text-[13px]';

  return (
    <div>
      <PageHeader
        title="周报"
        description="先汇总结构化事实，再让模型写叙述；模型不可用时保留大纲，不假装有结论"
      />

      <SectionCard className="mb-6">
        <div className="flex flex-wrap items-center gap-3">
          <PersonPicker persons={persons} value={personId} onChange={setPersonId} placeholder="全部人物" />
          <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} className={`${field} w-40`} />
          <span className="text-[13px] text-ink-3">至</span>
          <Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} className={`${field} w-40`} />
          <Button onClick={() => void generate()} disabled={busy}>
            生成本期报告
          </Button>
          <Button variant="outline" onClick={() => void generate(lastWeekOf())} disabled={busy}>
            上一周
          </Button>
        </div>
        <p className="mt-2.5 text-[12px] text-muted-foreground">不填日期则默认最近七天；报告生成后会保存为快照，可随时回看。</p>
      </SectionCard>

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {busy ? <Spinner label="正在汇总记录并生成报告…" /> : null}

      {report ? (
        <div className="space-y-9">
          <SectionCard>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">
                {fullDate(report.start)} ~ {fullDate(report.end)}（{report.days} 天）
              </h2>
              {report.person_name ? <Badge variant="secondary">{report.person_name}</Badge> : null}
              {report.status === 'failed' ? (
                <Badge className="bg-heat-bg text-heat-text" title={report.failure_reason}>
                  叙述生成失败，已保留结构化汇总
                </Badge>
              ) : null}
              <span className="text-[12px] tabular-nums text-ink-3">
                {report.event_count} 条记录 · {report.open_count} 项未完成 · 由 {report.generated_by} 生成
              </span>
            </div>
            <p className="mt-3 whitespace-pre-wrap text-[14px] leading-[1.85] text-ink-2">{report.summary}</p>
          </SectionCard>

          {/* The six buckets are siblings of one kind, so they share ONE panel:
              six bordered cards with six coloured headings read as a bag of
              islands and spend colour that only heat and danger have earned. */}
          {BUCKETS.some(({ key }) => report[key].length > 0) ? (
            <section className="panel">
              <div className="border-b border-hairline px-5 py-[13px]">
                <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">跟进事项</h2>
              </div>
              <ul className="panel-rows">
                {BUCKETS.map(({ key, title, tone }) => {
                  const items = report[key];
                  if (items.length === 0) return null;
                  return (
                    <li key={key} className="px-5 py-4">
                      <h3 className={cn('text-[13.5px] font-semibold tracking-[-0.01em]', BUCKET_STYLES[tone])}>
                        {title}
                        <span className="ml-2 font-normal tabular-nums text-ink-4">{items.length}</span>
                      </h3>
                      <FollowUpList items={items} />
                    </li>
                  );
                })}
              </ul>
            </section>
          ) : null}

          {report.promises.length > 0 ? (
            <SectionCard title={`本期承诺（${report.promises.length}）`}>
              <ul className="space-y-2 text-[13.5px] leading-[1.7] text-ink-2">
                {report.promises.map((promise, index) => (
                  <li key={`${promise.event_id}-${index}`}>
                    <span className="tabular-nums text-ink-3">{promise.event_date}</span> ·{' '}
                    <span className="font-medium text-foreground">{promise.person_name}</span>：{promise.who} 说「
                    {promise.what}」
                    {promise.deadline ? <span className="text-ink-3">（{promise.deadline}）</span> : ''}
                  </li>
                ))}
              </ul>
            </SectionCard>
          ) : null}

          {report.events.length > 0 ? (
            <SectionCard title={`本期记录（${report.events.length}）`}>
              <ul className="space-y-2 text-[13.5px] leading-[1.7] text-ink-2">
                {report.events.map((event) => (
                  <li key={event.id} className="flex flex-wrap items-baseline gap-x-2">
                    <span className="tabular-nums text-ink-3">{event.event_date}</span>
                    <Link to={`/persons/${event.person_id}`} className="font-medium text-primary hover:underline">
                      {event.person_name}
                    </Link>
                    <span>{event.summary}</span>
                    {event.extraction_status === 'failed' ? (
                      <Badge className="bg-destructive/10 text-destructive">提取失败</Badge>
                    ) : null}
                  </li>
                ))}
              </ul>
            </SectionCard>
          ) : null}

          {report.persons.length > 0 ? (
            <SectionCard title="按人物汇总" bodyClassName="p-0">
              <ul className="panel-rows">
                {report.persons.map((person) => (
                  <li key={person.person_id} className="flex flex-wrap items-baseline gap-x-2 px-5 py-3 text-[13.5px] leading-[1.7]">
                    <Link to={`/persons/${person.person_id}`} className="font-medium text-primary hover:underline">
                      {person.person_name || '未知人物'}
                    </Link>
                    <span className="text-ink-2">
                      {person.event_count} 条记录，最近 {person.last_event_date || '—'}
                      {person.open_follow_ups > 0 ? `，${person.open_follow_ups} 项未完成` : ''}
                    </span>
                  </li>
                ))}
              </ul>
            </SectionCard>
          ) : null}
        </div>
      ) : null}

      {history.length > 0 ? (
        <section className="panel mt-5">
          <div className="border-b border-hairline px-5 py-[13px]">
            <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">报告历史</h2>
          </div>
          <ul className="panel-rows">
            {history.map((item) => (
              <li key={item.id} className="group flex items-start justify-between gap-3 px-5 py-3 transition-colors hover:bg-hover">
                <button
                  type="button"
                  onClick={() => void open(item.id)}
                  className={cn(
                    'flex-1 text-left text-[13.5px] leading-[1.7] hover:text-primary',
                    report?.id === item.id ? 'font-medium text-primary' : 'text-ink-2',
                  )}
                >
                  <span className="mr-2 tabular-nums text-[12px] text-ink-3">{relativeTime(item.generated_at)}</span>
                  {fullDate(item.start)} ~ {fullDate(item.end)}
                  <span className="ml-2 inline-flex flex-wrap items-center gap-1 align-middle">
                    {item.person_name ? <Badge variant="secondary">{item.person_name}</Badge> : null}
                    {item.status === 'failed' ? <Badge className="bg-heat-bg text-heat-text">叙述失败</Badge> : null}
                    <Badge variant="outline">{item.event_count} 条记录</Badge>
                  </span>
                </button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 shrink-0 px-2 text-[12px] text-ink-3 opacity-0 transition-opacity group-hover:opacity-100"
                  onClick={() => void remove(item.id)}
                >
                  删除
                </Button>
              </li>
            ))}
          </ul>
        </section>
      ) : (
        <EmptyState title="还没有报告" description="选择时间范围后点「生成本期报告」。" />
      )}
    </div>
  );
}
