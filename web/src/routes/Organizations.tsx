import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Archive, ArchiveRestore, Building2, Pencil, Plus, Trash2, Users, X } from 'lucide-react';
import { Badge } from '../components/ui/badge';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { Avatar, EmptyState, Field, PageHeader, SectionCard } from '../components/layout';
import { organizationApi, personApi } from '../api/client';
import type { Organization, Person, PersonWithActivity } from '../api/types';
import { RELATION_OPTIONS } from '../relationOptions';
import { useEnterState } from '../lib/overlay';
import { cn } from '../lib/utils';

/** 组织分类是封闭的三个：公司 / 政府 / 其他。 */
const ORG_KINDS = ['公司', '政府', '其他'] as const;

/** 性别是用户显式选择的二值事实。 */
const GENDERS = ['男', '女'] as const;

const EMPTY_ORG_DRAFT = { name: '', kind: '公司' as string, description: '' };
const EMPTY_PERSON_DRAFT = {
  name: '',
  gender: '男' as string,
  relation: '朋友' as string,
  org_id: '',
  position: '',
  importance: 3,
  notes: '',
};

/** Row-level action: invisible until the row is hovered, so the list stays quiet. */
const ROW_ACTION =
  'flex size-7 items-center justify-center rounded-full text-ink-4 transition-colors hover:bg-fill hover:text-foreground';

export default function Organizations() {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [persons, setPersons] = useState<PersonWithActivity[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  // Organisation state: the popup form. `editingOrg === null` means create mode.
  const [orgFormOpen, setOrgFormOpen] = useState(false);
  const [editingOrg, setEditingOrg] = useState<Organization | null>(null);
  const [orgDraft, setOrgDraft] = useState(EMPTY_ORG_DRAFT);
  const [savingOrg, setSavingOrg] = useState(false);
  // The layer mounts already-open, so it needs the frame to transition from.
  const orgFormState = useEnterState(orgFormOpen);

  // Person state: filter by organisation, then CRUD.
  const [orgFilter, setOrgFilter] = useState('');
  const [personFormOpen, setPersonFormOpen] = useState(false);
  const [personDraft, setPersonDraft] = useState(EMPTY_PERSON_DRAFT);
  const [editingPerson, setEditingPerson] = useState<Person | null>(null);
  const [savingPerson, setSavingPerson] = useState(false);

  const reload = useCallback(async () => {
    try {
      const [orgList, personList] = await Promise.all([organizationApi.list(true), personApi.list()]);
      setOrgs(orgList);
      setPersons(personList);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const activeOrgs = orgs.filter((org) => !org.archived_at);
  const visiblePersons =
    orgFilter === 'none'
      ? persons.filter((p) => !p.org_id)
      : orgFilter
        ? persons.filter((p) => p.org_id === orgFilter)
        : persons;

  const flashNotice = (message: string) => {
    setNotice(message);
    window.setTimeout(() => setNotice(''), 3000);
  };

  // —— Organisation actions ——
  const openCreateOrg = () => {
    setOrgDraft(EMPTY_ORG_DRAFT);
    setEditingOrg(null);
    setOrgFormOpen(true);
  };

  const openEditOrg = (org: Organization) => {
    setOrgDraft({ name: org.name, kind: org.kind || '公司', description: org.description || '' });
    setEditingOrg(org);
    setOrgFormOpen(true);
  };

  const closeOrgForm = () => {
    setOrgFormOpen(false);
    setEditingOrg(null);
  };

  // Escape closes the popup; the rest of the page waits.
  useEffect(() => {
    if (!orgFormOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeOrgForm();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [orgFormOpen]);

  const saveOrg = async () => {
    const name = orgDraft.name.trim();
    if (!name || savingOrg) return;
    setSavingOrg(true);
    setError('');
    try {
      const payload = { name, kind: orgDraft.kind, description: orgDraft.description.trim() };
      if (editingOrg) {
        await organizationApi.update(editingOrg.id, payload);
        flashNotice('组织信息已保存');
      } else {
        const created = await organizationApi.create(payload);
        flashNotice(`已创建组织「${created.name}」`);
      }
      closeOrgForm();
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSavingOrg(false);
    }
  };

  // Archiving is the reversible alternative to deleting: nobody loses their
  // employer and the organisation stops being offered in every picker.
  const toggleArchiveOrg = async (org: Organization) => {
    const archived = Boolean(org.archived_at);
    if (!archived && !window.confirm(`归档「${org.name}」？它不会再出现在组织选择框里，成员关系保留，随时可以恢复。`)) return;
    setError('');
    try {
      if (archived) await organizationApi.restore(org.id);
      else await organizationApi.archive(org.id);
      await reload();
      flashNotice(archived ? `已恢复「${org.name}」` : `已归档「${org.name}」`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const deleteOrg = async (org: Organization) => {
    if (!window.confirm(`删除「${org.name}」？该组织下的人物会被解除归属，人物本身保留。`)) return;
    setError('');
    try {
      await organizationApi.delete(org.id);
      if (editingOrg?.id === org.id) closeOrgForm();
      await reload();
      flashNotice(`已删除「${org.name}」`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  // —— Person actions ——
  const savePerson = async () => {
    const name = personDraft.name.trim();
    if (!name || savingPerson) return;
    setSavingPerson(true);
    setError('');
    try {
      const payload = {
        name,
        gender: personDraft.gender,
        relation: personDraft.relation,
        org_id: personDraft.org_id,
        position: personDraft.position.trim(),
        importance: personDraft.importance,
        notes: personDraft.notes.trim(),
      };
      if (editingPerson) {
        await personApi.update(editingPerson.id, { ...editingPerson, ...payload });
        flashNotice(`已保存「${name}」`);
      } else {
        await personApi.create(payload);
        flashNotice(`已创建人物「${name}」`);
      }
      setPersonDraft(EMPTY_PERSON_DRAFT);
      setEditingPerson(null);
      setPersonFormOpen(false);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSavingPerson(false);
    }
  };

  const startEditPerson = (person: PersonWithActivity) => {
    setEditingPerson(person);
    setPersonDraft({
      name: person.name,
      gender: person.gender || '男',
      relation: person.relation || '朋友',
      org_id: person.org_id,
      position: person.position || '',
      importance: person.importance,
      notes: person.notes || '',
    });
    setPersonFormOpen(true);
  };

  const deletePerson = async (person: PersonWithActivity) => {
    if (!window.confirm(`删除「${person.name}」及其全部记录和画像？此操作不可撤销。`)) return;
    setError('');
    try {
      await personApi.delete(person.id);
      if (editingPerson?.id === person.id) {
        setEditingPerson(null);
        setPersonFormOpen(false);
        setPersonDraft(EMPTY_PERSON_DRAFT);
      }
      await reload();
      flashNotice(`已删除「${person.name}」`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (loading) {
    return <Spinner label="载入人物与组织…" />;
  }

  const field = `${controlClass} h-9 text-[13px]`;

  return (
    <div>
      <PageHeader title="人物与组织" />

      {error ? (
        <div className="mb-5">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {notice ? (
        <div className="mb-5">
          <Notice>{notice}</Notice>
        </div>
      ) : null}

      <div className="grid gap-5 lg:grid-cols-[22rem_minmax(0,1fr)]">
        {/* —— 左侧：组织 —— */}
        <aside>
          <section className="panel">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline px-5 py-3.5">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">组织</h2>
              <button
                onClick={openCreateOrg}
                title="新建组织"
                aria-label="新建组织"
                className="flex size-7 items-center justify-center rounded-full text-ink-3 transition-colors hover:bg-fill hover:text-foreground"
              >
                <Plus className="size-4" />
              </button>
            </div>

            {orgs.length === 0 ? (
              <EmptyState icon={<Building2 className="size-6" />} title="还没有组织" description="点右上角 + 新建一个组织，再往里放人。" />
            ) : (
              <ul className="panel-rows">
                {orgs.map((org) => {
                  const active = orgFilter === org.id;
                  const archived = Boolean(org.archived_at);
                  return (
                    <li key={org.id} className="group relative">
                      <button
                        onClick={() => setOrgFilter(active ? '' : org.id)}
                        className={cn(
                          'flex w-full items-center gap-2.5 py-2.5 pl-5 pr-16 text-left transition-colors',
                          active ? 'bg-fill' : 'hover:bg-hover',
                        )}
                      >
                        <Building2 className={cn('size-4 shrink-0', archived ? 'text-faint' : 'text-ink-3')} />
                        <span className="min-w-0 flex-1">
                          <span
                            className={cn(
                              'block truncate text-[13.5px]',
                              archived
                                ? 'text-ink-4 line-through'
                                : active
                                  ? 'font-semibold text-foreground'
                                  : 'font-medium text-foreground',
                            )}
                          >
                            {org.name}
                          </span>
                          <span className="block truncate text-[12px] text-ink-3">
                            {org.kind || '其他'}
                            {archived ? ' · 已归档' : ''}
                          </span>
                        </span>
                      </button>
                      {/* Row-level edit / archive / delete; hover-revealed so the list stays quiet. */}
                      <div className="absolute right-2 top-1/2 flex -translate-y-1/2 gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                        <button onClick={() => openEditOrg(org)} title={`编辑「${org.name}」`} className={ROW_ACTION}>
                          <Pencil className="size-3.5" />
                        </button>
                        <button
                          onClick={() => void toggleArchiveOrg(org)}
                          title={archived ? `恢复「${org.name}」` : `归档「${org.name}」`}
                          className={ROW_ACTION}
                        >
                          {archived ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}
                        </button>
                        <button
                          onClick={() => void deleteOrg(org)}
                          title={`删除「${org.name}」`}
                          className={`${ROW_ACTION} hover:text-destructive`}
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        </aside>

        {/* —— 右侧：人物 —— */}
        <section className="min-w-0">
          {personFormOpen ? (
            <SectionCard title={editingPerson ? `编辑人物：${editingPerson.name}` : '新建人物'} className="mb-5">
              <div className="grid gap-3.5 md:grid-cols-2">
                <Field label="姓名">
                  <input
                    autoFocus
                    value={personDraft.name}
                    onChange={(e) => setPersonDraft({ ...personDraft, name: e.target.value })}
                    placeholder="必填"
                    className={controlClass}
                  />
                </Field>
                <Field label="性别">
                  <select
                    value={personDraft.gender}
                    onChange={(e) => setPersonDraft({ ...personDraft, gender: e.target.value })}
                    className={controlClass}
                  >
                    {GENDERS.map((g) => (
                      <option key={g} value={g}>
                        {g}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="分类">
                  <select
                    value={personDraft.relation}
                    onChange={(e) => setPersonDraft({ ...personDraft, relation: e.target.value })}
                    className={controlClass}
                  >
                    {RELATION_OPTIONS.map((r) => (
                      <option key={r} value={r}>
                        {r}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="组织">
                  <select
                    value={personDraft.org_id}
                    onChange={(e) => setPersonDraft({ ...personDraft, org_id: e.target.value })}
                    className={controlClass}
                  >
                    <option value="">无组织</option>
                    {activeOrgs.map((org) => (
                      <option key={org.id} value={org.id}>
                        {org.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="职位">
                  <input
                    value={personDraft.position}
                    onChange={(e) => setPersonDraft({ ...personDraft, position: e.target.value })}
                    placeholder="可选"
                    className={controlClass}
                  />
                </Field>
                <div className="md:col-span-2">
                  <Field label="备注">
                    <textarea
                      value={personDraft.notes}
                      onChange={(e) => setPersonDraft({ ...personDraft, notes: e.target.value })}
                      rows={2}
                      className={`${controlClass} h-auto resize-y py-3 leading-[1.85]`}
                    />
                  </Field>
                </div>
              </div>
              <div className="mt-4 flex justify-end gap-2">
                <Button
                  variant="ghost"
                  onClick={() => {
                    setPersonFormOpen(false);
                    setEditingPerson(null);
                    setPersonDraft(EMPTY_PERSON_DRAFT);
                  }}
                >
                  取消
                </Button>
                <Button onClick={() => void savePerson()} disabled={savingPerson || !personDraft.name.trim()}>
                  {savingPerson ? '保存中…' : '保存'}
                </Button>
              </div>
            </SectionCard>
          ) : null}

          <section className="panel">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline px-5 py-3.5">
              <div className="flex min-w-0 items-center gap-2.5">
                <select
                  value={orgFilter}
                  onChange={(e) => setOrgFilter(e.target.value)}
                  className={`${field} w-44`}
                  aria-label="按组织筛选人物"
                >
                  <option value="">全部人物</option>
                  <option value="none">未归属人物</option>
                  {orgs.map((org) => (
                    <option key={org.id} value={org.id}>
                      {org.name}
                    </option>
                  ))}
                </select>
                <span className="shrink-0 whitespace-nowrap text-[12px] tabular-nums text-ink-3">
                  共 {visiblePersons.length} 位
                </span>
              </div>
              <Button
                size="sm"
                onClick={() => {
                  setPersonDraft(EMPTY_PERSON_DRAFT);
                  setEditingPerson(null);
                  setPersonFormOpen(true);
                }}
              >
                <Plus className="size-3.5" />
                新建人物
              </Button>
            </div>

            {visiblePersons.length === 0 ? (
              <EmptyState
                icon={<Users className="size-6" />}
                title={orgFilter ? '这个筛选下还没有人物' : '还没有人物'}
                description={orgFilter ? '换一个筛选，或点右上角「新建人物」。' : '点右上角「新建人物」开始。'}
              />
            ) : (
              <ul className="panel-rows">
                {visiblePersons.map((person) => (
                  <li
                    key={person.id}
                    className="group grid grid-cols-[minmax(0,1fr)_3rem_minmax(0,9rem)_minmax(0,9rem)_auto] items-center gap-3 px-5 py-3 transition-colors hover:bg-hover"
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <Avatar name={person.name} id={person.id} size="md" />
                      <Link
                        to={`/persons/${person.id}`}
                        className="truncate text-[13.5px] font-medium text-foreground hover:text-primary"
                      >
                        {person.name}
                      </Link>
                      {person.is_self ? (
                        <Badge variant="secondary" className="shrink-0">
                          我
                        </Badge>
                      ) : null}
                    </div>
                    <span className="truncate text-[12px] text-ink-3">{person.gender || '—'}</span>
                    <span className="hidden truncate text-[12px] text-ink-3 sm:block">{person.org_name || '—'}</span>
                    <span className="hidden truncate text-[12px] text-ink-3 md:block">{person.position || '—'}</span>
                    <div className="flex gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100">
                      <button onClick={() => startEditPerson(person)} title={`编辑「${person.name}」`} className={ROW_ACTION}>
                        <Pencil className="size-3.5" />
                      </button>
                      {person.is_self ? null : (
                        <button
                          onClick={() => void deletePerson(person)}
                          title={`删除「${person.name}」`}
                          className={`${ROW_ACTION} hover:text-destructive`}
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </section>
      </div>

      {/* —— 新建/编辑组织：弹出式窗口 —— */}
      {orgFormOpen ? (
        <div
          data-state={orgFormState}
          className="al-scrim fixed inset-0 z-50 flex items-start justify-center p-4 pt-[12vh]"
          role="dialog"
          aria-modal="true"
          onClick={closeOrgForm}
        >
          <div
            className="al-sheet w-full max-w-sm overflow-hidden rounded-2xl bg-card/85 shadow-overlay"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-hairline px-5 py-3.5">
              <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-foreground">
                {editingOrg ? `编辑组织：${editingOrg.name}` : '新建组织'}
              </h2>
              <button
                onClick={closeOrgForm}
                aria-label="关闭"
                className="flex size-8 items-center justify-center rounded-full text-ink-3 transition-colors hover:bg-fill hover:text-foreground"
              >
                <X className="size-4" />
              </button>
            </div>
            <div className="space-y-3.5 p-5">
              <Field label="名称">
                <input
                  autoFocus
                  value={orgDraft.name}
                  onChange={(e) => setOrgDraft({ ...orgDraft, name: e.target.value })}
                  placeholder="必填"
                  className={controlClass}
                />
              </Field>
              <Field label="类型">
                <select
                  value={orgDraft.kind}
                  onChange={(e) => setOrgDraft({ ...orgDraft, kind: e.target.value })}
                  className={controlClass}
                >
                  {ORG_KINDS.map((kind) => (
                    <option key={kind} value={kind}>
                      {kind}
                    </option>
                  ))}
                </select>
              </Field>
              <div className="flex justify-end gap-2 pt-1">
                <Button variant="ghost" onClick={closeOrgForm}>
                  取消
                </Button>
                <Button onClick={() => void saveOrg()} disabled={savingOrg || !orgDraft.name.trim()}>
                  {savingOrg ? '保存中…' : editingOrg ? '保存' : '创建'}
                </Button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
