import { useCallback, useEffect, useSyncExternalStore, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Building2, MessageSquareText, NotebookPen, X } from 'lucide-react';
import EventTimeline from '../components/EventTimeline';
import QuickRecord from '../components/QuickRecord';
import TraitList from '../components/TraitList';
import { ErrorNote, Spinner } from '../components/ui';
import { Badge } from '../components/ui/badge';
import { Button } from '../components/ui/button';
import { Textarea } from '../components/ui/textarea';
import { organizationApi, personApi, reportApi } from '../api/client';
import { getPortraitState, startPortrait, subscribePortrait } from '../lib/portraitStore';
import { Avatar, EmptyState, SectionCard } from '../components/layout';
import { useEnterState } from '../lib/overlay';
import type { Event, Organization, Person, Trait } from '../api/types';
import { fullDate, shortDate, todayISO } from '../format';

/** What the 人物概览 card shows: one AI-written paragraph over a recent window. */
type ProfileSnapshot = {
  summary: string;
  status: 'succeeded' | 'failed';
  failure_reason?: string;
  start: string;
  end: string;
  generated_at: string;
};

/** Local-calendar YYYY-MM-DD; toISOString would drift a day off in GMT+8 mornings. */
const localDate = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

export default function PersonDetail() {
  const { id = '' } = useParams();
  const navigate = useNavigate();

  const [person, setPerson] = useState<Person | null>(null);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [traits, setTraits] = useState<Trait[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');

  // The AI profile paragraph: latest snapshot if one exists, generated on demand.
  const [profile, setProfile] = useState<ProfileSnapshot | null>(null);
  // In-flight/finished generation lives in a module-level store: leaving the
  // page mid-generation keeps it running, and coming back picks it up here.
  const portrait = useSyncExternalStore(
    useCallback((cb: () => void) => subscribePortrait(id, cb), [id]),
    () => getPortraitState(id),
  );
  const profileBusy = portrait.status === 'running';

  // Popup surfaces: recording a moment and asking the AI about this person.
  const [recordOpen, setRecordOpen] = useState(false);
  const [askOpen, setAskOpen] = useState(false);
  const [question, setQuestion] = useState('');
  // Both layers mount already-open, so they need the frame to transition from.
  const recordState = useEnterState(recordOpen);
  const askState = useEnterState(askOpen);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      const [personData, traitData, eventData, reportList] = await Promise.all([
        personApi.get(id),
        personApi.traits(id),
        personApi.events(id),
        reportApi.list(id),
      ]);
      setPerson(personData);
      setTraits(traitData);
      setEvents(eventData);
      // The newest snapshot stands in for the profile paragraph until the user
      // asks for a fresh one; a failed generation still shows its reason.
      const latest = reportList.find((r) => r.status === 'succeeded') ?? reportList[0] ?? null;
      setProfile(
        latest
          ? {
              summary: latest.summary,
              status: latest.status,
              failure_reason: latest.failure_reason,
              start: latest.start,
              end: latest.end,
              generated_at: latest.generated_at,
            }
          : null,
      );
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
    // previous person's draft question on the next one.
    setQuestion('');
    setRecordOpen(false);
    setAskOpen(false);
    void load();
    organizationApi
      .list()
      .then(setOrganizations)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [load]);

  // Escape closes either popup; the rest of the page waits.
  useEffect(() => {
    if (!recordOpen && !askOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setRecordOpen(false);
        setAskOpen(false);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [recordOpen, askOpen]);

  // The profile window is the last 30 days: "近期" with enough depth to show a
  // pattern rather than one meeting. Handing off to the store means the request
  // is not owned by this component instance and survives navigation.
  const generateProfile = () => {
    if (!person || profileBusy) return;
    const end = todayISO();
    const start = localDate(new Date(Date.now() - 29 * 24 * 60 * 60 * 1000));
    startPortrait(person.id, { person_id: person.id, start, end });
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

      {/* Headline: who this is, plus the two actions worth reaching from here.
          One white panel — the portrait does not need a bordered box of its own. */}
      <section className="panel mb-5">
        <div className="flex flex-wrap items-start justify-between gap-4 p-5">
          <div className="flex min-w-0 gap-4">
            <Avatar name={person.name} id={person.id} size="lg" />
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-[clamp(20px,2.6vw,26px)] font-bold leading-[1.15] tracking-[-0.03em] text-foreground">
                  {person.name}
                </h1>
                {person.is_self ? (
                  <Badge variant="secondary">我 · 系统保留</Badge>
                ) : person.relation ? (
                  <Badge variant="secondary">{person.relation}</Badge>
                ) : null}
              </div>
              <p className="mt-1.5 text-[13px] leading-[1.6] text-muted-foreground">
                {[currentOrg?.name, person.position].filter(Boolean).join(' · ') || '未填写组织与职位'} · 建档于{' '}
                <span className="tabular-nums">{fullDate(person.created_at)}</span>
              </p>
            </div>
          </div>

          <div className="flex shrink-0 gap-2">
            <Button onClick={() => setRecordOpen(true)}>
              <NotebookPen className="size-4" />
              新增记录
            </Button>
            <Button variant="outline" onClick={() => setAskOpen(true)}>
              <MessageSquareText className="size-4" />
              问一下
            </Button>
          </div>
        </div>
      </section>

      <div className="grid gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        {/* —— 左 2/3：概览与时间线 —— */}
        <div className="min-w-0 space-y-9">
          <SectionCard
            title="人物概览"
            actions={
              <Button size="sm" variant="outline" onClick={() => void generateProfile()} disabled={profileBusy}>
                {profileBusy ? '生成中…' : profile ? '重新生成' : '生成画像'}
              </Button>
            }
          >
            {profileBusy ? (
              <Spinner label="AI 正在汇总近 30 天的记录…（离开此页生成也会继续）" />
            ) : portrait.status === 'error' ? (
              <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                上次生成失败：{portrait.message}，可重试。
              </p>
            ) : portrait.status === 'done' && portrait.snapshot.status === 'failed' ? (
              <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                上次生成失败：{portrait.snapshot.failure_reason || '未知原因'}，可重试。
              </p>
            ) : portrait.status === 'done' ? (
              <>
                <p className="text-[14px] leading-[1.85] text-ink-2">{portrait.snapshot.summary}</p>
                <p className="mt-3.5 text-[12px] tabular-nums text-ink-3">
                  统计窗口 {portrait.snapshot.start} ~ {portrait.snapshot.end} · 生成于{' '}
                  {fullDate(portrait.snapshot.generated_at)}
                </p>
              </>
            ) : profile && profile.status === 'succeeded' && profile.summary ? (
              <>
                <p className="text-[14px] leading-[1.85] text-ink-2">{profile.summary}</p>
                <p className="mt-3.5 text-[12px] tabular-nums text-ink-3">
                  统计窗口 {profile.start} ~ {profile.end} · 生成于 {fullDate(profile.generated_at)}
                </p>
              </>
            ) : profile && profile.status === 'failed' ? (
              <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                上次生成失败：{profile.failure_reason || '未知原因'}，可重试。
              </p>
            ) : (
              <p className="text-[13.5px] leading-[1.7] text-muted-foreground">
                还没有画像。点右上角「生成画像」，AI 会汇总近 30 天与 TA 有关的记录，写成一段画像。
              </p>
            )}

            <div className="mt-4 border-t border-hairline pt-3.5">
              {person.is_self ? (
                <p className="text-[12px] leading-[1.6] text-ink-3">
                  「我」是系统保留人物，代表使用系统的人，不能删除。
                </p>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10"
                  onClick={() => void removePerson()}
                >
                  删除这个人物（不可撤销）
                </Button>
              )}
            </div>
          </SectionCard>

          <SectionCard
            title="时间线"
            actions={
              <Link to="/events" className="text-[12.5px] text-ink-3 transition-colors hover:text-primary">
                全部记录
              </Link>
            }
            bodyClassName="p-0"
          >
            <EventTimeline events={events} />
          </SectionCard>
        </div>

        {/* —— 右 1/3：AI 画像的采纳与剔除 —— */}
        <aside className="min-w-0 space-y-9">
          <SectionCard
            title="AI 画像"
            description="✓ 采纳，✗ 剔除；新记录会自动更新，依据失效的条目会提示重算"
            actions={<span className="text-[12px] tabular-nums text-ink-3">{traits.length} 条</span>}
            bodyClassName="p-0"
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
        </aside>
      </div>

      {/* —— 新增记录：弹出式窗口 —— */}
      {recordOpen ? (
        <div
          data-state={recordState}
          className="al-scrim fixed inset-0 z-50 flex items-start justify-center p-4 pt-[8vh]"
          role="dialog"
          aria-modal="true"
          onClick={() => setRecordOpen(false)}
        >
          <div
            className="al-sheet w-full max-w-xl overflow-hidden rounded-2xl bg-card/85 shadow-overlay"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-hairline px-5 py-3.5">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">新增记录 · {person.name}</h2>
              <button
                onClick={() => setRecordOpen(false)}
                aria-label="关闭"
                className="flex size-8 items-center justify-center rounded-full text-ink-3 transition-colors hover:bg-fill hover:text-foreground"
              >
                <X className="size-4" />
              </button>
            </div>
            <div className="p-5">
              <QuickRecord
                personId={person.id}
                participantPicker
                onRecorded={() => {
                  setRecordOpen(false);
                  void load();
                }}
                bare
              />
            </div>
          </div>
        </div>
      ) : null}

      {/* —— 问一下：弹出式提问窗口 —— */}
      {askOpen ? (
        <div
          data-state={askState}
          className="al-scrim fixed inset-0 z-50 flex items-start justify-center p-4 pt-[14vh]"
          role="dialog"
          aria-modal="true"
          onClick={() => setAskOpen(false)}
        >
          <div
            className="al-sheet w-full max-w-lg overflow-hidden rounded-2xl bg-card/85 shadow-overlay"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-hairline px-5 py-3.5">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">问一下 · {person.name}</h2>
              <button
                onClick={() => setAskOpen(false)}
                aria-label="关闭"
                className="flex size-8 items-center justify-center rounded-full text-ink-3 transition-colors hover:bg-fill hover:text-foreground"
              >
                <X className="size-4" />
              </button>
            </div>
            <div className="p-5">
              <Textarea
                autoFocus
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                rows={4}
                placeholder={`关于${person.name}的问题，例如「下次找他帮忙该怎么开口」`}
                className="resize-y text-[15px] leading-[1.75]"
              />
              <p className="mt-2 text-[12px] text-ink-3">回答会引用 TA 的记录和关系。</p>
              <div className="mt-3.5 flex justify-end gap-2">
                <Button variant="ghost" size="sm" onClick={() => setAskOpen(false)}>
                  取消
                </Button>
                <Button
                  onClick={() => {
                    setAskOpen(false);
                    navigate('/advice', { state: { personId: person.id, question } });
                    setQuestion('');
                  }}
                  disabled={!question.trim()}
                >
                  <MessageSquareText className="size-4" />
                  去问 AI
                </Button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
