import { useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';
import { eventApi, organizationApi, personApi } from '../api/client';
import type { Event, IngestReport, Organization, PersonWithActivity } from '../api/types';
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
  /** Show the org → person picker for co-participants (relationship records). */
  participantPicker?: boolean;
}

/** A co-participant attached to the record: a structured relation to a real
 *  person in the database, not a free-text mention. */
type Picked = { person_id: string; person_name: string; org_name: string };

export default function QuickRecord({ personId, onRecorded, onAsk, bare = false, participantPicker = false }: Props) {
  const [text, setText] = useState('');
  const [date, setDate] = useState(todayISO());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [report, setReport] = useState<IngestReport | null>(null);

  // Co-participant picker state: two-level cascade (organisation → person).
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [pickerOrg, setPickerOrg] = useState('');
  const [pickerPerson, setPickerPerson] = useState('');
  const [picked, setPicked] = useState<Picked[]>([]);
  // Names are inserted straight into the textarea at the caret, so we need the
  // element to read/restore the selection around each insertion.
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (!participantPicker) return;
    // The picker is optional decoration; a failed load must not block writing.
    Promise.all([organizationApi.list(), personApi.list()])
      .then(([orgList, personList]) => {
        setOrgs(orgList);
        setPersons(personList);
      })
      .catch(() => undefined);
  }, [participantPicker]);

  const submit = async (ask: boolean) => {
    const raw = text.trim();
    if (!raw || busy) return;

    setBusy(true);
    setError('');
    setReport(null);
    try {
      // Participants travel inside the text as a machine-readable header so the
      // extraction model treats the record as an interaction with these people,
      // plus a structured participants link for queries and the graph.
      const hint = picked.length ? `【共同参与：${picked.map((p) => p.person_name).join('、')}】\n` : '';
      const result = await eventApi.create({ person_id: personId, event_date: date, raw_text: hint + raw });
      if (picked.length) {
        try {
          await eventApi.setParticipants(
            result.event.id,
            picked.map((p) => ({ person_id: p.person_id })),
          );
        } catch (e) {
          result.report.warnings = [
            ...result.report.warnings,
            `记录已保存，但共同参与标记失败：${e instanceof Error ? e.message : String(e)}`,
          ];
        }
      }
      setText('');
      setDate(todayISO());
      setPicked([]);
      setReport(result.report);
      onRecorded(result.event, result.report);
      if (ask) onAsk?.(result.event);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const pickerCandidates = persons.filter(
    (p) =>
      p.id !== personId &&
      !picked.some((x) => x.person_id === p.id) &&
      (pickerOrg === 'none' ? !p.org_id : pickerOrg ? p.org_id === pickerOrg : false),
  );

  /** Put the person's name into the textarea at the caret so the user writes
   *  the record around it, and track them for the structured participant link. */
  const addPicked = () => {
    const person = persons.find((p) => p.id === pickerPerson);
    if (!person) return;
    setPicked((current) =>
      current.some((x) => x.person_id === person.id) ? current : [...current, { person_id: person.id, person_name: person.name, org_name: person.org_name || '' }],
    );
    const el = textareaRef.current;
    const caret = el ? (el.selectionStart ?? text.length) : text.length;
    const before = text.slice(0, caret);
    const after = text.slice(el ? (el.selectionEnd ?? caret) : caret);
    // Keep the name from gluing onto surrounding words.
    const gapBefore = before && !/[\s，。；、：:"'（）]/.test(before.slice(-1)) ? ' ' : '';
    const gapAfter = after && !/^[\s，。；、："'（）]/.test(after) ? ' ' : '';
    setText(`${before}${gapBefore}${person.name}${gapAfter}${after}`);
    const nextCaret = (before + gapBefore + person.name + gapAfter).length;
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(nextCaret, nextCaret);
    });
    setPickerPerson('');
  };

  return (
    <div className={cn(bare ? '' : 'rounded-xl border border-border bg-card p-4 shadow-sm')}>
      {bare ? (
        <div className="mb-2 flex items-center justify-end gap-3">
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
        ref={textareaRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') submit(false);
        }}
        rows={4}
        placeholder="今天发生了什么？直接写，AI 会提取摘要、你的感受、对方的反应和承诺。"
        className="resize-y text-base leading-6"
      />

      {participantPicker ? (
        <div className="mt-3 rounded-lg border border-dashed border-border p-3">
          <div className="flex items-center gap-2">
            <select
              value={pickerOrg}
              onChange={(e) => {
                setPickerOrg(e.target.value);
                setPickerPerson('');
              }}
              className={`${controlClass} h-8 min-w-0 flex-[2] text-xs`}
              aria-label="选择组织"
            >
              <option value="">选择组织…</option>
              {orgs.map((org) => (
                <option key={org.id} value={org.id}>
                  {org.name}
                </option>
              ))}
              <option value="none">未归属人物</option>
            </select>
            <select
              value={pickerPerson}
              onChange={(e) => setPickerPerson(e.target.value)}
              className={`${controlClass} h-8 min-w-0 flex-[2] text-xs`}
              aria-label="选择人名"
              disabled={!pickerOrg}
            >
              <option value="">选择人名…</option>
              {pickerCandidates.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <Button size="sm" variant="outline" className="min-w-0 flex-[1]" onClick={addPicked} disabled={!pickerPerson}>
              添加
            </Button>
          </div>
          {picked.length > 0 ? (
            <div className="mt-1.5 flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
              <span>已添加：</span>
              {picked.map((p, i) => (
                <span key={p.person_id} className="inline-flex items-center gap-0.5 whitespace-nowrap">
                  {p.person_name}
                  <button
                    onClick={() => setPicked((current) => current.filter((x) => x.person_id !== p.person_id))}
                    aria-label={`移除${p.person_name}`}
                    className="text-muted-foreground hover:text-red-600"
                  >
                    <X className="size-3" />
                  </button>
                  {i < picked.length - 1 ? '、' : null}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

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

      {report && report.async ? (
        <div className="mt-3">
          <Notice>已保存。AI 正在后台提取摘要、画像和索引，稍后刷新即可看到结果。</Notice>
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
