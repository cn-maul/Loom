import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import TraitList from '../components/TraitList';
import { EmptyState, ErrorNote, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { Textarea } from '../components/ui/textarea';
import { positionApi, organizationApi, followUpApi, personApi, relationshipApi } from '../api/client';
import type {
  Event,
  FollowUp,
  OrgPositionLink,
  Organization,
  Person,
  RelationshipLink,
  Trait,
} from '../api/types';
import { fullDate, shortDate, todayISO } from '../format';

const EMPTY_POSITION = { org_id: '', role: '', start_date: '', end_date: '', notes: '' };

export default function PersonDetail() {
  const { id = '' } = useParams();
  const navigate = useNavigate();

  const [person, setPerson] = useState<Person | null>(null);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [traits, setTraits] = useState<Trait[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [followUps, setFollowUps] = useState<FollowUp[]>([]);
  const [positions, setPositions] = useState<OrgPositionLink[]>([]);
  const [colleagues, setColleagues] = useState<OrgPositionLink[]>([]);
  const [relationships, setRelationships] = useState<RelationshipLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState({ relation: '', importance: 3, notes: '', position: '', org_id: '' });
  const [saving, setSaving] = useState(false);

  const [question, setQuestion] = useState('');
  const [positionForm, setPositionForm] = useState(EMPTY_POSITION);
  const [addingPosition, setAddingPosition] = useState(false);
  const [savingPosition, setSavingPosition] = useState(false);

  const reloadPositions = useCallback(async () => {
    if (!id) return;
    setPositions(await positionApi.byPerson(id));
  }, [id]);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      const [personData, traitData, eventData, followUpData, positionData, relationData] = await Promise.all([
        personApi.get(id),
        personApi.traits(id),
        personApi.events(id),
        followUpApi.byPerson(id),
        positionApi.byPerson(id),
        relationshipApi.byPerson(id),
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
      setFollowUps(followUpData);
      setPositions(positionData);
      setRelationships(relationData);
      // Colleagues are derived from the person's own posting, so they can only
      // be fetched once the person is known.
      if (personData.org_id) {
        const members = await organizationApi.members(personData.org_id, true);
        setColleagues(members.filter((member) => member.person_id !== id));
      } else {
        setColleagues([]);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    setLoading(true);
    setError('');
    setActionError('');
    // Everything below is per-person state: leaving it behind would show the
    // previous person's draft question or edit form on the next one.
    setQuestion('');
    setEditing(false);
    setAddingPosition(false);
    setPositionForm(EMPTY_POSITION);
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
      await load();
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

  const addPosition = async () => {
    if (!person || !positionForm.org_id) return;
    setSavingPosition(true);
    setActionError('');
    try {
      await positionApi.create(person.id, {
        org_id: positionForm.org_id,
        role: positionForm.role || undefined,
        start_date: positionForm.start_date || undefined,
        end_date: positionForm.end_date || undefined,
        notes: positionForm.notes || undefined,
      });
      setPositionForm(EMPTY_POSITION);
      setAddingPosition(false);
      await reloadPositions();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setSavingPosition(false);
    }
  };

  // Ending a stint keeps the row: the history of where someone worked is part
  // of the record, so only the end date is written.
  const endPosition = async (pos: OrgPositionLink) => {
    if (!person) return;
    setActionError('');
    try {
      await positionApi.update(pos.id, {
        person_id: pos.person_id,
        org_id: pos.org_id,
        role: pos.role,
        start_date: pos.start_date,
        end_date: todayISO(),
        notes: pos.notes,
      });
      await reloadPositions();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    }
  };

  const removePosition = async (pos: OrgPositionLink) => {
    if (!window.confirm(`删除「${pos.org_name}」这段任职记录？此操作不可撤销。`)) return;
    setActionError('');
    try {
      await positionApi.remove(pos.id);
      await reloadPositions();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) {
    return <Spinner label="载入人物…" />;
  }

  if (!person) {
    return <>{error ? <ErrorNote>{error}</ErrorNote> : <EmptyState>找不到这个人物。</EmptyState>}</>;
  }

  const eventLabels: Record<string, string> = {};
  for (const event of events) eventLabels[event.id] = shortDate(event.event_date);
  const currentOrg = organizations.find((org) => org.id === person.org_id);
  const orgOptions = organizations.filter((org) => !org.archived_at);

  return (
    <div className="space-y-6">
      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {actionError ? <ErrorNote>{actionError}</ErrorNote> : null}

      <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold text-foreground">{person.name}</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {person.relation || '未分类'}
              {[currentOrg?.name ?? '', person.position]
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
                {orgOptions.map((org) => (
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
          staleAction={(trait) =>
            trait.source_event_ids.length > 0 ? (
              <Link to={`/events/${trait.source_event_ids[0]}`} className="underline">
                去来源记录重新提取
              </Link>
            ) : null
          }
        />
      </div>

      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-foreground">组织与任职</h2>
          <div className="flex items-center gap-3">
            <Link to="/organizations" className="text-xs text-muted-foreground hover:text-primary">
              组织管理
            </Link>
            <Button variant="outline" onClick={() => setAddingPosition((current) => !current)}>
              {addingPosition ? '取消' : '添加任职'}
            </Button>
          </div>
        </div>

        {addingPosition ? (
          <div className="mb-4 grid gap-3 rounded-lg border border-dashed border-border p-3 md:grid-cols-5">
            <label className="text-sm text-muted-foreground md:col-span-2">
              组织
              <select
                value={positionForm.org_id}
                onChange={(e) => setPositionForm({ ...positionForm, org_id: e.target.value })}
                className={`${controlClass} mt-1 text-muted-foreground`}
              >
                <option value="">请选择</option>
                {orgOptions.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-sm text-muted-foreground">
              角色/职位
              <input
                value={positionForm.role}
                onChange={(e) => setPositionForm({ ...positionForm, role: e.target.value })}
                placeholder="如：后端工程师"
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground">
              起始
              <input
                type="date"
                value={positionForm.start_date}
                onChange={(e) => setPositionForm({ ...positionForm, start_date: e.target.value })}
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground">
              结束（留空=在职）
              <input
                type="date"
                value={positionForm.end_date}
                onChange={(e) => setPositionForm({ ...positionForm, end_date: e.target.value })}
                className={`${controlClass} mt-1`}
              />
            </label>
            <label className="text-sm text-muted-foreground md:col-span-5">
              备注
              <input
                value={positionForm.notes}
                onChange={(e) => setPositionForm({ ...positionForm, notes: e.target.value })}
                className={`${controlClass} mt-1`}
              />
            </label>
            <div className="md:col-span-5">
              <Button onClick={addPosition} disabled={savingPosition || !positionForm.org_id}>
                {savingPosition ? '保存中…' : '保存任职'}
              </Button>
            </div>
          </div>
        ) : null}

        {positions.length === 0 ? (
          <p className="text-sm text-muted-foreground">还没有任职记录。添加之后，这个人的工作经历会完整保留。</p>
        ) : (
          <ul className="space-y-2">
            {positions.map((pos) => {
              const current = !pos.end_date;
              return (
                <li key={pos.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-3 py-2">
                  <div className="min-w-0 text-sm">
                    <Link to="/organizations" className="font-medium text-foreground hover:text-primary">
                      {pos.org_name}
                    </Link>
                    {pos.role ? <span className="ml-2 text-muted-foreground">{pos.role}</span> : null}
                    <span className={`ml-2 text-xs ${current ? 'text-muted-foreground' : 'text-muted-foreground/70'}`}>
                      {current ? '在职' : '已结束'}
                      {pos.start_date ? ` · ${pos.start_date}` : ''}
                      {pos.end_date ? ` → ${pos.end_date}` : ''}
                    </span>
                    {pos.notes ? <p className="mt-0.5 text-xs text-muted-foreground">{pos.notes}</p> : null}
                  </div>
                  <div className="flex gap-2">
                    {current ? (
                      <Button variant="outline" onClick={() => endPosition(pos)}>
                        结束
                      </Button>
                    ) : null}
                    <Button
                      variant="outline"
                      className="text-muted-foreground hover:border-red-300 hover:text-red-600"
                      onClick={() => removePosition(pos)}
                    >
                      删除
                    </Button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}

        {currentOrg ? (
          <div className="mt-4 border-t border-border pt-3">
            <p className="text-sm text-foreground">
              {currentOrg.name}
              {currentOrg.kind ? <span className="ml-2 text-xs text-muted-foreground">{currentOrg.kind}</span> : null}
            </p>
            {currentOrg.description ? (
              <p className="mt-1 text-sm leading-6 text-muted-foreground">{currentOrg.description}</p>
            ) : null}
            {colleagues.length > 0 ? (
              <p className="mt-2 text-xs text-muted-foreground">
                同组织：
                {colleagues.map((member, index) => (
                  <span key={member.id}>
                    {index > 0 ? '、' : ''}
                    <Link to={`/persons/${member.person_id}`} className="hover:text-primary">
                      {member.person_name}
                      {member.role ? `（${member.role}）` : ''}
                    </Link>
                  </span>
                ))}
              </p>
            ) : null}
          </div>
        ) : null}
      </div>

      <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-foreground">关系</h2>
          <Link to="/relationships" state={{ personId: person.id }} className="text-xs text-muted-foreground hover:text-primary">
            在图谱中查看
          </Link>
        </div>
        {relationships.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            还没有关系记录。关系只能在图谱页手动添加——同场出现只算共同经历，不会自动推断成关系。
          </p>
        ) : (
          <ul className="space-y-2">
            {relationships.map((rel) => {
              const outgoing = rel.from_person_id === person.id;
              const otherId = outgoing ? rel.to_person_id : rel.from_person_id;
              const otherName = outgoing ? rel.to_person_name : rel.from_person_name;
              const ended = Boolean(rel.end_date);
              return (
                <li key={rel.id} className="rounded-lg border border-border px-3 py-2 text-sm">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-foreground">
                      {outgoing ? `${person.name} → ` : ''}
                      <Link to={`/persons/${otherId}`} className="hover:text-primary">
                        {otherName}
                      </Link>
                      {outgoing ? '' : ` → ${person.name}`}
                    </span>
                    <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{rel.relation_type}</span>
                    {rel.direction === 'undirected' ? (
                      <span className="text-xs text-muted-foreground">双向</span>
                    ) : null}
                    {rel.confirmed ? null : (
                      <span className="text-xs text-amber-700 dark:text-amber-400">未确认</span>
                    )}
                    {ended ? <span className="text-xs text-muted-foreground">已结束 {rel.end_date}</span> : null}
                    {rel.start_date ? <span className="text-xs text-muted-foreground">起 {rel.start_date}</span> : null}
                  </div>
                  {rel.notes ? <p className="mt-1 text-xs leading-5 text-muted-foreground">{rel.notes}</p> : null}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold text-foreground">跟进事项</h2>
          <Link
            to="/follow-ups"
            state={{ personId: person.id }}
            className="text-xs text-muted-foreground hover:text-primary"
          >
            全部事项
          </Link>
        </div>
        {followUps.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border bg-card p-6 text-center text-sm text-muted-foreground">
            暂无跟进事项。记录里的承诺或建议里采纳的策略会出现在这里。
          </div>
        ) : (
          <ul className="space-y-2">
            {followUps.map((item) => {
              const open = item.status === 'pending' || item.status === 'waiting';
              const overdue = open && item.due_date && item.due_date < todayISO();
              return (
                <li
                  key={item.id}
                  className={`rounded-xl border bg-card p-3 shadow-sm ${
                    overdue ? 'border-red-300/60 dark:border-red-500/40' : 'border-border'
                  }`}
                >
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-medium text-foreground">{item.title}</span>
                    <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                      {item.status === 'pending' ? '待办' : item.status === 'waiting' ? '等待对方' : item.status === 'completed' ? '已完成' : '已取消'}
                    </span>
                    {item.owner ? <span className="text-xs text-muted-foreground">{item.owner}的球</span> : null}
                    {item.due_date ? (
                      <span className={`text-xs ${overdue ? 'text-red-600 dark:text-red-400' : 'text-muted-foreground'}`}>
                        {fullDate(item.due_date)}
                      </span>
                    ) : null}
                  </div>
                  {item.description ? (
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.description}</p>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
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
