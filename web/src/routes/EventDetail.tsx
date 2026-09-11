import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Plus, RefreshCw, Trash2, UserPlus, X } from 'lucide-react';
import { controlClass, EmptyState, ErrorNote, Notice, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { EventStatusBadge } from '../components/EventStatus';
import { eventApi, followUpApi, personApi } from '../api/client';
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
      <div className="space-y-4">
        {error ? <ErrorNote>{error}</ErrorNote> : <EmptyState>找不到这条记录。</EmptyState>}
        <Link to="/events" className="text-sm text-primary">
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

  return (
    <div className="space-y-4">
      <div className="flex items-baseline justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Link to="/events" className="text-primary">
            记录
          </Link>
          <span>/</span>
          <span className="font-mono">{fullDate(event.event_date)}</span>
          <EventStatusBadge event={event} />
          {event.manually_edited === 1 ? (
            <span className="text-xs text-muted-foreground">人工修订过，重新提取会覆盖</span>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            className="h-8 text-muted-foreground"
            disabled={busy === 'retry'}
            onClick={() => void retry()}
          >
            <RefreshCw className={`mr-1 size-3.5 ${busy === 'retry' ? 'animate-spin' : ''}`} />
            重新提取
          </Button>
          <Button
            onClick={() => void remove()}
            variant="outline"
            disabled={busy === 'delete'}
            className="h-8 text-muted-foreground hover:border-red-300 hover:text-red-600"
          >
            <Trash2 className="mr-1 size-3.5" />
            删除
          </Button>
        </div>
      </div>

      {error ? <ErrorNote>{error}</ErrorNote> : null}

      {event.extraction_status === 'failed' ? (
        <Notice>
          提取失败{event.extraction_error ? `：${event.extraction_error}` : ''}。模型配置可用后点「重新提取」，
          也可以自己填写下面的内容。
        </Notice>
      ) : null}

      <div className="space-y-4 rounded-xl border border-border bg-card p-5 shadow-sm">
        {editing ? (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-3">
              <label className="space-y-1">
                <span className="text-xs text-muted-foreground">日期</span>
                <input
                  type="date"
                  value={form.event_date}
                  onChange={(e) => setForm({ ...form, event_date: e.target.value })}
                  className={controlClass}
                />
              </label>
              <label className="space-y-1">
                <span className="text-xs text-muted-foreground">形式</span>
                <select
                  value={form.record_type}
                  onChange={(e) => setForm({ ...form, record_type: e.target.value })}
                  className={`${controlClass} text-muted-foreground`}
                >
                  {RECORD_TYPES.map((type) => (
                    <option key={type} value={type}>
                      {type || '未分类'}
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-1">
                <span className="text-xs text-muted-foreground">渠道</span>
                <select
                  value={form.channel}
                  onChange={(e) => setForm({ ...form, channel: e.target.value })}
                  className={`${controlClass} text-muted-foreground`}
                >
                  {CHANNELS.map((channel) => (
                    <option key={channel} value={channel}>
                      {channel || '未说明'}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <label className="block space-y-1">
              <span className="text-xs text-muted-foreground">摘要</span>
              <input
                value={form.summary}
                onChange={(e) => setForm({ ...form, summary: e.target.value })}
                className={controlClass}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="space-y-1">
                <span className="text-xs text-muted-foreground">我的感受</span>
                <input
                  value={form.my_feeling}
                  onChange={(e) => setForm({ ...form, my_feeling: e.target.value })}
                  className={controlClass}
                />
              </label>
              <label className="space-y-1">
                <span className="text-xs text-muted-foreground">对方反应</span>
                <input
                  value={form.their_reaction}
                  onChange={(e) => setForm({ ...form, their_reaction: e.target.value })}
                  className={controlClass}
                />
              </label>
            </div>
            <label className="block space-y-1">
              <span className="text-xs text-muted-foreground">原文</span>
              <textarea
                value={form.raw_text}
                onChange={(e) => setForm({ ...form, raw_text: e.target.value })}
                rows={6}
                className={`${controlClass} h-auto py-2 leading-7`}
              />
            </label>
            <div className="flex gap-2">
              <Button onClick={() => void save()} disabled={busy === 'save' || !form.raw_text.trim() || !form.event_date}>
                {busy === 'save' ? '保存中…' : '保存'}
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  setEditing(false);
                  setError('');
                }}
                className="text-muted-foreground"
              >
                取消
              </Button>
            </div>
          </div>
        ) : (
          <>
            <div className="flex justify-end">
              <Button variant="ghost" onClick={() => setEditing(true)} className="h-8 text-muted-foreground">
                编辑
              </Button>
            </div>

            <dl className="space-y-2">
              {[
                { label: '摘要', value: event.summary },
                { label: '我的感受', value: event.my_feeling },
                { label: '对方反应', value: event.their_reaction },
              ].map((field) =>
                field.value ? (
                  <div key={field.label} className="flex gap-3 text-sm">
                    <dt className="w-20 shrink-0 text-muted-foreground">{field.label}</dt>
                    <dd className="leading-7 text-foreground">{field.value}</dd>
                  </div>
                ) : null,
              )}
              {!event.summary && !event.my_feeling && !event.their_reaction ? (
                <p className="text-sm text-muted-foreground">这条记录还没有 AI 提取结果，可以自己填写。</p>
              ) : null}
            </dl>

            {event.promises.length > 0 ? (
              <div className="space-y-2">
                <h2 className="text-sm font-medium text-muted-foreground">承诺</h2>
                <ul className="space-y-2">
                  {event.promises.map((promise, index) => {
                    const turned = linked.some((item) => item.title === promise.what);
                    return (
                      <li key={index} className="flex flex-wrap items-center gap-2 text-sm">
                        <span className="rounded-full bg-primary/10 px-3 py-1 text-xs text-primary">
                          {promise.who || '对方'}：{promise.what}
                          {promise.deadline ? `（${promise.deadline}）` : ''}
                        </span>
                        {turned ? (
                          <span className="text-xs text-muted-foreground">已转为跟进事项</span>
                        ) : (
                          <Button
                            variant="outline"
                            className="h-7 px-2 text-xs text-muted-foreground"
                            disabled={busy === 'followup'}
                            onClick={() => void turnIntoFollowUp(promise)}
                          >
                            <Plus className="mr-1 size-3" />
                            转为跟进
                          </Button>
                        )}
                      </li>
                    );
                  })}
                </ul>
              </div>
            ) : null}

            <div>
              <h2 className="mb-1 text-sm font-medium text-muted-foreground">原文</h2>
              <p className="whitespace-pre-wrap rounded-lg bg-muted px-3 py-2 text-sm leading-7 text-foreground">
                {event.raw_text}
              </p>
            </div>

            <p className="text-xs text-muted-foreground">
              记录于 {fullDate(event.created_at)}
              {[event.record_type, event.channel].filter(Boolean).length > 0
                ? ` · ${[event.record_type, event.channel].filter(Boolean).join(' · ')}`
                : ''}
            </p>
          </>
        )}
      </div>

      <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold text-foreground">参与人（{participants.length}）</h2>
        <ul className="mb-3 space-y-1">
          {participants.map((participant) => (
            <li key={participant.person_id} className="flex items-center justify-between gap-2 text-sm">
              <Link to={`/persons/${participant.person_id}`} className="text-primary hover:underline">
                {participant.person_name || nameOf(participant.person_id)}
                {participant.person_id === event.person_id ? <span className="ml-1 text-xs text-muted-foreground">（锚定人）</span> : null}
              </Link>
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">{participant.role || '在场'}</span>
                <button
                  onClick={() => void removeParticipant(participant.person_id)}
                  disabled={busy === 'participant'}
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-red-600"
                  title="移除参与人"
                >
                  <X className="size-3.5" />
                </button>
              </div>
            </li>
          ))}
        </ul>

        {available.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2">
            <select
              value={adding.person_id}
              onChange={(e) => setAdding({ ...adding, person_id: e.target.value })}
              className={`${controlClass} w-auto text-muted-foreground`}
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
              className={`${controlClass} w-40`}
            />
            <Button
              variant="outline"
              className="h-9"
              disabled={!adding.person_id || busy === 'participant'}
              onClick={() => void addParticipant()}
            >
              <UserPlus className="mr-1 size-3.5" />
              添加
            </Button>
          </div>
        ) : null}
      </div>

      {linked.length > 0 ? (
        <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
          <h2 className="mb-2 text-sm font-semibold text-foreground">由这条记录转出的跟进</h2>
          <ul className="space-y-1 text-sm">
            {linked.map((item) => (
              <li key={item.id} className="flex items-center gap-2">
                <Link to="/follow-ups" className="text-primary hover:underline">
                  {item.title}
                </Link>
                <span className="text-xs text-muted-foreground">{item.status === 'completed' ? '已完成' : item.owner}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
