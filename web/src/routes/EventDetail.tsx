import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { EmptyState, ErrorNote, Spinner } from '../components/ui';
import { Button } from '../components/ui/button';
import { eventApi, personApi } from '../api/client';
import type { Event, Person } from '../api/types';
import { fullDate } from '../format';

export default function EventDetail() {
  const { id = '' } = useParams();
  const navigate = useNavigate();

  const [event, setEvent] = useState<Event | null>(null);
  const [person, setPerson] = useState<Person | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    eventApi
      .get(id)
      .then(async (data) => {
        if (cancelled) return;
        setEvent(data);
        try {
          const owner = await personApi.get(data.person_id);
          if (!cancelled) setPerson(owner);
        } catch {
          // A deleted person must not hide the event that still references them.
        }
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  const remove = async () => {
    if (!event) return;
    if (!window.confirm('删除这条记录？相关的向量索引会一起清掉。')) return;
    setError('');
    try {
      await eventApi.delete(event.id);
      navigate(`/persons/${event.person_id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) {
    return <Spinner label="载入记录…" />;
  }

  if (!event) {
    return (
      <div className="space-y-4">
        {error ? <ErrorNote>{error}</ErrorNote> : <EmptyState>找不到这条记录。</EmptyState>}
        <Link to="/" className="text-sm text-primary">
          返回首页
        </Link>
      </div>
    );
  }

  const fields: { label: string; value: string }[] = [
    { label: '摘要', value: event.summary },
    { label: '我的感受', value: event.my_feeling },
    { label: '对方反应', value: event.their_reaction },
  ];

  return (
    <div className="space-y-4">
      {error ? <ErrorNote>{error}</ErrorNote> : null}

      <div className="flex items-baseline justify-between gap-3">
        <div className="text-sm text-muted-foreground">
          <span className="font-mono">{fullDate(event.event_date)}</span>
          {person ? (
            <Link to={`/persons/${person.id}`} className="ml-2 font-medium text-primary">
              {person.name}
            </Link>
          ) : null}
        </div>
        <Button
          onClick={remove}
          variant="outline"
          className="h-8 text-muted-foreground hover:border-red-300 hover:text-red-600"
        >
          删除
        </Button>
      </div>

      <div className="space-y-4 rounded-xl border border-border bg-card p-5 shadow-sm">
        {fields.some((field) => field.value) ? (
          <dl className="space-y-2">
            {fields.map((field) =>
              field.value ? (
                <div key={field.label} className="flex gap-3 text-sm">
                  <dt className="w-20 shrink-0 text-muted-foreground">{field.label}</dt>
                  <dd className="leading-7 text-foreground">{field.value}</dd>
                </div>
              ) : null,
            )}
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">这条记录还没有 AI 提取结果。</p>
        )}

        {event.promises.length > 0 ? (
          <ul className="flex flex-wrap gap-2">
            {event.promises.map((promise, index) => (
              <li key={index} className="rounded-full bg-primary/10 px-3 py-1 text-xs text-primary">
                {promise.who || '对方'}：{promise.what}
                {promise.deadline ? `（${promise.deadline}）` : ''}
              </li>
            ))}
          </ul>
        ) : null}

        <div>
          <h2 className="mb-1 text-sm font-medium text-muted-foreground">原文</h2>
          <p className="whitespace-pre-wrap rounded-lg bg-muted px-3 py-2 text-sm leading-7 text-foreground">
            {event.raw_text}
          </p>
        </div>

        <p className="text-xs text-muted-foreground">记录于 {fullDate(event.created_at)}</p>
      </div>
    </div>
  );
}
