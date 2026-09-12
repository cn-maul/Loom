import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { MessageSquareText, Share2 } from 'lucide-react';
import AdvicePanel from '../components/AdvicePanel';
import PersonPicker from '../components/PersonPicker';
import { ErrorNote, Spinner, controlClass } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Textarea } from '../components/ui/textarea';
import { adviceApi, organizationApi, personApi, positionApi, relationshipApi } from '../api/client';
import { PageHeader, SectionCard } from '../components/layout';
import type { AdviceSession, OrgPositionLink, Organization, PersonWithActivity, RelationshipLink } from '../api/types';
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

  // Structured context: what the AI is about to read. It is shown as a fact
  // panel so the user can see (and correct) the relationship/position inputs
  // before asking, instead of wondering whether the answer knows them.
  const [relationships, setRelationships] = useState<RelationshipLink[]>([]);
  const [positions, setPositions] = useState<OrgPositionLink[]>([]);

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

  const loadStructure = useCallback(async (id: string) => {
    if (!id) {
      setRelationships([]);
      setPositions([]);
      return;
    }
    const [rels, pos] = await Promise.all([relationshipApi.byPerson(id), positionApi.byPerson(id)]);
    setRelationships(rels);
    setPositions(pos);
  }, []);

  // Switching person must not leave the previous person's answer on screen: the
  // evidence list would silently describe someone else.
  useEffect(() => {
    setAdvice(null);
    setError('');
    void loadHistory(personId);
    void loadStructure(personId).catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [personId, loadHistory, loadStructure]);

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
  const currentPositions = positions.filter((p) => !p.end_date);

  return (
    <div>
      <PageHeader title="问一问" description="基于画像、相关记录和你标注的关系与组织，给出逐条有依据的沟通建议" />

      <div className="grid gap-5 lg:grid-cols-[19rem_minmax(0,1fr)]">
        {/* Left: the person's past questions, conversation style. */}
        <aside className="space-y-4">
          <SectionCard title="这个人问过" bodyClassName="p-3">
            {history.length === 0 ? (
              <p className="text-sm leading-6 text-muted-foreground">还没有建议历史。问第一个问题后，这里会变成可回看的对话记录。</p>
            ) : (
              <ul className="space-y-1">
                {history.map((item) => (
                  <li key={item.id} className="rounded-lg px-2 py-1.5 transition-colors hover:bg-muted">
                    <div className="flex items-start justify-between gap-2">
                      <button
                        type="button"
                        onClick={() => void open(item.id)}
                        className={`flex-1 text-left text-sm leading-6 ${
                          advice?.id === item.id ? 'text-primary' : 'text-foreground hover:text-primary'
                        }`}
                      >
                        {item.question}
                      </button>
                      <Button variant="ghost" size="sm" className="h-6 px-1.5 text-xs" onClick={() => void remove(item.id)}>
                        删除
                      </Button>
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5 pl-0 text-xs text-muted-foreground">
                      <span>{relativeTime(item.created_at)}</span>
                      {item.source_stale === 1 ? <Badge variant="destructive">依据已变</Badge> : null}
                      {item.adopted_strategy_name ? <Badge variant="secondary">已采纳</Badge> : null}
                      {item.follow_up_ids.length > 0 ? <Badge variant="outline">跟进 {item.follow_up_ids.length}</Badge> : null}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </SectionCard>

          <SectionCard title="AI 会读到的关系与组织" bodyClassName="p-3">
            <div className="space-y-2 text-xs leading-5 text-muted-foreground">
              {relationships.length === 0 && currentPositions.length === 0 ? (
                <p>
                  还没有关系边或任职记录。去
                  <Link to="/relationships" className="mx-1 text-primary hover:underline">关系图谱</Link>
                  或人物详情页补上，AI 才会知道「TA 是你的上级 / 同事」这类立场。
                </p>
              ) : null}

              {relationships.length > 0 ? (
                <div>
                  <p className="mb-1 flex items-center gap-1 font-medium text-foreground">
                    <Share2 className="size-3.5" /> 关系
                  </p>
                  <ul className="space-y-1">
                    {relationships.map((rel) => {
                      const outgoing = rel.from_person_id === personId;
                      const other = outgoing ? rel.to_person_name : rel.from_person_name;
                      const arrow = rel.direction === 'undirected' ? '—' : outgoing ? '→' : '←';
                      return (
                        <li key={rel.id}>
                          <span className="text-foreground">{personName}</span> {arrow} {other}
                          <span className="ml-1 rounded bg-muted px-1.5 py-0.5">{rel.relation_type}</span>
                          {rel.confirmed ? null : <span className="ml-1 text-amber-600 dark:text-amber-400">未确认</span>}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ) : null}

              {currentPositions.length > 0 ? (
                <div className="mt-2">
                  <p className="mb-1 font-medium text-foreground">现任组织与职位</p>
                  <ul className="space-y-1">
                    {currentPositions.map((pos) => (
                      <li key={pos.id}>
                        {pos.org_name}
                        {pos.role ? `（${pos.role}）` : ''}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </div>
          </SectionCard>
        </aside>

        {/* Right: ask + the active answer. */}
        <div className="min-w-0 space-y-4">
          <SectionCard>
            <div className="mb-3 flex flex-wrap items-center gap-3">
              <select
                value={orgFilter}
                onChange={(e) => {
                  const value = e.target.value;
                  setOrgFilter(value);
                  const next =
                    value === 'none' ? persons.filter((p) => !p.org_id) : value ? persons.filter((p) => p.org_id === value) : persons;
                  if (next.length > 0 && !next.some((p) => p.id === personId)) setPersonId(next[0].id);
                }}
                className={`${controlClass} h-9 w-40 text-muted-foreground`}
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
              <span className="text-xs text-muted-foreground">左侧是这次回答会使用的结构化关系背景</span>
            </div>

            <Textarea
              value={question}
              onChange={(e) => setQuestion(e.target.value)}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void ask();
              }}
              rows={3}
              placeholder="想问什么？例如「明天要跟他谈延期，怎么开口比较不容易翻脸」"
              className="resize-y text-base leading-6"
            />
            <Input
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="这次想达成的目标（可选），例如「把话说清楚，但不伤关系」"
              className="mt-2"
            />

            <div className="mt-3 flex items-center gap-3">
              <Button onClick={ask} disabled={busy || !question.trim() || !personId}>
                <MessageSquareText className="size-4" />
                生成建议
              </Button>
              {busy ? <Spinner label="AI 正在检索记录并生成（可能需要一两分钟）…" /> : null}
            </div>
          </SectionCard>

          {error ? <ErrorNote>{error}</ErrorNote> : null}
          {advice ? <AdvicePanel advice={advice} personName={personName} onAdopted={setAdvice} /> : null}

          <p className="text-xs text-muted-foreground">
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