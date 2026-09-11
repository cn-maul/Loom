import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Building2, NotebookPen, Star } from 'lucide-react';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import TraitList from '../components/TraitList';
import { ErrorNote, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { Textarea } from '../components/ui/textarea';
import { positionApi, organizationApi, followUpApi, personApi, relationshipApi } from '../api/client';
import { Avatar, EmptyState, Field, SectionCard } from '../components/layout';
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

const FOLLOW_UP_LABEL: Record<string, string> = {
  pending: '待办',
  waiting: '等待对方',
  completed: '已完成',
  cancelled: '已取消',
};

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
    return (
      <>
        {error ? (
          <ErrorNote>{error}</ErrorNote>
        ) : (
          <EmptyState icon={<Building2 className="size-6" />} title="找不到这个人物" description="可能已经被删除了。" />
        )}
      </>
    );
  }

  const eventLabels: Record<string, string> = {};
  for (const event of events) eventLabels[event.id] = shortDate(event.event_date);
  const currentOrg = organizations.find((org) => org.id === person.org_id);
  const orgOptions = organizations.filter((org) => !org.archived_at);
  const openFollowUps = followUps.filter((item) => item.status === 'pending' || item.status === 'waiting').length;

  return (
    <div>
      {error ? (
        <div className="mb-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {actionError ? (
        <div className="mb-4">
          <ErrorNote>{actionError}</ErrorNote>
        </div>
      ) : null}

      {/* Headline: who this is, and the three numbers worth glancing at. */}
      <div className="mb-5 rounded-xl border border-border bg-card p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 gap-4">
            <Avatar name={person.name} id={person.id} size="lg" />
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-xl font-semibold tracking-tight text-foreground">{person.name}</h1>
                {person.relation ? (
                  <span className="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">
                    {person.relation}
                  </span>
                ) : null}
                <span className="inline-flex items-center gap-px" title={`重要度 ${person.importance}/5`}>
                  {Array.from({ length: Math.max(1, Math.min(5, person.importance)) }, (_, index) => (
                    <Star key={index} className="size-3.5 fill-amber-400 text-amber-400" />
                  ))}
                </span>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">
                {[currentOrg?.name, person.position].filter(Boolean).join(' · ') || '未填写组织与职位'} · 建档于{' '}
                {fullDate(person.created_at)}
              </p>
              {!editing && person.notes ? (
                <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">{person.notes}</p>
              ) : null}
            </div>
          </div>

          <div className="flex items-start gap-5">
            <div className="flex gap-5 text-center">
              <div>
                <div className="text-xl font-semibold tabular-nums text-foreground">{events.length}</div>
                <div className="text-xs text-muted-foreground">记录</div>
              </div>
              <div>
                <div className="text-xl font-semibold tabular-nums text-foreground">{traits.length}</div>
                <div className="text-xs text-muted-foreground">画像</div>
              </div>
              <div>
                <div className="text-xl font-semibold tabular-nums text-foreground">{openFollowUps}</div>
                <div className="text-xs text-muted-foreground">待办</div>
              </div>
            </div>
            <div className="flex gap-2">
              {editing ? (
                <Button onClick={() => void save()} disabled={saving}>
                  {saving ? '保存中…' : '保存'}
                </Button>
              ) : (
                <Button onClick={() => setEditing(true)} variant="outline">
                  编辑
                </Button>
              )}
              <Button
                onClick={() => void removePerson()}
                variant="outline"
                className="text-muted-foreground hover:border-red-300 hover:text-red-600"
              >
                删除
              </Button>
            </div>
          </div>
        </div>

        {editing ? (
          <div className="mt-4 grid gap-3 border-t border-border pt-4 md:grid-cols-3">
            <Field label="关系">
              <input
                value={draft.relation}
                onChange={(e) => setDraft({ ...draft, relation: e.target.value })}
                className={`${controlClass} h-9`}
              />
            </Field>
            <Field label="职位">
              <input
                value={draft.position}
                onChange={(e) => setDraft({ ...draft, position: e.target.value })}
                placeholder="如：后端工程师"
                className={`${controlClass} h-9`}
              />
            </Field>
            <Field label="所属组织">
              <select
                value={draft.org_id}
                onChange={(e) => setDraft({ ...draft, org_id: e.target.value })}
                className={`${controlClass} h-9 text-muted-foreground`}
              >
                <option value="">无组织</option>
                {orgOptions.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="重要度（1-5）">
              <input
                type="number"
                min={1}
                max={5}
                value={draft.importance}
                onChange={(e) => setDraft({ ...draft, importance: Number(e.target.value) || 3 })}
                className={`${controlClass} h-9`}
              />
            </Field>
            <div className="md:col-span-2">
              <Field label="备注">
                <textarea
                  value={draft.notes}
                  onChange={(e) => setDraft({ ...draft, notes: e.target.value })}
                  rows={2}
                  className={`${controlClass} h-auto`}
                />
              </Field>
            </div>
          </div>
        ) : null}
      </div>

      {/* Two columns: the writing surface on the left, everything derived on the right. */}
      <div className="grid gap-5 lg:grid-cols-3">
        <div className="space-y-5 lg:col-span-2">
          <SectionCard title="记一笔" description="写完之后会自动提取摘要、感受与承诺">
            <QuickRecord personId={person.id} onRecorded={() => void load()} bare />
          </SectionCard>

          <SectionCard
            title="时间线"
            description="以 TA 为主角或参与人的记录"
            actions={
              <Link to="/events" className="text-xs text-muted-foreground hover:text-primary">
                全部记录
              </Link>
            }
          >
            <EventTimeline events={events} />
          </SectionCard>

          <SectionCard title="直接问 AI" description="带着这个问题去建议页，回答会引用这里的记录">
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
          </SectionCard>
        </div>

        <div className="space-y-5">
          <SectionCard
            title="AI 画像"
            description="✓ 采纳，✗ 剔除；新记录会自动更新"
            actions={<span className="text-xs text-muted-foreground">{traits.length} 条</span>}
          >
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
          </SectionCard>

          <SectionCard
            title="组织与任职"
            actions={
              <>
                <Link to="/organizations" className="text-xs text-muted-foreground hover:text-primary">
                  组织管理
                </Link>
                <Button variant="outline" size="sm" onClick={() => setAddingPosition((current) => !current)}>
                  {addingPosition ? '取消' : '添加任职'}
                </Button>
              </>
            }
          >
            {addingPosition ? (
              <div className="mb-4 grid gap-3 rounded-lg border border-dashed border-border p-3 md:grid-cols-2">
                <div className="md:col-span-2">
                  <Field label="组织">
                    <select
                      value={positionForm.org_id}
                      onChange={(e) => setPositionForm({ ...positionForm, org_id: e.target.value })}
                      className={`${controlClass} h-9 text-muted-foreground`}
                    >
                      <option value="">请选择</option>
                      {orgOptions.map((org) => (
                        <option key={org.id} value={org.id}>
                          {org.name}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>
                <Field label="角色 / 职位">
                  <input
                    value={positionForm.role}
                    onChange={(e) => setPositionForm({ ...positionForm, role: e.target.value })}
                    placeholder="如：后端工程师"
                    className={`${controlClass} h-9`}
                  />
                </Field>
                <Field label="起始">
                  <input
                    type="date"
                    value={positionForm.start_date}
                    onChange={(e) => setPositionForm({ ...positionForm, start_date: e.target.value })}
                    className={`${controlClass} h-9`}
                  />
                </Field>
                <Field label="结束" hint="留空表示在职">
                  <input
                    type="date"
                    value={positionForm.end_date}
                    onChange={(e) => setPositionForm({ ...positionForm, end_date: e.target.value })}
                    className={`${controlClass} h-9`}
                  />
                </Field>
                <Field label="备注">
                  <input
                    value={positionForm.notes}
                    onChange={(e) => setPositionForm({ ...positionForm, notes: e.target.value })}
                    className={`${controlClass} h-9`}
                  />
                </Field>
                <div className="md:col-span-2">
                  <Button size="sm" onClick={() => void addPosition()} disabled={savingPosition || !positionForm.org_id}>
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
                    <li
                      key={pos.id}
                      className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-3 py-2"
                    >
                      <div className="min-w-0 text-sm">
                        <Link to="/organizations" className="font-medium text-foreground hover:text-primary">
                          {pos.org_name}
                        </Link>
                        {pos.role ? <span className="ml-2 text-muted-foreground">{pos.role}</span> : null}
                        <span className="ml-2 text-xs text-muted-foreground">
                          <span
                            className={`mr-1 rounded px-1.5 py-0.5 ${
                              current ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-300' : 'bg-muted text-muted-foreground'
                            }`}
                          >
                            {current ? '在职' : '已结束'}
                          </span>
                          {pos.start_date ? pos.start_date : '?'}
                          {pos.end_date ? ` → ${pos.end_date}` : ' 至今'}
                        </span>
                        {pos.notes ? <p className="mt-0.5 text-xs text-muted-foreground">{pos.notes}</p> : null}
                      </div>
                      <div className="flex gap-2">
                        {current ? (
                          <Button size="sm" variant="outline" onClick={() => void endPosition(pos)}>
                            结束
                          </Button>
                        ) : null}
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-muted-foreground hover:text-red-600"
                          onClick={() => void removePosition(pos)}
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
                  <div className="mt-2">
                    <p className="mb-1.5 text-xs text-muted-foreground">同组织现任</p>
                    <ul className="flex flex-wrap gap-1.5">
                      {colleagues.map((member) => (
                        <li key={member.id}>
                          <Link
                            to={`/persons/${member.person_id}`}
                            className="flex items-center gap-1.5 rounded-full border border-border px-2 py-1 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-primary"
                          >
                            <Avatar name={member.person_name ?? ''} id={member.person_id} size="sm" className="size-5 text-[10px]" />
                            {member.person_name}
                            {member.role ? `（${member.role}）` : ''}
                          </Link>
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </div>
            ) : null}
          </SectionCard>

          <SectionCard
            title="关系"
            actions={
              <Link
                to="/relationships"
                state={{ personId: person.id }}
                className="text-xs text-muted-foreground hover:text-primary"
              >
                在图谱中查看
              </Link>
            }
          >
            {relationships.length === 0 ? (
              <p className="text-sm leading-6 text-muted-foreground">
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
                        <Avatar name={otherName ?? ''} id={otherId} size="sm" className="size-6 text-[10px]" />
                        <span className="font-medium text-foreground">
                          {outgoing ? `${person.name} → ` : ''}
                          <Link to={`/persons/${otherId}`} className="hover:text-primary">
                            {otherName}
                          </Link>
                          {outgoing ? '' : ` → ${person.name}`}
                        </span>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{rel.relation_type}</span>
                        {rel.direction === 'undirected' ? <span className="text-xs text-muted-foreground">双向</span> : null}
                        {rel.confirmed ? null : <span className="text-xs text-amber-700 dark:text-amber-400">未确认</span>}
                        {ended ? <span className="text-xs text-muted-foreground">已结束 {rel.end_date}</span> : null}
                        {rel.start_date ? <span className="text-xs text-muted-foreground">起 {rel.start_date}</span> : null}
                      </div>
                      {rel.notes ? <p className="mt-1 text-xs leading-5 text-muted-foreground">{rel.notes}</p> : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </SectionCard>

          <SectionCard
            title="跟进事项"
            actions={
              <Link
                to="/follow-ups"
                state={{ personId: person.id }}
                className="text-xs text-muted-foreground hover:text-primary"
              >
                全部事项
              </Link>
            }
          >
            {followUps.length === 0 ? (
              <p className="text-sm leading-6 text-muted-foreground">
                暂无跟进事项。记录里的承诺或建议里采纳的策略会出现在这里。
              </p>
            ) : (
              <ul className="space-y-2">
                {followUps.map((item) => {
                  const open = item.status === 'pending' || item.status === 'waiting';
                  const overdue = open && item.due_date && item.due_date < todayISO();
                  return (
                    <li
                      key={item.id}
                      className={`rounded-lg border bg-card px-3 py-2 ${
                        overdue ? 'border-red-300/60 dark:border-red-500/40' : 'border-border'
                      }`}
                    >
                      <div className="flex flex-wrap items-center gap-2 text-sm">
                        <span
                          className={`size-1.5 shrink-0 rounded-full ${
                            item.status === 'completed'
                              ? 'bg-emerald-500'
                              : overdue
                                ? 'bg-red-500'
                                : item.status === 'waiting'
                                  ? 'bg-amber-500'
                                  : 'bg-blue-500'
                          }`}
                        />
                        <Link to={`/follow-ups?person=${item.person_id}`} className="font-medium text-foreground hover:text-primary">
                          {item.title}
                        </Link>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                          {FOLLOW_UP_LABEL[item.status] ?? item.status}
                        </span>
                        {item.owner ? <span className="text-xs text-muted-foreground">{item.owner}的球</span> : null}
                        {item.due_date ? (
                          <span className={`text-xs ${overdue ? 'text-red-600 dark:text-red-400' : 'text-muted-foreground'}`}>
                            {overdue ? '逾期 ' : ''}
                            {fullDate(item.due_date)}
                          </span>
                        ) : null}
                      </div>
                      {item.description ? <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.description}</p> : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </SectionCard>
        </div>
      </div>

      {events.length === 0 ? (
        <div className="mt-5">
          <EmptyState
            icon={<NotebookPen className="size-6" />}
            title={`还没有和${person.name}的记录`}
            description="写第一条：什么时候、在哪儿、说了什么。"
          />
        </div>
      ) : null}
    </div>
  );
}
