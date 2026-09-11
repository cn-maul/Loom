import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import TraitList from '../components/TraitList';
import { EmptyState, ErrorNote, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { Textarea } from '../components/ui/textarea';
import { organizationApi, personApi } from '../api/client';
import type { Event, Organization, Person, Trait } from '../api/types';
import { fullDate, shortDate } from '../format';

export default function PersonDetail() {
  const { id = '' } = useParams();
  const navigate = useNavigate();

  const [person, setPerson] = useState<Person | null>(null);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [traits, setTraits] = useState<Trait[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState({ relation: '', importance: 3, notes: '', position: '', org_id: '' });
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      const [personData, traitData, eventData] = await Promise.all([
        personApi.get(id),
        personApi.traits(id),
        personApi.events(id),
      ]);
      setPerson(personData);
      setDraft({
        relation: personData.relation,
        importance: personData.importance,
        notes: personData.notes,
        position: personData.position,
        org_id: personData.org_id,
      });
      setTraits(traitData);
      setEvents(eventData);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    setLoading(true);
    void load();
    organizationApi
      .list()
      .then(setOrganizations)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [load]);

  const save = async () => {
    if (!person) return;
    setSaving(true);
    setError('');
    try {
      const updated = await personApi.update(person.id, {
        ...person,
        relation: draft.relation,
        importance: draft.importance,
        notes: draft.notes,
        position: draft.position,
        org_id: draft.org_id,
      });
      setPerson(updated);
      setEditing(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const removePerson = async () => {
    if (!person) return;
    if (!window.confirm(`删除 ${person.name} 及其全部记录和画像？此操作不可撤销。`)) return;
    setError('');
    try {
      await personApi.delete(person.id);
      navigate('/');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const [question, setQuestion] = useState('');

  if (loading) {
    return <Spinner label="载入人物…" />;
  }

  if (!person) {
    return <>{error ? <ErrorNote>{error}</ErrorNote> : <EmptyState>找不到这个人物。</EmptyState>}</>;
  }

  const eventLabels: Record<string, string> = {};
  for (const event of events) eventLabels[event.id] = shortDate(event.event_date);

  return (
    <div className="space-y-6">
      {error ? <ErrorNote>{error}</ErrorNote> : null}

      <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-foreground">{person.name}</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {person.relation || '未分类'}
              {[person.org_id ? organizations.find((org) => org.id === person.org_id)?.name : '', person.position]
                .filter(Boolean)
                .map((part) => ` · ${part}`)
                .join('')}{' '}
              · 重要度 {person.importance}/5 · 建档于 {fullDate(person.created_at)}
            </p>
            {!editing && person.notes ? <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{person.notes}</p> : null}
          </div>
          <div className="flex gap-2">
            {editing ? (
              <Button onClick={save} disabled={saving}>
                {saving ? '保存中…' : '保存'}
              </Button>
            ) : (
              <Button onClick={() => setEditing(true)} variant="outline">
                编辑
              </Button>
            )}
            <Button
              onClick={removePerson}
              variant="outline"
              className="text-muted-foreground hover:border-red-300 hover:text-red-600"
            >
              删除
            </Button>
          </div>
        </div>

        {editing ? (
          <div className="mt-4 grid gap-3 md:grid-cols-3">
            <label className="text-sm text-muted-foreground">
              关系
              <input
                value={draft.relation}
                onChange={(e) => setDraft({ ...draft, relation: e.target.value })}
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground">
              职位
              <input
                value={draft.position}
                onChange={(e) => setDraft({ ...draft, position: e.target.value })}
                placeholder="如：后端工程师"
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground">
              所属组织
              <select
                value={draft.org_id}
                onChange={(e) => setDraft({ ...draft, org_id: e.target.value })}
                className={`${controlClass} mt-1 text-muted-foreground`}
              >
                <option value="">无组织</option>
                {organizations.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm text-muted-foreground">
              重要度（1-5）
              <input
                type="number"
                min={1}
                max={5}
                value={draft.importance}
                onChange={(e) => setDraft({ ...draft, importance: Number(e.target.value) || 3 })}
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground md:col-span-3">
              备注
              <textarea
                value={draft.notes}
                onChange={(e) => setDraft({ ...draft, notes: e.target.value })}
                rows={2}
                className={`${controlClass} mt-1 h-auto`}
              />
            </label>
          </div>
        ) : null}
      </div>

      <div>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-foreground">AI 画像</h2>
          <span className="text-xs text-muted-foreground">✓ 采纳，✗ 剔除；新记录会自动更新</span>
        </div>
        <TraitList
          traits={traits}
          eventLabels={eventLabels}
          onVerified={(traitId, verified) =>
            setTraits((current) =>
              verified === -1
                ? current.filter((trait) => trait.id !== traitId)
                : current.map((trait) => (trait.id === traitId ? { ...trait, verified } : trait)),
            )
          }
        />
      </div>

      <div>
        <h2 className="mb-3 text-sm font-semibold text-foreground">记一笔</h2>
        <QuickRecord personId={person.id} onRecorded={() => void load()} />
      </div>

      <div>
        <h2 className="mb-3 text-sm font-semibold text-foreground">直接问 AI</h2>
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <Textarea
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            rows={2}
            placeholder={`关于${person.name}的问题，例如「下次找他帮忙该怎么开口」`}
            className="resize-y text-base leading-6"
          />
          <Button
            onClick={() => navigate('/advice', { state: { personId: person.id, question } })}
            disabled={!question.trim()}
            className="mt-3"
          >
            去问 AI
          </Button>
        </div>
      </div>

      <div>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-foreground">时间线</h2>
          <Link to="/" className="text-xs text-muted-foreground hover:text-primary">
            返回首页
          </Link>
        </div>
        <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
          <EventTimeline events={events} />
        </div>
      </div>
    </div>
  );
}
