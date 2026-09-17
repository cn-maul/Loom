import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { MessageSquareText } from 'lucide-react';
import AdvicePanel from '../components/AdvicePanel';
import PersonPicker from '../components/PersonPicker';
import { ErrorNote, Spinner, controlClass } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Textarea } from '../components/ui/textarea';
import { adviceApi, organizationApi, personApi } from '../api/client';
import { EmptyState, PageHeader, SectionCard } from '../components/layout';
import type { AdviceSession, Organization, PersonWithActivity } from '../api/types';
import { relativeTime } from '../format';

export default function Advice() {
  const location = useLocation() as { state?: { personId?: string; question?: string } };
  const preset = location.state ?? {};

  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [orgFilter, setOrgFilter] = useState('');
  const [personId, setPersonId] = useState(preset.personId ?? '');
  const [question, setQuestion] = useState(preset.question ?? '');
  const [goal, setGoal] = useState('');
  const [history, setHistory] = useState<AdviceSession[]>([]);
  const [advice, setAdvice] = useState<AdviceSession | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    personApi
      .list()
      .then((data) => {
        setPersons(data);
        setPersonId((current) => current || data[0]?.id || '');
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    organizationApi
      .list()
      .then(setOrganizations)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const loadHistory = useCallback(async (id: string) => {
    if (!id) {
      setHistory([]);
      return;
    }
    try {
      setHistory(await adviceApi.list(id));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  // Switching person must not leave the previous person's answer on screen: the
  // evidence list would silently describe someone else.
  useEffect(() => {
    setAdvice(null);
    setError('');
    void loadHistory(personId);
  }, [personId, loadHistory]);

  const ask = async () => {
    const text = question.trim();
    if (!text || !personId || busy) return;
    setBusy(true);
    setError('');
    setAdvice(null);
    try {
      const session = await adviceApi.generate({ person_id: personId, question: text, goal: goal.trim() || undefined });
      setAdvice(session);
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
      setAdvice(await adviceApi.get(id));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const remove = async (id: string) => {
    // Deleting the reasoning behind an already-adopted strategy is a real
    // consequence, so it is confirmed rather than one click away.
    if (!window.confirm('删除这份建议？已由它转出的跟进事项会保留，但会标记来源已删除。')) return;
    setError('');
    try {
      await adviceApi.remove(id);
      if (advice?.id === id) setAdvice(null);
      void loadHistory(personId);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const selectablePersons = orgFilter === 'none' ? persons.filter((p) => !p.org_id) : orgFilter ? persons.filter((p) => p.org_id === orgFilter) : persons;
  const personName = persons.find((person) => person.id === personId)?.name ?? '';

  return (
    <div>
      <PageHeader title="问一问" description="基于画像和相关记录，给出逐条有依据的沟通建议" />

      <div className="grid gap-5 lg:grid-cols-[19rem_minmax(0,1fr)]">
        {/* Left: the person's past questions as one continuous surface — this is
            a list of siblings, so hairlines do the dividing, not nine cards. */}
        <aside>
          <section className="panel">
            <div className="border-b border-hairline px-5 py-[13px]">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">这个人问过</h2>
            </div>
            {history.length === 0 ? (
              <EmptyState
                className="px-5 py-8"
                title="还没有建议历史"
                description="问第一个问题后，这里会变成可回看的对话记录。"
              />
            ) : (
              <ul className="panel-rows">
                {history.map((item) => (
                  <li key={item.id} className="group px-5 py-3 transition-colors hover:bg-hover">
                    <div className="flex items-start justify-between gap-2">
                      <button
                        type="button"
                        onClick={() => void open(item.id)}
                        className={`flex-1 text-left text-[13.5px] leading-[1.65] ${
                          advice?.id === item.id ? 'font-medium text-primary' : 'text-foreground hover:text-primary'
                        }`}
                      >
                        {item.question}
                      </button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 shrink-0 px-2 text-[12px] text-ink-3 opacity-0 transition-opacity group-hover:opacity-100"
                        onClick={() => void remove(item.id)}
                      >
                        删除
                      </Button>
                    </div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[12px] text-ink-3">
                      <span className="tabular-nums">{relativeTime(item.created_at)}</span>
                      {item.source_stale === 1 ? (
                        <Badge className="bg-heat-bg text-heat-text">依据已变</Badge>
                      ) : null}
                      {item.adopted_strategy_name ? (
                        <Badge className="bg-live-bg text-live-text">已采纳</Badge>
                      ) : null}
                      {item.follow_up_ids.length > 0 ? <Badge variant="secondary">跟进 {item.follow_up_ids.length}</Badge> : null}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </aside>

        {/* Right: ask + the active answer. */}
        <div className="min-w-0 space-y-9">
          <SectionCard>
            <div className="mb-3.5 flex flex-wrap items-center gap-3">
              <select
                value={orgFilter}
                onChange={(e) => {
                  const value = e.target.value;
                  setOrgFilter(value);
                  const next =
                    value === 'none' ? persons.filter((p) => !p.org_id) : value ? persons.filter((p) => p.org_id === value) : persons;
                  if (next.length > 0 && !next.some((p) => p.id === personId)) setPersonId(next[0].id);
                }}
                className={`${controlClass} h-9 w-40 text-[13px] text-ink-2`}
                title="先按组织过滤，人物下拉更准确"
              >
                <option value="">全部组织</option>
                <option value="none">未归属</option>
                {organizations.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
              <PersonPicker persons={selectablePersons} value={personId} onChange={setPersonId} placeholder="选择要咨询的人物" />
            </div>

            <Textarea
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void ask();
              }}
              rows={3}
              placeholder="想问什么？例如「明天要跟他谈延期，怎么开口比较不容易翻脸」"
              className="resize-y text-[15px] leading-[1.75]"
            />
            <Input
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="这次想达成的目标（可选），例如「把话说清楚，但不伤关系」"
              className="mt-2.5"
            />

            <div className="mt-3.5 flex flex-wrap items-center gap-3">
              <Button onClick={ask} disabled={busy || !question.trim() || !personId}>
                <MessageSquareText className="size-4" />
                生成建议
              </Button>
              {busy ? <Spinner label="AI 正在检索记录并生成（可能需要一两分钟）…" /> : null}
            </div>
          </SectionCard>

          {error ? <ErrorNote>{error}</ErrorNote> : null}
          {advice ? <AdvicePanel advice={advice} personName={personName} onAdopted={setAdvice} /> : null}

          <p className="text-[12px] leading-[1.7] text-muted-foreground">
            建议会保存下来，之后可以回看当时问了什么、依据了哪些记录。也可以在
            <Link to="/" className="mx-1 text-primary hover:underline">
              首页
            </Link>
            里继续补充记录。
          </p>
        </div>
      </div>
    </div>
  );
}
