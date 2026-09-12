import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search } from 'lucide-react';
import { personApi } from '../api/client';
import type { PersonWithActivity } from '../api/types';
import { Avatar } from './layout';
import { ErrorNote, Spinner } from './ui';
import { cn } from '../lib/utils';

/**
 * Global people search (Ctrl/⌘+K). Person lookup was a two-step detour through
 * the directory page; with the palette any page reaches a person in one
 * keystroke. Matching happens client-side: a personal CRM's person list is
 * small, and server search would add latency on every keystroke.
 */
export default function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [query, setQuery] = useState('');
  const [cursor, setCursor] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  // Fresh directory on every open; the list is cheap and stale rows would send
  // the user to a person that may no longer exist.
  useEffect(() => {
    if (!open) return;
    setQuery('');
    setCursor(0);
    setError('');
    setLoading(true);
    personApi
      .list()
      .then(setPersons)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
    const t = window.setTimeout(() => inputRef.current?.focus(), 40);
    return () => window.clearTimeout(t);
  }, [open]);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return persons.slice(0, 8);
    return persons
      .filter((p) => {
        const hay = [p.name, p.relation, p.org_name, p.position].filter(Boolean).join(' ').toLowerCase();
        return hay.includes(q);
      })
      .slice(0, 8);
  }, [persons, query]);

  const choose = (person: PersonWithActivity) => {
    onClose();
    navigate(`/persons/${person.id}`);
  };

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        setCursor((c) => Math.min(c + 1, results.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setCursor((c) => Math.max(c - 1, 0));
      } else if (e.key === 'Enter' && results[cursor]) {
        e.preventDefault();
        choose(results[cursor]);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  // Keep the highlighted row inside the scrolling list.
  useEffect(() => {
    listRef.current?.children[cursor]?.scrollIntoView({ block: 'nearest' });
  }, [cursor]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-background/70 p-4 pt-[10vh] backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="全局搜索"
        className="w-full max-w-lg overflow-hidden rounded-2xl border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="relative border-b border-border">
          <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground/60" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setCursor(0);
            }}
            placeholder="搜索人物…（按分类、组织、职位也能匹配）"
            className="w-full bg-transparent py-4 pl-10 pr-4 text-[15px] text-foreground outline-none"
          />
        </div>

        <div className="max-h-[50vh] overflow-y-auto p-2">
          {error ? (
            <div className="px-3 py-2">
              <ErrorNote>{error}</ErrorNote>
            </div>
          ) : loading ? (
            <div className="px-3 py-2">
              <Spinner label="载入人物…" />
            </div>
          ) : results.length === 0 ? (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">
              {persons.length === 0 ? '还没有人物。' : `没有匹配「${query.trim()}」的人物。`}
            </p>
          ) : (
            <ul ref={listRef}>
              {results.map((person, index) => (
                <li key={person.id}>
                  <button
                    type="button"
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => choose(person)}
                    className={cn(
                      'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors',
                      index === cursor ? 'bg-accent text-accent-foreground' : 'text-foreground',
                    )}
                  >
                    <Avatar name={person.name} id={person.id} size="sm" className="size-7 text-[10px]" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{person.name}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {[person.relation, person.org_name].filter(Boolean).join(' · ') || '未分类'}
                      </span>
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">{person.event_count} 条记录</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex items-center justify-between border-t border-border px-4 py-2 text-[11px] text-muted-foreground">
          <span>↑↓ 选择 · Enter 打开</span>
          <span>Esc 关闭</span>
        </div>
      </div>
    </div>
  );
}
