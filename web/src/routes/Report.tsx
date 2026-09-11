import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import PersonPicker from '../components/PersonPicker';
import { EmptyState, ErrorNote, Spinner } from '../components/ui';
import { aiApi, personApi } from '../api/client';
import type { PersonWithActivity, WeeklyReport } from '../api/types';
import { fullDate } from '../format';

/** The report body is Markdown prose from the model; only the handful of markers it
 *  actually produces are rendered, so a stray token shows up as text rather than breaking. */
function MarkdownLite({ text }: { text: string }) {
  const blocks: { kind: 'h3' | 'h4' | 'p' | 'ul'; content: string }[] = [];
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith('### ')) blocks.push({ kind: 'h4', content: line.slice(4) });
    else if (line.startsWith('## ')) blocks.push({ kind: 'h3', content: line.slice(3) });
    else if (line.startsWith('- ') || line.startsWith('* ')) blocks.push({ kind: 'ul', content: line.slice(2) });
    else blocks.push({ kind: 'p', content: line.replace(/^#+\s*/, '') });
  }

  const nodes: ReactNode[] = [];
  let list: string[] = [];
  const flush = (key: string) => {
    if (list.length === 0) return;
    nodes.push(
      <ul key={key} className="my-2 list-disc space-y-1 pl-5 text-sm leading-6 text-muted-foreground">
        {list.map((item, index) => (
          <li key={index}>{item}</li>
        ))}
      </ul>,
    );
    list = [];
  };

  blocks.forEach((block, index) => {
    if (block.kind === 'ul') {
      list.push(block.content);
      if (index === blocks.length - 1) flush(`list-${index}`);
      return;
    }
    flush(`list-${index}`);
    if (block.kind === 'h3') nodes.push(<h3 key={index} className="mb-1 mt-4 text-sm font-semibold text-foreground">{block.content}</h3>);
    else if (block.kind === 'h4') nodes.push(<h4 key={index} className="mb-1 mt-3 text-sm font-medium text-foreground">{block.content}</h4>);
    else nodes.push(<p key={index} className="my-1 text-sm leading-7 text-muted-foreground">{block.content}</p>);
  });

  return <div>{nodes}</div>;
}

export default function Report() {
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [personId, setPersonId] = useState('');
  const [report, setReport] = useState<WeeklyReport | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    personApi.list().then(setPersons).catch(() => undefined);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setBusy(true);
    setError('');
    aiApi
      .weeklyReport(personId || undefined)
      .then((data) => {
        if (!cancelled) setReport(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setBusy(false);
      });
    return () => {
      cancelled = true;
    };
  }, [personId]);

  const promises = report?.persons.flatMap((slice) => slice.promises.map((text) => ({ text, personId: slice.person_id }))) ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">关系周报</h1>
        <PersonPicker persons={persons} value={personId} onChange={setPersonId} placeholder="全部人物" />
      </div>

      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {busy ? <Spinner label="正在汇总本周记录…" /> : null}

      {!busy && report ? (
        <>
          <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
            <p className="mb-2 text-xs text-muted-foreground">
              {fullDate(report.start)} ~ {fullDate(report.end)} · {report.event_count} 条记录 · 生成于 {report.generated_by}
            </p>
            <MarkdownLite text={report.summary} />
          </div>

          {promises.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h2 className="mb-2 text-sm font-semibold text-foreground">需要跟进的承诺</h2>
              <ul className="space-y-1 text-sm leading-6 text-muted-foreground">
                {promises.map(({ text, personId: pid }) => (
                  <li key={`${pid}-${text}`}>
                    <Link to={pid ? `/persons/${pid}` : '/'} className="hover:text-primary">
                      {text}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {report.persons.length > 0 ? (
            <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
              <h2 className="mb-2 text-sm font-semibold text-foreground">本周明细</h2>
              <div className="space-y-4">
                {report.persons.map((slice) => (
                  <div key={slice.person_id}>
                    <Link to={`/persons/${slice.person_id}`} className="text-sm font-medium text-primary">
                      {slice.person_name}
                    </Link>
                    <ul className="mt-1 list-disc space-y-1 pl-5 text-sm leading-6 text-muted-foreground">
                      {slice.events.map((line) => (
                        <li key={line}>{line}</li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <EmptyState>本周没有记录。</EmptyState>
          )}
        </>
      ) : null}
    </div>
  );
}
