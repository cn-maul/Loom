import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Plus, RefreshCw, Trash2, UserPlus, X } from 'lucide-react';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { EventStatusBadge } from '../components/EventStatus';
import { eventApi, followUpApi, personApi } from '../api/client';
import { EmptyState, Field, PageHeader, SectionCard } from '../components/layout';
import type { Event, EventParticipant, FollowUp, PersonWithActivity } from '../api/types';
import { fullDate } from '../format';

const RECORD_TYPES = ['', '见面', '通话', '消息', '邮件', '会议'];
const CHANNELS = ['', '当面', '微信', '电话', '邮件', '其他'];
const OWNERS = ['我', '对方'];

export default function EventDetail() {
  const { id = '' } = useParams();
  const navigate = useNavigate();

  const [event, setEvent] = useState<Event | null>(null);
  const [participants, setParticipants] = useState<EventParticipant[]>([]);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [linked, setLinked] = useState<FollowUp[]>([]);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');

  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    event_date: '',
    record_type: '',
    channel: '',
    summary: '',
    my_feeling: '',
    their_reaction: '',
    raw_text: '',
  });
  const [adding, setAdding] = useState({ person_id: '', role: '' });

  const loadLinked = useCallback(async () => {
    // Follow-ups carry the record they came from, so the ones already turned
    // out of this note can be listed without a dedicated endpoint.
    const items = await followUpApi.list();
    setLinked(items.filter((item) => item.source_event_id === id));
  }, [id]);

  const load = useCallback(async () => {
    try {
      const [data, attendance, people] = await Promise.all([
        eventApi.get(id),
        eventApi.participants(id),
        personApi.list(),
      ]);
      setEvent(data);
      setParticipants(attendance);
      setPersons(people);
      setForm({
        event_date: data.event_date,
        record_type: data.record_type ?? '',
        channel: data.channel ?? '',
        summary: data.summary ?? '',
        my_feeling: data.my_feeling ?? '',
        their_reaction: data.their_reaction ?? '',
        raw_text: data.raw_text,
      });
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    // Switching records must not leave the previous one's open editor or
    // half-filled attendee picker behind; load() refills the form itself.
    setEditing(false);
    setAdding({ person_id: '', role: '' });
    setError('');
    setLoading(true);
    void load();
    void loadLinked();
  }, [load, loadLinked]);

  const save = async () => {
    if (!event) return;
    setBusy('save');
    try {
      // promises travel along unchanged: the edit form does not own them, and
      // leaving the field out would clear the extraction's commitments.
      const updated = await eventApi.update(event.id, {
        ...form,
        person_id: event.person_id,
        promises: event.promises,
      });
      setEvent(updated);
      setEditing(false);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const retry = async () => {
    if (!event) return;
    const force = event.manually_edited === 1;
    if (force && !window.confirm('这条记录的提取结果你手动改过，重新提取会覆盖你的修改。继续？')) return;
    setBusy('retry');
    try {
      const result = await eventApi.retryExtract(event.id, force);
      setEvent(result.event);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const addParticipant = async () => {
    if (!event || !adding.person_id) return;
    setBusy('participant');
    try {
      const next = await eventApi.setParticipants(event.id, [
        ...participants.map((p) => ({ person_id: p.person_id, role: p.role })),
        { person_id: adding.person_id, role: adding.role.trim() },
      ]);
      setParticipants(next);
      setAdding({ person_id: '', role: '' });
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const removeParticipant = async (personId: string) => {
    if (!event) return;
    if (participants.length <= 1) {
      setError('至少保留一位参与人，否则这条记录在任何人的时间线里都找不到。');
      return;
    }
    setBusy('participant');
    try {
      const next = await eventApi.removeParticipant(event.id, personId);
      setParticipants(next);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  // A promise becomes work only when someone says so: the button is what turns
  // a sentence in the note into an item on the follow-up list.
  const turnIntoFollowUp = async (promise: { who: string; what: string; deadline: string }) => {
    if (!event) return;
    setBusy('followup');
    try {
      await followUpApi.create({
        person_id: event.person_id,
        title: promise.what,
        description: `来自 ${fullDate(event.event_date)} 的记录`,
        owner: OWNERS.includes(promise.who) ? promise.who : '对方',
        due_text: promise.deadline,
        source_event_id: event.id,
      });
      await loadLinked();
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy('');
    }
  };

  const remove = async () => {
    if (!event) return;
    if (!window.confirm('删除这条记录？相关的向量索引会一起清掉。')) return;
    setBusy('delete');
    try {
      await eventApi.delete(event.id);
      navigate(`/persons/${event.person_id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setBusy('');
    }
  };

  if (loading) {
    return <Spinner label="载入记录…" />;
  }

  if (!event) {
    return (
      <div className="space-y-9">
        {error ? <ErrorNote>{error}</ErrorNote> : <EmptyState title="找不到这条记录" />}
        <Link to="/events" className="text-[13.5px] text-primary hover:underline">
          返回记录列表
        </Link>
      </div>
    );
  }

  const nameOf = (personId: string) =>
    participants.find((p) => p.person_id === personId)?.person_name ??
    persons.find((p) => p.id === personId)?.name ??
    personId;
  const available = persons.filter((p) => !participants.some((q) => q.person_id === p.id));
  const field = `${controlClass} h-9 text-[13px] text-ink-2`;

  return (
    <div>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-2">
            <Link to="/events" className="text-[13px] font-normal text-ink-3 transition-colors hover:text-primary">
              记录
            </Link>
            <span className="text-[13px] font-normal text-ink-4">/</span>
            <span className="font-mono tabular-nums">{fullDate(event.event_date)}</span>
            <EventStatusBadge event={event} />
            {event.manually_edited === 1 ? (
              <span className="text-[12px] font-normal text-ink-3">人工修订过，重新提取会覆盖</span>
            ) : null}
          </span>
        }
        actions={
          <>
            <Button variant="outline" size="sm" className="text-ink-2" disabled={busy === 'retry'} onClick={() => void retry()}>
              <RefreshCw className={`size-3.5 ${busy === 'retry' ? 'animate-spin' : ''}`} />
              重新提取
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-destructive hover:bg-destructive/10"
              disabled={busy === 'delete'}
              onClick={() => void remove()}
            >
              <Trash2 className="size-3.5" />
              删除
            </Button>
          </>
        }
      />

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}

      {event.extraction_status === 'failed' ? (
        <div className="mb-5">
          <Notice>
            提取失败{event.extraction_error ? `：${event.extraction_error}` : ''}。模型配置可用后点「重新提取」，
            也可以自己填写下面的内容。
          </Notice>
        </div>
      ) : null}

      <div className="grid gap-5 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <SectionCard
            title="记录内容"
            actions={
              editing ? null : (
                <Button variant="ghost" size="sm" onClick={() => setEditing(true)} className="text-ink-3">
                  编辑
                </Button>
              )
            }
          >
            {editing ? (
              <div className="space-y-3.5">
                <div className="grid gap-3 sm:grid-cols-3">
                  <Field label="日期">
                    <input
                      type="date"
                      value={form.event_date}
                      onChange={(e) => setForm({ ...form, event_date: e.target.value })}
                      className={controlClass}
                    />
                  </Field>
                  <Field label="形式">
                    <select
                      value={form.record_type}
                      onChange={(e) => setForm({ ...form, record_type: e.target.value })}
                      className={`${controlClass} text-ink-2`}
                    >
                      {RECORD_TYPES.map((type) => (
                        <option key={type} value={type}>
                          {type || '未分类'}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <Field label="渠道">
                    <select
                      value={form.channel}
                      onChange={(e) => setForm({ ...form, channel: e.target.value })}
                      className={`${controlClass} text-ink-2`}
                    >
                      {CHANNELS.map((channel) => (
                        <option key={channel} value={channel}>
                          {channel || '未说明'}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>

                <Field label="摘要">
                  <input
                    value={form.summary}
                    onChange={(e) => setForm({ ...form, summary: e.target.value })}
                    className={controlClass}
                  />
                </Field>

                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label="我的感受">
                    <input
                      value={form.my_feeling}
                      onChange={(e) => setForm({ ...form, my_feeling: e.target.value })}
                      className={controlClass}
                    />
                  </Field>
                  <Field label="对方反应">
                    <input
                      value={form.their_reaction}
                      onChange={(e) => setForm({ ...form, their_reaction: e.target.value })}
                      className={controlClass}
                    />
                  </Field>
                </div>

                <Field label="原文">
                  <textarea
                    value={form.raw_text}
                    onChange={(e) => setForm({ ...form, raw_text: e.target.value })}
                    rows={6}
                    className={`${controlClass} h-auto resize-y py-3 leading-[1.85]`}
                  />
                </Field>

                <div className="flex gap-2">
                  <Button
                    onClick={() => void save()}
                    disabled={busy === 'save' || !form.raw_text.trim() || !form.event_date}
                  >
                    {busy === 'save' ? '保存中…' : '保存'}
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setEditing(false);
                      setError('');
                    }}
                    className="text-ink-3"
                  >
                    取消
                  </Button>
                </div>
              </div>
            ) : (
              <>
                <dl className="space-y-2.5">
                  {[
                    { label: '摘要', value: event.summary },
                    { label: '我的感受', value: event.my_feeling },
                    { label: '对方反应', value: event.their_reaction },
                  ].map((item) =>
                    item.value ? (
                      <div key={item.label} className="flex gap-3">
                        <dt className="w-20 shrink-0 text-[13px] text-ink-3">{item.label}</dt>
                        <dd className="text-[13.5px] leading-[1.85] text-ink-2">{item.value}</dd>
                      </div>
                    ) : null,
                  )}
                  {!event.summary && !event.my_feeling && !event.their_reaction ? (
                    <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                      这条记录还没有 AI 提取结果，可以自己填写。
                    </p>
                  ) : null}
                </dl>

                {event.promises.length > 0 ? (
                  <div className="mt-4 space-y-2.5 border-t border-hairline pt-4">
                    <h2 className="text-[12px] font-medium text-ink-3">承诺</h2>
                    <ul className="space-y-2.5">
                      {event.promises.map((promise, index) => {
                        const turned = linked.some((item) => item.title === promise.what);
                        return (
                          <li key={index} className="flex flex-wrap items-center gap-2">
                            <span className="rounded-full bg-primary/10 px-2.5 py-[3px] text-[11.5px] text-primary">
                              {promise.who || '对方'}：{promise.what}
                              {promise.deadline ? `（${promise.deadline}）` : ''}
                            </span>
                            {turned ? (
                              <span className="text-[12px] text-ink-3">已转为跟进事项</span>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-7 px-2.5 text-[12px] text-ink-2"
                                disabled={busy === 'followup'}
                                onClick={() => void turnIntoFollowUp(promise)}
                              >
                                <Plus className="size-3" />
                                转为跟进
                              </Button>
                            )}
                          </li>
                        );
                      })}
                    </ul>
                  </div>
                ) : null}

                <div className="mt-4 border-t border-hairline pt-4">
                  <h2 className="mb-1.5 text-[12px] font-medium text-ink-3">原文</h2>
                  <p className="whitespace-pre-wrap rounded-md bg-fill px-3.5 py-3 text-[13.5px] leading-[1.85] text-ink-2">
                    {event.raw_text}
                  </p>
                </div>

                <p className="mt-4 text-[12px] tabular-nums text-ink-3">
                  记录于 {fullDate(event.created_at)}
                  {[event.record_type, event.channel].filter(Boolean).length > 0
                    ? ` · ${[event.record_type, event.channel].filter(Boolean).join(' · ')}`
                    : ''}
                </p>
              </>
            )}
          </SectionCard>
        </div>

        <div className="space-y-9">
          <SectionCard title={`参与人（${participants.length}）`} bodyClassName="p-0">
            <ul className="panel-rows">
              {participants.map((participant) => (
                <li key={participant.person_id} className="flex items-center justify-between gap-2 px-5 py-3">
                  <Link
                    to={`/persons/${participant.person_id}`}
                    className="text-[13.5px] font-medium text-primary hover:underline"
                  >
                    {participant.person_name || nameOf(participant.person_id)}
                    {participant.person_id === event.person_id ? (
                      <span className="ml-1.5 text-[12px] font-normal text-ink-3">（锚定人）</span>
                    ) : null}
                  </Link>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <span className="text-[12px] text-ink-3">{participant.role || '在场'}</span>
                    <button
                      onClick={() => void removeParticipant(participant.person_id)}
                      disabled={busy === 'participant'}
                      className="flex size-7 items-center justify-center rounded-full text-ink-4 transition-colors hover:bg-fill hover:text-destructive"
                      title="移除参与人"
                      aria-label={`移除${participant.person_name || nameOf(participant.person_id)}`}
                    >
                      <X className="size-3.5" />
                    </button>
                  </div>
                </li>
              ))}
            </ul>

            {available.length > 0 ? (
              <div className="flex flex-wrap items-center gap-2 px-5 pb-5 pt-3">
                <select
                  value={adding.person_id}
                  onChange={(e) => setAdding({ ...adding, person_id: e.target.value })}
                  className={`${field} w-auto`}
                >
                  <option value="">添加参与人…</option>
                  {available.map((person) => (
                    <option key={person.id} value={person.id}>
                      {person.name}
                    </option>
                  ))}
                </select>
                <input
                  value={adding.role}
                  onChange={(e) => setAdding({ ...adding, role: e.target.value })}
                  placeholder="参与方式（可选）"
                  className={`${field} w-40`}
                />
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!adding.person_id || busy === 'participant'}
                  onClick={() => void addParticipant()}
                >
                  <UserPlus className="size-3.5" />
                  添加
                </Button>
              </div>
            ) : null}
          </SectionCard>

          {linked.length > 0 ? (
            <SectionCard title="由这条记录转出的跟进" bodyClassName="p-0">
              <ul className="panel-rows">
                {linked.map((item) => (
                  <li key={item.id} className="flex items-center gap-2 px-5 py-3">
                    <span className="text-[13.5px] font-medium text-foreground">{item.title}</span>
                    <span className="text-[12px] text-ink-3">{item.status === 'completed' ? '已完成' : item.owner}</span>
                  </li>
                ))}
              </ul>
            </SectionCard>
          ) : null}
        </div>
      </div>
    </div>
  );
}
