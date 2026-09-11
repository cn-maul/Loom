import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Building2, Pencil } from 'lucide-react';
import { EmptyState, ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { PageHeader, SectionCard } from '../components/layout';
import { organizationApi, personApi } from '../api/client';
import type { Organization, OrgPositionLink, PersonWithActivity } from '../api/types';
import { fullDate, todayISO } from '../format';

const KIND_OPTIONS = ['公司', '学校', '政府机构', '社团', '其他'];

export default function Organizations() {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [selectedId, setSelectedId] = useState('');
  const [members, setMembers] = useState<OrgPositionLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  // Create form.
  const [draft, setDraft] = useState({ name: '', kind: '公司', description: '' });

  // Inline edit of the selected organisation.
  const [editing, setEditing] = useState(false);
  const [editDraft, setEditDraft] = useState({ name: '', kind: '', description: '' });

  // Add-member form on the selected organisation.
  const [adding, setAdding] = useState(false);
  const [memberDraft, setMemberDraft] = useState({ person_id: '', role: '', start_date: '' });

  const reloadOrgs = useCallback(async (keepId?: string) => {
    try {
      const data = await organizationApi.list(true);
      setOrgs(data);
      setSelectedId((current) => {
        const wanted = keepId ?? current;
        return data.some((o) => o.id === wanted) ? wanted : data[0]?.id ?? '';
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    Promise.all([organizationApi.list(true), personApi.list()])
      .then(([all, people]) => {
        setOrgs(all);
        setPersons(people);
        setSelectedId(all[0]?.id ?? '');
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, []);

  const selected = useMemo(() => orgs.find((o) => o.id === selectedId) ?? null, [orgs, selectedId]);

  const reloadMembers = useCallback(async (orgId: string) => {
    if (!orgId) {
      setMembers([]);
      return;
    }
    try {
      setMembers(await organizationApi.members(orgId));
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void reloadMembers(selectedId);
  }, [selectedId, reloadMembers]);

  const create = async () => {
    const name = draft.name.trim();
    if (!name || busy) return;
    setBusy(true);
    setActionError('');
    try {
      const created = await organizationApi.create({ name, kind: draft.kind, description: draft.description.trim() });
      setDraft({ name: '', kind: draft.kind, description: '' });
      await reloadOrgs(created.id);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = async () => {
    if (!selected || busy) return;
    const name = editDraft.name.trim();
    if (!name) return;
    setBusy(true);
    setActionError('');
    try {
      const updated = await organizationApi.update(selected.id, {
        name,
        kind: editDraft.kind.trim(),
        description: editDraft.description.trim(),
      });
      setOrgs((current) => current.map((o) => (o.id === updated.id ? updated : o)));
      setEditing(false);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const toggleArchive = async (org: Organization) => {
    if (busy) return;
    setBusy(true);
    setActionError('');
    try {
      const updated = org.archived_at ? await organizationApi.restore(org.id) : await organizationApi.archive(org.id);
      await reloadOrgs(updated.id);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const removeOrg = async (org: Organization) => {
    if (!window.confirm(`删除组织「${org.name}」？成员不会删除，只会变为「未归属」。历史任职记录会一并删除。`)) return;
    setBusy(true);
    setActionError('');
    try {
      await organizationApi.delete(org.id);
      await reloadOrgs();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const addMember = async () => {
    if (!selected || !memberDraft.person_id || busy) return;
    setBusy(true);
    setActionError('');
    try {
      await organizationApi.addMember(memberDraft.person_id, {
        org_id: selected.id,
        role: memberDraft.role.trim(),
        start_date: memberDraft.start_date,
      });
      setMemberDraft({ person_id: '', role: '', start_date: '' });
      setAdding(false);
      await reloadMembers(selected.id);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  // Ending a stint keeps it in the history; the PUT replaces the whole posting,
  // so the link's own fields travel along with the new end_date.
  const endMembership = async (pos: OrgPositionLink) => {
    if (busy) return;
    setBusy(true);
    setActionError('');
    try {
      await organizationApi.endMembership(pos, todayISO());
      await reloadMembers(selectedId);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const removeMembership = async (pos: OrgPositionLink) => {
    if (!window.confirm(`删除 ${pos.person_name} 的这段任职记录？此操作不可撤销。`)) return;
    setBusy(true);
    setActionError('');
    try {
      await organizationApi.removeMembership(pos.id);
      await reloadMembers(selectedId);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const currentMembers = members.filter((m) => !m.end_date);
  const pastMembers = members.filter((m) => m.end_date);
  const activeOrgs = orgs.filter((o) => !o.archived_at);
  const archivedOrgs = orgs.filter((o) => o.archived_at);

  if (loading) return <Spinner label="载入组织…" />;

  return (
    <div className="space-y-4">
      {error ? <ErrorNote>{error}</ErrorNote> : null}
      {actionError ? <ErrorNote>{actionError}</ErrorNote> : null}

      <PageHeader
        title="组织"
        description="这些人的组织背景：现任成员、历史任职；归档的组织不再出现在人物表单中"
        actions={
          <span className="text-xs text-muted-foreground">
            {activeOrgs.length} 个在用
            {archivedOrgs.length > 0 ? ` · ${archivedOrgs.length} 个已归档` : ''}
          </span>
        }
      />

      <div className="flex flex-col gap-5 lg:flex-row">
        <aside className="w-full shrink-0 space-y-4 lg:w-80">
          <SectionCard title="新建组织">
            <div className="space-y-2">
              <input
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                onKeyDown={(e) => e.key === 'Enter' && create()}
                placeholder="名称，如：某某科技"
                className={controlClass}
              />
              <div className="flex gap-2">
                <select
                  value={draft.kind}
                  onChange={(e) => setDraft({ ...draft, kind: e.target.value })}
                  className={`${controlClass} min-w-0 flex-1 text-muted-foreground`}
                >
                  {KIND_OPTIONS.map((k) => (
                    <option key={k} value={k}>
                      {k}
                    </option>
                  ))}
                </select>
                <Button onClick={create} disabled={busy || !draft.name.trim()} variant="outline" className="shrink-0">
                  新建
                </Button>
              </div>
              <input
                value={draft.description}
                onChange={(e) => setDraft({ ...draft, description: e.target.value })}
                placeholder="说明（可选）"
                className={controlClass}
              />
            </div>
          </SectionCard>

          <SectionCard title="全部组织" bodyClassName="p-3">
            {orgs.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">还没有组织。人物可以不归属任何组织。</p>
            ) : (
              <>
                <ul className="space-y-1">
                  {activeOrgs.map((org) => (
                    <li key={org.id}>
                      <button
                        onClick={() => setSelectedId(org.id)}
                        className={`w-full rounded-lg px-3 py-2 text-left transition-colors ${
                          selectedId === org.id ? 'bg-primary/10 ring-1 ring-primary' : 'hover:bg-muted'
                        }`}
                      >
                        <div className="flex items-baseline justify-between gap-2">
                          <span className="truncate text-sm font-medium text-foreground">{org.name}</span>
                          {org.kind ? <span className="shrink-0 text-xs text-muted-foreground">{org.kind}</span> : null}
                        </div>
                      </button>
                    </li>
                  ))}
                </ul>
                {archivedOrgs.length > 0 ? (
                  <>
                    <p className="mb-1 mt-3 border-t border-border pt-3 text-xs font-medium text-muted-foreground">
                      已归档（不在人物表单中出现）
                    </p>
                    <ul className="space-y-1">
                      {archivedOrgs.map((org) => (
                        <li key={org.id}>
                          <button
                            onClick={() => setSelectedId(org.id)}
                            className={`w-full rounded-lg px-3 py-2 text-left opacity-70 transition-colors ${
                              selectedId === org.id ? 'bg-primary/10 ring-1 ring-primary' : 'hover:bg-muted'
                            }`}
                          >
                            <span className="truncate text-sm text-foreground">{org.name}</span>
                          </button>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : null}
              </>
            )}
          </SectionCard>
        </aside>

        <section className="min-w-0 flex-1">
          {!selected ? (
            <EmptyState>选择或新建一个组织。</EmptyState>
          ) : (
            <div className="space-y-4">
              <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
                {selected.archived_at ? (
                  <div className="mb-3">
                    <Notice>已归档：不出现在人物表单和组织筛选中，历史记录保留。归档于 {fullDate(selected.archived_at)}。</Notice>
                  </div>
                ) : null}
                {editing ? (
                  <div className="space-y-3">
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className="text-sm text-muted-foreground">
                        名称
                        <input
                          value={editDraft.name}
                          onChange={(e) => setEditDraft({ ...editDraft, name: e.target.value })}
                          className={`${controlClass} mt-1`}
                        />
                      </label>
                      <label className="text-sm text-muted-foreground">
                        类型
                        <input
                          value={editDraft.kind}
                          onChange={(e) => setEditDraft({ ...editDraft, kind: e.target.value })}
                          placeholder="公司 / 学校 / …"
                          className={`${controlClass} mt-1`}
                        />
                      </label>
                    </div>
                    <label className="block text-sm text-muted-foreground">
                      说明
                      <textarea
                        value={editDraft.description}
                        onChange={(e) => setEditDraft({ ...editDraft, description: e.target.value })}
                        rows={2}
                        className={`${controlClass} mt-1 h-auto resize-y`}
                      />
                    </label>
                    <div className="flex justify-end gap-2">
                      <Button variant="ghost" onClick={() => setEditing(false)}>
                        取消
                      </Button>
                      <Button onClick={saveEdit} disabled={busy || !editDraft.name.trim()}>
                        {busy ? '保存中…' : '保存'}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground">
                        <Building2 className="size-4 shrink-0 text-muted-foreground" />
                        {selected.name}
                      </h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {[selected.kind, selected.description].filter(Boolean).join(' · ') || '暂无类型与说明'}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button
                        variant="outline"
                        onClick={() => {
                          setEditDraft({ name: selected.name, kind: selected.kind, description: selected.description });
                          setEditing(true);
                        }}
                      >
                        <Pencil className="mr-1.5 size-3.5" />
                        编辑
                      </Button>
                      <Button variant="outline" onClick={() => void toggleArchive(selected)} disabled={busy}>
                        {selected.archived_at ? '恢复' : '归档'}
                      </Button>
                      <Button
                        variant="ghost"
                        className="text-muted-foreground hover:text-red-600"
                        onClick={() => void removeOrg(selected)}
                        disabled={busy}
                      >
                        删除
                      </Button>
                    </div>
                  </div>
                )}
              </div>

              <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
                <div className="mb-3 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-foreground">现任成员（{currentMembers.length}）</h3>
                  <Button size="sm" variant="outline" onClick={() => setAdding((v) => !v)}>
                    {adding ? '收起' : '添加成员'}
                  </Button>
                </div>

                {adding ? (
                  <div className="mb-4 rounded-lg border border-border bg-muted/40 p-3">
                    <div className="grid gap-2 sm:grid-cols-3">
                      <select
                        value={memberDraft.person_id}
                        onChange={(e) => setMemberDraft({ ...memberDraft, person_id: e.target.value })}
                        className={`${controlClass} text-muted-foreground`}
                      >
                        <option value="">选择人物…</option>
                        {persons.map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <input
                        value={memberDraft.role}
                        onChange={(e) => setMemberDraft({ ...memberDraft, role: e.target.value })}
                        placeholder="职位（可选）"
                        className={controlClass}
                      />
                      <input
                        type="date"
                        value={memberDraft.start_date}
                        onChange={(e) => setMemberDraft({ ...memberDraft, start_date: e.target.value })}
                        title="入职/加入日期（可选）"
                        className={controlClass}
                      />
                    </div>
                    <div className="mt-2 flex justify-end">
                      <Button size="sm" onClick={addMember} disabled={busy || !memberDraft.person_id}>
                        {busy ? '保存中…' : '添加'}
                      </Button>
                    </div>
                  </div>
                ) : null}

                {currentMembers.length === 0 ? (
                  <p className="py-3 text-center text-sm text-muted-foreground">暂无现任成员。</p>
                ) : (
                  <ul className="space-y-2">
                    {currentMembers.map((m) => (
                      <li key={m.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-3 py-2">
                        <div className="min-w-0">
                          <Link to={`/persons/${m.person_id}`} className="text-sm font-medium text-foreground hover:text-primary">
                            {m.person_name}
                          </Link>
                          <span className="ml-2 text-xs text-muted-foreground">
                            {[m.role, m.start_date ? `自 ${fullDate(m.start_date)}` : ''].filter(Boolean).join(' · ')}
                          </span>
                        </div>
                        <div className="flex shrink-0 gap-1">
                          <Button size="sm" variant="ghost" disabled={busy} onClick={() => void endMembership(m)}>
                            结束任职
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-muted-foreground hover:text-red-600"
                            disabled={busy}
                            onClick={() => void removeMembership(m)}
                          >
                            删除
                          </Button>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}

                {pastMembers.length > 0 ? (
                  <>
                    <h3 className="mb-2 mt-4 border-t border-border pt-3 text-sm font-semibold text-muted-foreground">
                      历史成员（{pastMembers.length}）
                    </h3>
                    <ul className="space-y-2">
                      {pastMembers.map((m) => (
                        <li key={m.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted/40 px-3 py-2">
                          <div className="min-w-0">
                            <Link to={`/persons/${m.person_id}`} className="text-sm text-foreground hover:text-primary">
                              {m.person_name}
                            </Link>
                            <span className="ml-2 text-xs text-muted-foreground">
                              {[m.role, m.start_date && m.end_date ? `${fullDate(m.start_date)} ~ ${fullDate(m.end_date)}` : m.end_date ? `至 ${fullDate(m.end_date)}` : ''].filter(Boolean).join(' · ')}
                            </span>
                          </div>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-muted-foreground hover:text-red-600"
                            disabled={busy}
                            onClick={() => void removeMembership(m)}
                          >
                            删除
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </>
                ) : null}
              </div>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

// personApiList removed — personApi is imported at the top now.
