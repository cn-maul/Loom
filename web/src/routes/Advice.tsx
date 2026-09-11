import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import AdvicePanel from '../components/AdvicePanel';
import PersonPicker from '../components/PersonPicker';
import { ErrorNote, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { Textarea } from '../components/ui/textarea';
import { aiApi, personApi } from '../api/client';
import type { AdviceResponse, PersonWithActivity } from '../api/types';

export default function Advice() {
  const location = useLocation() as { state?: { personId?: string; question?: string } };
  const preset = location.state ?? {};

  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [personId, setPersonId] = useState(preset.personId ?? '');
  const [question, setQuestion] = useState(preset.question ?? '');
  const [advice, setAdvice] = useState<AdviceResponse | null>(null);
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

  const ask = async () => {
    const text = question.trim();
    if (!text || !personId || busy) return;
    setBusy(true);
    setError('');
    setAdvice(null);
    try {
      setAdvice(await aiApi.advice({ person_id: personId, question: text }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const personName = persons.find((person) => person.id === personId)?.name ?? '';

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
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

        <div className="mt-3 flex items-center gap-3">
          <Button onClick={ask} disabled={busy || !question.trim() || !personId}>
            生成建议
          </Button>
          {busy ? <Spinner label="AI 正在检索记录并生成（可能需要一两分钟）…" /> : null}
        </div>
      </div>

      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {advice ? <AdvicePanel advice={advice} personName={personName} /> : null}
    </div>
  );
}
