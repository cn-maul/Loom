import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { NotebookPen, Search, X } from 'lucide-react';
import { eventApi, personApi } from '../api/client';
import type { PersonWithActivity } from '../api/types';
import { relativeTime, todayISO } from '../format';
import { Avatar } from './layout';
import { ErrorNote, Notice, Spinner, controlClass } from './ui';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';
import { cn } from '../lib/utils';

/**
 * The global "记一笔" drawer. Writing a record is the single most frequent
 * action in Loom, so it is reachable from anywhere: pick a person (or none for
 * a quick "standalone" moment), type, save. On success it offers the one-step
 * follow-through that matters — turn this record straight into a question.
 */
export default function QuickRecordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();

  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [personId, setPersonId] = useState('');
  const [search, setSearch] = useState('');
  const [text, setText] = useState('');
  const [date, setDate] = useState(todayISO());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // Load the directory once per open; the list is cheap and re-read keeps it fresh.
  useEffect(() => {
    if (!open) return;
    setSaved(false);
    setText('');
    setDate(todayISO());
    setError('');
    setSearch('');
    personApi
      .list()
      .then((data) => {
        setPersons(data);
        setPersonId((current) => (data.some((p) => p.id === current) ? current : data[0]?.id ?? ''));
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
    // Focus the writing surface when the drawer opens.
    const t = window.setTimeout(() => inputRef.current?.focus(), 60);
    return () => window.clearTimeout(t);
  }, [open]);

  // Escape closes; this drawer is modal so the rest of the app waits.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return persons;
    return persons.filter((p) => {
      const hay = [p.name, p.relation, p.org_name, p.position].filter(Boolean).join(' ').toLowerCase();
      return hay.includes(q);
    });
  }, [persons, search]);

  const selected = persons.find((p) => p.id === personId) ?? null;

  const submit = async (ask: boolean) => {
    const raw = text.trim();
    if (!raw || busy) return;
    setBusy(true);
    setError('');
    try {
      await eventApi.create({ person_id: personId, event_date: date, raw_text: raw });
      setText('');
      setSaved(true);
      if (ask) {
        onClose();
        navigate('/advice', { state: { personId, question: raw } });
        return;
      }
      // Keep the drawer open for back-to-back entries; the saved note confirms it landed.
      window.setTimeout(() => setSaved(false), 2500);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-background/70 p-4 pt-[8vh] backdrop-blur-sm" onClick={onClose}>
      <div
        className="w-full max-w-xl overflow-hidden rounded-2xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-3.5">
          <div className="flex items-center gap-2">
            <NotebookPen className="size-4 text-primary" />
            <h2 className="text-sm font-semibold text-foreground">记一笔</h2>
          </div>
          <button onClick={onClose} aria-label="关闭" className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
            <X className="size-4" />
          </button>
        </div>

        <div className="max-h-[70vh] overflow-y-auto p-5">
          {/* Person: the primary axis of every record. */}
          <div className="mb-4">
            <span className="mb-1.5 block text-xs font-medium text-muted-foreground">关于谁</span>
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="搜索人物…"
                className={`${controlClass} pl-8`}
              />
            </div>
            {filtered.length > 0 ? (
              <ul className="mt-2 grid max-h-44 overflow-y-auto rounded-lg border border-border sm:grid-cols-2">
                {filtered.slice(0, 30).map((p) => {
                  const active = p.id === personId;
                  return (
                    <li key={p.id}>
                      <button
                        onClick={() => setPersonId(p.id)}
                        className={cn(
                          'flex w-full items-center gap-2 px-2.5 py-2 text-left transition-colors',
                          active ? 'bg-primary/10' : 'hover:bg-muted',
                        )}
                      >
                        <Avatar name={p.name} id={p.id} size="sm" className="size-6 text-[10px]" />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-medium text-foreground">{p.name}</span>
                          <span className="block truncate text-xs text-muted-foreground">
                            {[p.relation, p.org_name, p.position].filter(Boolean).join(' · ') || '未分类'}
                          </span>
                        </span>
                        {active ? <span className="size-2 shrink-0 rounded-full bg-primary" /> : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className="mt-2 text-xs text-muted-foreground">没有匹配的人物。</p>
            )}
          </div>

          {/* The writing surface. */}
          <div className="mb-3 flex items-center justify-between gap-3">
            {selected ? (
              <span className="text-xs text-muted-foreground">
                记录给 <span className="font-medium text-foreground">{selected.name}</span>
                {selected.last_event_date ? ` · 上次 ${relativeTime(selected.last_event_date)}` : ' · 尚无记录'}
              </span>
            ) : (
              <span className="text-xs text-amber-600 dark:text-amber-400">未选择人物，请先选择。</span>
            )}
            <input
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              className={`${controlClass} h-8 w-auto px-2 py-0 text-xs text-muted-foreground`}
              title="记录日期（默认今天）"
            />
          </div>

          <Textarea
            ref={inputRef}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void submit(false);
            }}
            rows={5}
            placeholder="今天发生了什么？直接写，AI 会提取摘要、你的感受、对方的反应和承诺。"
            className="resize-y text-base leading-6"
          />

          {error ? (
            <div className="mt-3">
              <ErrorNote>{error}</ErrorNote>
            </div>
          ) : null}
          {saved ? (
            <div className="mt-3">
              <Notice>已记录，AI 正在后台提取。Ctrl/⌘ + Enter 继续写下一条。</Notice>
            </div>
          ) : null}
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-border px-5 py-3.5">
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            <kbd className="rounded border border-border bg-muted px-1.5 py-0.5">Ctrl/⌘</kbd>+
            <kbd className="rounded border border-border bg-muted px-1.5 py-0.5">Enter</kbd>
            提交
          </div>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={onClose}>
              取消
            </Button>
            <Button variant="outline" size="sm" onClick={() => void submit(true)} disabled={busy || !text.trim() || !personId}>
              记录并提问
            </Button>
            <Button size="sm" onClick={() => void submit(false)} disabled={busy || !text.trim() || !personId}>
              {busy ? <Spinner /> : '记录'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}