import { useState } from 'react';
import { eventApi } from '../api/client';
import type { Event, IngestReport } from '../api/types';
import { todayISO } from '../format';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';
import { ErrorNote, Notice, Spinner, controlClass } from './ui';
import { cn } from '../lib/utils';

interface Props {
  personId: string;
  onRecorded: (event: Event, report: IngestReport) => void;
  /** Present when the host page can take the user straight to the advice flow. */
  onAsk?: (event: Event) => void;
  /** Set when the host already wraps this in a card, to avoid double borders. */
  bare?: boolean;
}

export default function QuickRecord({ personId, onRecorded, onAsk, bare = false }: Props) {
  const [text, setText] = useState('');
  const [date, setDate] = useState(todayISO());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [report, setReport] = useState<IngestReport | null>(null);

  const submit = async (ask: boolean) => {
    const raw = text.trim();
    if (!raw || busy) return;

    setBusy(true);
    setError('');
    setReport(null);
    try {
      const result = await eventApi.create({ person_id: personId, event_date: date, raw_text: raw });
      setText('');
      setDate(todayISO());
      setReport(result.report);
      onRecorded(result.event, result.report);
      if (ask) onAsk?.(result.event);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={cn(bare ? '' : 'rounded-xl border border-border bg-card p-4 shadow-sm')}>
      {bare ? (
        <div className="mb-2 flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">Ctrl / ⌘ + Enter 快速提交</span>
          <input
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className={`${controlClass} h-8 w-auto px-2 py-0 text-xs text-muted-foreground`}
          />
        </div>
      ) : (
        <div className="mb-3 flex items-center justify-between gap-3">
          <span className="text-sm font-medium text-foreground">记一笔</span>
          <input
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className={`${controlClass} h-auto w-auto px-2 py-1 text-muted-foreground`}
          />
        </div>
      )}

      <Textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') submit(false);
        }}
        rows={4}
        placeholder="今天发生了什么？直接写，AI 会提取摘要、你的感受、对方的反应和承诺。"
        className="resize-y text-base leading-6"
      />

      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Button onClick={() => submit(false)} disabled={busy || !text.trim()}>
          记录
        </Button>
        {onAsk ? (
          <Button
            onClick={() => submit(true)}
            disabled={busy || !text.trim()}
            variant="outline"
            className="border-primary text-primary hover:bg-primary/10 hover:text-primary"
          >
            记录并提问
          </Button>
        ) : null}
        {busy ? <Spinner label="AI 正在处理：提取要点、建立索引、更新画像…" /> : null}
      </div>

      {error ? (
        <div className="mt-3">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      {report && report.warnings.length > 0 ? (
        <div className="mt-3 space-y-2">
          {report.warnings.map((warning) => (
            <Notice key={warning}>{warning}</Notice>
          ))}
        </div>
      ) : null}
    </div>
  );
}
