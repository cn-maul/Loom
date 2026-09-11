import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import AdvicePanel from '../components/AdvicePanel';
import PersonPicker from '../components/PersonPicker';
import { ErrorNote, Spinner } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Textarea } from '../components/ui/textarea';
import { adviceApi, personApi } from '../api/client';
import { PageHeader, SectionCard } from '../components/layout';
import type { AdviceSession, PersonWithActivity } from '../api/types';
import { relativeTime } from '../format';

export default function Advice() {
  const location = useLocation() as { state?: { personId?: string; question?: string } };
  const preset = location.state ?? {};

  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
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

  const personName = persons.find((person) => person.id === personId)?.name ?? '';

  return (
    <div>
      <PageHeader title="建议" description="问一个具体问题；每条结论都会带上它引用的记录，方便你核对" />
      <SectionCard className="mb-5">
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <PersonPicker persons={persons} value={personId} onChange={setPersonId} placeholder="选择要咨询的人物" />
          <span className="text-xs text-muted-foreground">AI 会结合画像、语义检索到的相关记录和近期事件</span>
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
            生成建议
          </Button>
          {busy ? <Spinner label="AI 正在检索记录并生成（可能需要一两分钟）…" /> : null}
        </div>
      </SectionCard>

      <div className="space-y-4">
        {error ? <ErrorNote>{error}</ErrorNote> : null}
        {advice ? <AdvicePanel advice={advice} personName={personName} onAdopted={setAdvice} /> : null}

        {history.length > 0 ? (
          <SectionCard title="这个人的建议历史" bodyClassName="p-3">
            <ul className="divide-y divide-border">
            {history.map((item) => (
              <li key={item.id} className="flex items-start justify-between gap-3 py-2">
                <button
                  type="button"
                  onClick={() => void open(item.id)}
                  className={`flex-1 text-left text-sm leading-6 hover:text-primary ${
                    advice?.id === item.id ? 'text-primary' : 'text-foreground'
                  }`}
                >
                  <span className="mr-2 font-mono text-xs text-muted-foreground">{relativeTime(item.created_at)}</span>
                  {item.question}
                  <span className="ml-2 inline-flex flex-wrap items-center gap-1 align-middle">
                    {item.source_stale === 1 ? <Badge variant="destructive">依据已变化</Badge> : null}
                    {item.adopted_strategy_name ? <Badge variant="secondary">已采纳</Badge> : null}
                    {item.follow_up_ids.length > 0 ? (
                      <Badge variant="outline">跟进 {item.follow_up_ids.length}</Badge>
                    ) : null}
                  </span>
                </button>
                <Button variant="ghost" size="sm" onClick={() => void remove(item.id)}>
                  删除
                </Button>
              </li>
            ))}
            </ul>
          </SectionCard>
        ) : null}

        <p className="text-xs text-muted-foreground">
          建议会保存下来，之后可以回看当时问了什么、依据了哪些记录。也可以在
          <Link to="/" className="mx-1 text-primary hover:underline">
            人物列表
          </Link>
          里继续补充记录。
        </p>
      </div>
    </div>
  );
}
