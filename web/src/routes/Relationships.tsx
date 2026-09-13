import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { graphApi, aiApi, relationshipApi } from '../api/client';
import type { GraphCoLink, GraphData, GraphEdge, GraphNode, GraphOrg, Relationship } from '../api/types';
import { ErrorNote, Notice, Spinner, controlClass } from '../components/ui';
import { Button } from '../components/ui/button';
import { PageHeader, SectionCard } from '../components/layout';
import { fullDate } from '../format';

// ── 画布尺寸与布局 ────────────────────────────────────────────
const W = 1000;
const H = 680;

type Pt = { x: number; y: number };

/**
 * 轻量力导向布局（Fruchterman-Reingold 简化版）：不用任何依赖，
 * 数据量是个人 CRM 量级，O(n²) 迭代几百次毫秒级完成。
 * links 的 strength：任职边拉得最紧，关系边其次，共同经历几乎只是松耦合。
 */
function computeLayout(ids: string[], links: { a: string; b: string; strength: number }[]): Map<string, Pt> {
  const pos = new Map<string, Pt>();
  const n = ids.length;
  ids.forEach((id, i) => {
    const angle = (2 * Math.PI * i) / Math.max(n, 1);
    const ring = n <= 1 ? 0 : Math.min(W, H) * 0.34;
    pos.set(id, { x: W / 2 + ring * Math.cos(angle), y: H / 2 + ring * Math.sin(angle) });
  });
  if (n <= 1) return pos;

  const k = Math.sqrt((W * H) / n) * 0.85;
  for (let iter = 0; iter < 160; iter++) {
    const cool = 1 - iter / 160;
    const disp = new Map<string, Pt>(ids.map((id) => [id, { x: 0, y: 0 }]));
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const a = ids[i];
        const b = ids[j];
        const pa = pos.get(a)!;
        const pb = pos.get(b)!;
        let dx = pa.x - pb.x;
        let dy = pa.y - pb.y;
        let d = Math.hypot(dx, dy);
        if (d < 1) {
          dx = ((i * 7 + j * 13) % 10) - 5 || 1;
          dy = ((i * 11 + j * 3) % 10) - 5 || 1;
          d = Math.hypot(dx, dy);
        }
        const f = (k * k) / d;
        const ux = dx / d;
        const uy = dy / d;
        disp.get(a)!.x += ux * f;
        disp.get(a)!.y += uy * f;
        disp.get(b)!.x -= ux * f;
        disp.get(b)!.y -= uy * f;
      }
    }
    for (const link of links) {
      const pa = pos.get(link.a);
      const pb = pos.get(link.b);
      if (!pa || !pb) continue;
      const dx = pb.x - pa.x;
      const dy = pb.y - pa.y;
      const d = Math.hypot(dx, dy) || 1;
      const f = ((d * d) / k) * link.strength;
      const ux = dx / d;
      const uy = dy / d;
      disp.get(link.a)!.x += ux * f;
      disp.get(link.a)!.y += uy * f;
      disp.get(link.b)!.x -= ux * f;
      disp.get(link.b)!.y -= uy * f;
    }
    for (const id of ids) {
      const p = pos.get(id)!;
      const d = disp.get(id)!;
      d.x += (W / 2 - p.x) * 0.03;
      d.y += (H / 2 - p.y) * 0.03;
      const len = Math.hypot(d.x, d.y) || 1;
      const step = Math.min(len, 14) * cool;
      p.x = Math.max(50, Math.min(W - 50, p.x + (d.x / len) * step));
      p.y = Math.max(50, Math.min(H - 50, p.y + (d.y / len) * step));
    }
  }
  return pos;
}

const today = () => new Date().toISOString().slice(0, 10);

/** 边的时间窗 [start, end]（空 = 无穷）与筛选窗口是否相交。 */
function edgeInWindow(edge: { start_date: string; end_date: string }, from: string, to: string): boolean {
  const start = edge.start_date || '0000-01-01';
  const end = edge.end_date || '9999-12-31';
  if (from && end < from) return false;
  if (to && start > to) return false;
  return true;
}

interface RelationshipForm {
  id?: string;
  from_person_id: string;
  to_person_id: string;
  relation_type: string;
  direction: 'directed' | 'undirected';
  start_date: string;
  end_date: string;
  confirmed: number;
  notes: string;
}

const EMPTY_FORM: RelationshipForm = {
  from_person_id: '',
  to_person_id: '',
  relation_type: '',
  direction: 'directed',
  start_date: '',
  end_date: '',
  confirmed: 1,
  notes: '',
};

export default function Relationships() {
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [types, setTypes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [inferring, setInferring] = useState(false);
  const [inferNotice, setInferNotice] = useState('');

  // 筛选
  const [typeFilter, setTypeFilter] = useState('');
  const [orgFilter, setOrgFilter] = useState('');
  const [fromFilter, setFromFilter] = useState('');
  const [toFilter, setToFilter] = useState('');
  const [showEnded, setShowEnded] = useState(true);
  const [showCo, setShowCo] = useState(false);

  // 选中与表单
  const [hoverId, setHoverId] = useState('');
  const [selEdgeId, setSelEdgeId] = useState('');
  const [selNodeId, setSelNodeId] = useState('');
  const [form, setForm] = useState<RelationshipForm | null>(null);

  const reload = useCallback(async () => {
    try {
      const [data, typeList] = await Promise.all([graphApi.get(), relationshipApi.types()]);
      setGraph(data);
      setTypes(typeList);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  // AI 按职位推断上下级：筛了组织就只推该组织，否则扫描全部。
  // 新边一律未确认，模型只补充、不覆盖已有关系。
  const inferHierarchy = async () => {
    if (!orgFilter && graph && graph.orgs.length > 1) {
      if (!window.confirm(`将对 ${graph.orgs.length} 个组织的在职成员各跑一次 AI 推断（每个组织一次模型调用）。继续？`)) return;
    }
    setInferring(true);
    setError('');
    setInferNotice('');
    try {
      const result = await aiApi.inferHierarchy(orgFilter || undefined);
      setInferNotice(
        result.created > 0
          ? `AI 新推断出 ${result.created} 条上下级关系，均为「待确认」状态，请在图谱中核对后确认或删除。`
          : '没有发现可新增的上下级关系（已存在的关系不会重复创建，拿不准的 AI 会跳过）。',
      );
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setInferring(false);
    }
  };

  const nodes = graph?.nodes ?? [];
  const orgs = graph?.orgs ?? [];

  // ── 过滤：组织决定可见节点，其余条件决定可见边 ────────────
  const visible = useMemo(() => {
    const personIds = new Set<string>();
    const orgIds = new Set<string>();
    for (const node of nodes) {
      if (!orgFilter || node.org_id === orgFilter) personIds.add(node.id);
    }
    for (const org of orgs) {
      if (!orgFilter || org.id === orgFilter) orgIds.add(org.id);
    }

    const inWindow = (e: GraphEdge) => edgeInWindow(e, fromFilter, toFilter);
    const relEdges = (graph?.edges ?? []).filter(
      (e) =>
        e.kind === 'relationship' &&
        personIds.has(e.from) &&
        personIds.has(e.to) &&
        (!typeFilter || e.type === typeFilter) &&
        inWindow(e) &&
        (showEnded || !e.end_date),
    );
    const posEdges = (graph?.edges ?? []).filter(
      (e) => e.kind === 'position' && personIds.has(e.from) && orgIds.has(e.to) && inWindow(e),
    );
    const coLinks: GraphCoLink[] = showCo
      ? (graph?.co_attendance ?? []).filter((c) => personIds.has(c.a) && personIds.has(c.b))
      : [];

    return { personIds, orgIds, relEdges, posEdges, coLinks };
  }, [graph, nodes, orgs, orgFilter, typeFilter, fromFilter, toFilter, showEnded, showCo]);

  // ── 布局：数据或过滤变化时重算 ────────────────────────────
  const layout = useMemo(() => {
    const ids = [...visible.personIds, ...visible.orgIds];
    const links = [
      ...visible.relEdges.map((e) => ({ a: e.from, b: e.to, strength: 0.6 })),
      ...visible.posEdges.map((e) => ({ a: e.from, b: e.to, strength: 1.2 })),
      ...visible.coLinks.map((c) => ({ a: c.a, b: c.b, strength: 0.08 })),
    ];
    return computeLayout(ids, links);
  }, [visible]);

  const nodeById = useMemo(() => {
    const map = new Map<string, GraphNode | GraphOrg>();
    for (const node of nodes) map.set(node.id, node);
    for (const org of orgs) map.set(org.id, org);
    return map;
  }, [nodes, orgs]);

  const nodeName = useCallback((id: string) => nodeById.get(id)?.name ?? '（已删除）', [nodeById]);

  const isPerson = (id: string) => {
    const node = nodeById.get(id);
    return !!node && !('member_count' in node);
  };

  const selEdge = visible.relEdges.find((e) => e.id === selEdgeId) ?? visible.posEdges.find((e) => e.id === selEdgeId) ?? null;
  const selNode = selNodeId ? nodeById.get(selNodeId) : null;

  const connected = useMemo(() => {
    // 悬停/选中某节点时只亮与它相连的边和节点。
    const focus = hoverId || selNodeId;
    if (!focus) return null;
    const ids = new Set<string>([focus]);
    const edgeIds = new Set<string>();
    for (const e of [...visible.relEdges, ...visible.posEdges]) {
      if (e.from === focus || e.to === focus) {
        ids.add(e.from);
        ids.add(e.to);
        edgeIds.add(e.id);
      }
    }
    for (const c of visible.coLinks) {
      if (c.a === focus || c.b === focus) {
        ids.add(c.a);
        ids.add(c.b);
        edgeIds.add(`co:${c.a}:${c.b}`);
      }
    }
    return { ids, edgeIds };
  }, [hoverId, selNodeId, visible]);

  const openEdit = (edge: GraphEdge) => {
    if (edge.kind !== 'relationship') return;
    setForm({
      id: edge.id,
      from_person_id: edge.from,
      to_person_id: edge.to,
      relation_type: edge.type,
      direction: (edge.direction as RelationshipForm['direction']) || 'directed',
      start_date: edge.start_date,
      end_date: edge.end_date,
      confirmed: edge.confirmed,
      notes: edge.notes,
    });
    setSelEdgeId(edge.id);
  };

  const submitForm = async () => {
    if (!form || saving) return;
    if (!form.from_person_id || !form.to_person_id || !form.relation_type.trim()) {
      setError('请选择两个人物并填写关系类型。');
      return;
    }
    if (form.from_person_id === form.to_person_id) {
      setError('关系的两端不能是同一个人。');
      return;
    }
    setSaving(true);
    setError('');
    try {
      const payload: Partial<Relationship> = {
        from_person_id: form.from_person_id,
        to_person_id: form.to_person_id,
        relation_type: form.relation_type.trim(),
        direction: form.direction,
        start_date: form.start_date,
        end_date: form.end_date,
        confirmed: form.confirmed,
        notes: form.notes,
      };
      if (form.id) await relationshipApi.update(form.id, payload);
      else await relationshipApi.create(payload);
      setForm(null);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const endRelationship = async (edge: GraphEdge) => {
    setSaving(true);
    setError('');
    try {
      await relationshipApi.update(edge.id, {
        from_person_id: edge.from,
        to_person_id: edge.to,
        relation_type: edge.type,
        direction: (edge.direction as Relationship['direction']) || 'directed',
        start_date: edge.start_date,
        end_date: today(),
        confirmed: edge.confirmed,
        notes: edge.notes,
      });
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const deleteRelationship = async (edge: GraphEdge) => {
    setSaving(true);
    setError('');
    try {
      await relationshipApi.remove(edge.id);
      setSelEdgeId('');
      setForm(null);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const renderEdge = (edge: GraphEdge, kind: 'rel' | 'pos') => {
    const a = layout.get(edge.from);
    const b = layout.get(edge.to);
    if (!a || !b) return null;
    const selected = edge.id === selEdgeId;
    const ended = !!edge.end_date;
    const unconfirmed = edge.kind === 'relationship' && !edge.confirmed;
    const dimmed = connected && !connected.edgeIds.has(edge.id) ? 'opacity-15' : '';
    const strokeClass =
      kind === 'pos'
        ? selected
          ? 'stroke-primary'
          : 'stroke-primary/50'
        : ended
          ? selected
            ? 'stroke-muted-foreground'
            : 'stroke-muted-foreground/60'
          : selected
            ? 'stroke-primary'
            : 'stroke-foreground/35';
    const dash =
      kind === 'pos' ? '5 3' : ended ? '2 4' : unconfirmed ? '8 3 2 3' : undefined;
    // 端点留出节点半径，箭头才有意义。
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const len = Math.hypot(dx, dy) || 1;
    const ra = nodeRadius(edge.from);
    const rb = nodeRadius(edge.to);
    const x1 = a.x + (dx / len) * ra;
    const y1 = a.y + (dy / len) * ra;
    const x2 = b.x - (dx / len) * (rb + (edge.direction === 'directed' && kind === 'rel' ? 9 : 0));
    const y2 = b.y - (dy / len) * (rb + (edge.direction === 'directed' && kind === 'rel' ? 9 : 0));
    return (
      <line
        key={edge.id}
        x1={x1}
        y1={y1}
        x2={x2}
        y2={y2}
        className={`${strokeClass} ${dimmed} cursor-pointer transition-opacity`}
        strokeWidth={selected ? 2.5 : 1.5}
        strokeDasharray={dash}
        markerEnd={kind === 'rel' && edge.direction === 'directed' ? 'url(#arrow)' : undefined}
        onClick={() => {
          setSelNodeId('');
          setSelEdgeId(edge.id);
        }}
        onDoubleClick={() => openEdit(edge)}
      >
        <title>
          {kind === 'pos'
            ? `${nodeName(edge.from)} 在 ${nodeName(edge.to)}${edge.type ? ` · ${edge.type}` : '（任职中）'}`
            : `${nodeName(edge.from)} —${edge.direction === 'directed' ? '→' : '—'} ${nodeName(edge.to)} · ${edge.type}${ended ? `（已结束 ${edge.end_date}）` : ''}`}
        </title>
      </line>
    );
  };

  const renderCoLink = (link: GraphCoLink) => {
    const a = layout.get(link.a);
    const b = layout.get(link.b);
    if (!a || !b) return null;
    const key = `co:${link.a}:${link.b}`;
    const selected = selEdgeId === key;
    const dimmed = connected && !connected.edgeIds.has(key) ? 'opacity-10' : '';
    return (
      <line
        key={key}
        x1={a.x}
        y1={a.y}
        x2={b.x}
        y2={b.y}
        className={`stroke-muted-foreground/40 ${dimmed} cursor-pointer`}
        strokeWidth={selected ? 2 : 1}
        strokeDasharray="1 4"
        onClick={() => {
          setSelNodeId('');
          setSelEdgeId(key);
        }}
      >
        <title>
          共同经历：{link.a_name} × {link.b_name} · {link.shared_count} 条共同记录（仅为同场出现，不是关系）
        </title>
      </line>
    );
  };

  const nodeRadius = (id: string) => {
    const node = nodeById.get(id);
    if (!node || 'member_count' in node) return 8;
    return 9 + Math.min((node as GraphNode).event_count, 30) * 0.35;
  };

  const renderNode = (id: string) => {
    const node = nodeById.get(id);
    const p = layout.get(id);
    if (!node || !p) return null;
    const selected = selNodeId === id;
    const dimmed = connected && !connected.ids.has(id) ? 'opacity-20' : '';
    if ('member_count' in node) {
      const org = node as GraphOrg;
      return (
        <g key={id} className={`${dimmed} cursor-pointer`} onClick={() => { setSelEdgeId(''); setSelNodeId(id); }}>
          <rect
            x={p.x - 36}
            y={p.y - 13}
            width={72}
            height={26}
            rx={8}
            className={selected ? 'fill-primary/15 stroke-primary' : 'fill-muted stroke-border'}
            strokeWidth={selected ? 2 : 1.2}
          />
          <text x={p.x} y={p.y + 4} textAnchor="middle" className="fill-foreground text-[10px]">
            {org.name}
          </text>
        </g>
      );
    }
    const person = node as GraphNode;
    const r = nodeRadius(id);
    return (
      <g
        key={id}
        className={`${dimmed} cursor-pointer`}
        onMouseEnter={() => setHoverId(id)}
        onMouseLeave={() => setHoverId('')}
        onClick={() => {
          setSelEdgeId('');
          setSelNodeId(id);
        }}
      >
        <circle
          cx={p.x}
          cy={p.y}
          r={r}
          className={
            selected || person.is_self
              ? 'fill-primary/20 stroke-primary'
              : 'fill-card stroke-primary/50'
          }
          strokeWidth={selected || person.is_self ? 2.2 : 1.4}
        />
        <text x={p.x} y={p.y + r + 12} textAnchor="middle" className="fill-foreground text-[11px]">
          {person.name}
          {person.is_self ? '（我）' : ''}
        </text>
      </g>
    );
  };

  const edgeTypes = useMemo(() => {
    const set = new Set<string>(types);
    for (const e of graph?.edges ?? []) if (e.kind === 'relationship') set.add(e.type);
    return [...set].sort();
  }, [types, graph]);

  return (
    <div>
      <PageHeader
        title="关系图谱"
        description="人物与组织的连线：关系可手动记录，也可按职位让 AI 推断上下级（推断结果待确认）；共同经历只作参考"
        actions={
          <>
            <Button variant="outline" onClick={() => void inferHierarchy()} disabled={inferring} title="按各组织在职成员的职位，让 AI 推断直接上下级">
              {inferring ? '推断中，可能需要几十秒…' : 'AI 推断上下级'}
            </Button>
            <Button
              variant="outline"
              onClick={() => {
                // 从选中的person节点发起新关系，省一次下拉选择。
                setForm({ ...EMPTY_FORM, from_person_id: selNodeId && isPerson(selNodeId) ? selNodeId : '' });
                setSelEdgeId('');
              }}
            >
              添加关系
            </Button>
          </>
        }
      />
      {inferNotice ? (
        <div className="mb-4">
          <Notice>{inferNotice}</Notice>
        </div>
      ) : null}
      {error ? (
        <div className="mb-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      ) : null}
      {graph?.truncated ? (
        <div className="mb-4">
          <Notice>
            图谱节点数超出单页上限，当前只显示 {graph.nodes.length} / {graph.nodes_total} 个人物。
            用搜索或筛选定位目标，数据量长期超出时考虑给图谱页加分页。
          </Notice>
        </div>
      ) : null}

      <SectionCard className="mb-5" bodyClassName="p-3">
        <div className="flex flex-wrap items-center gap-2">
          <select value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} className={`${controlClass} h-9 w-32`}>
            <option value="">全部类型</option>
            {edgeTypes.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        <select value={orgFilter} onChange={(e) => setOrgFilter(e.target.value)} className={`${controlClass} h-9 w-36`}>
          <option value="">全部组织</option>
          {orgs.map((org) => (
            <option key={org.id} value={org.id}>
              {org.name}
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1">
          <input type="date" value={fromFilter} onChange={(e) => setFromFilter(e.target.value)} className={`${controlClass} h-9 w-36`} title="筛选该时间段内存续的关系（起止相交即算）" />
          <span className="text-xs text-muted-foreground">至</span>
          <input type="date" value={toFilter} onChange={(e) => setToFilter(e.target.value)} className={`${controlClass} h-9 w-36`} title="筛选该时间段内存续的关系（起止相交即算）" />
        </div>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input type="checkbox" checked={showEnded} onChange={(e) => setShowEnded(e.target.checked)} />
          含已结束
        </label>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground" title="共同经历由参与人派生，仅展示，不代表关系">
          <input type="checkbox" checked={showCo} onChange={(e) => setShowCo(e.target.checked)} />
          共同经历
        </label>
        <span className="ml-auto text-xs text-muted-foreground">
          {visible.personIds.size} 人 · {visible.orgIds.size} 个组织 ·{' '}
          {visible.relEdges.length + visible.posEdges.length + visible.coLinks.length} 条连线
        </span>
        </div>
      </SectionCard>

      <div className="flex flex-col gap-4 xl:flex-row">
        <div className="min-w-0 flex-1 rounded-xl border border-border bg-card p-2 shadow-sm">
          {loading ? (
            <Spinner label="载入图谱…" />
          ) : visible.personIds.size + visible.orgIds.size === 0 ? (
            <p className="py-24 text-center text-sm text-muted-foreground">
              还没有可展示的节点。先到首页创建人物，或换个组织筛选条件。
            </p>
          ) : (
            <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full select-none">
              <defs>
                <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                  <path d="M 0 1 L 9 5 L 0 9" fill="none" className="stroke-foreground/60" strokeWidth={1.6} />
                </marker>
              </defs>
              {visible.coLinks.map(renderCoLink)}
              {visible.relEdges.map((e) => renderEdge(e, 'rel'))}
              {visible.posEdges.map((e) => renderEdge(e, 'pos'))}
              {[...visible.personIds, ...visible.orgIds].map(renderNode)}
            </svg>
          )}
          <div className="flex flex-wrap gap-4 px-3 pb-2 pt-1 text-[11px] text-muted-foreground">
            <span>— 实线 = 关系边（箭头为方向）</span>
            <span>╌ 任职（现任）</span>
            <span>┄ 灰虚线 = 已结束</span>
            {showCo ? <span>┈ 点线 = 共同经历（同场出现，非关系）</span> : null}
            <span>双击关系边可直接编辑</span>
          </div>
        </div>

        <aside className="w-full shrink-0 space-y-4 xl:w-96">
          {/* 选中边详情 */}
          {selEdge && (selEdge.kind === 'relationship' || selEdge.kind === 'position') ? (
            <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
              {selEdge.kind === 'relationship' ? (
                <>
                  <div className="mb-2 flex items-center justify-between">
                    <h3 className="text-sm font-semibold text-foreground">
                      {nodeName(selEdge.from)} {selEdge.direction === 'directed' ? '→' : '—'} {nodeName(selEdge.to)}
                    </h3>
                    <span className={`rounded-full px-2 py-0.5 text-xs ${selEdge.confirmed ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'}`}>
                      {selEdge.confirmed ? '已确认' : '待确认'}
                    </span>
                  </div>
                  <dl className="space-y-1 text-sm text-muted-foreground">
                    <div>类型：{selEdge.type}</div>
                    <div>
                      起止：{selEdge.start_date ? fullDate(selEdge.start_date) : '未知'} ～{' '}
                      {selEdge.end_date ? fullDate(selEdge.end_date) : '至今'}
                    </div>
                    {selEdge.notes ? <div className="whitespace-pre-wrap">备注：{selEdge.notes}</div> : null}
                    <div>
                      来源记录：
                      {selEdge.source_event_id ? (
                        <Link to={`/events/${selEdge.source_event_id}`} className="text-primary hover:underline">
                          查看记录
                        </Link>
                      ) : (
                        '无'
                      )}
                    </div>
                  </dl>
                  <div className="mt-3 flex gap-2">
                    <Button variant="outline" size="sm" onClick={() => openEdit(selEdge)}>
                      编辑
                    </Button>
                    {!selEdge.end_date ? (
                      <Button variant="outline" size="sm" disabled={saving} onClick={() => void endRelationship(selEdge)}>
                        结束关系
                      </Button>
                    ) : null}
                    <Button variant="outline" size="sm" disabled={saving} onClick={() => void deleteRelationship(selEdge)}>
                      删除
                    </Button>
                  </div>
                </>
              ) : (
                <>
                  <h3 className="mb-2 text-sm font-semibold text-foreground">任职</h3>
                  <p className="text-sm text-muted-foreground">
                    {nodeName(selEdge.from)} 现任 {nodeName(selEdge.to)}
                    {selEdge.type ? ` · ${selEdge.type}` : ''}
                    {selEdge.start_date ? `（自 ${fullDate(selEdge.start_date)}）` : ''}
                  </p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    任职在{' '}
                    <Link to="/organizations" className="text-primary hover:underline">
                      组织页
                    </Link>{' '}
                    管理；历史任职不画进图谱。
                  </p>
                </>
              )}
            </div>
          ) : null}

          {/* 共同经历详情 */}
          {selEdgeId.startsWith('co:') ? (
            <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
              <h3 className="mb-2 text-sm font-semibold text-foreground">共同经历</h3>
              {(() => {
                const link = visible.coLinks.find((c) => `co:${c.a}:${c.b}` === selEdgeId);
                if (!link) return null;
                return (
                  <div className="space-y-1 text-sm text-muted-foreground">
                    <div>
                      {link.a_name} × {link.b_name}
                    </div>
                    <div>{link.shared_count} 条共同记录</div>
                    <p className="text-xs">
                      这是「两人出现在同一条记录」的派生事实，<b>不是</b>朋友、同事或上下级关系；
                      要表达确定的关系，请用「添加关系」建立结构化边。
                    </p>
                  </div>
                );
              })()}
            </div>
          ) : null}

          {/* 选中节点详情 */}
          {selNode ? (
            <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
              {'member_count' in selNode ? (
                <>
                  <h3 className="text-sm font-semibold text-foreground">{(selNode as GraphOrg).name}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {(selNode as GraphOrg).kind || '组织'} · 现任 {(selNode as GraphOrg).member_count} 人
                  </p>
                  <Link to="/organizations" className="mt-2 inline-block text-sm text-primary hover:underline">
                    打开组织页 →
                  </Link>
                </>
              ) : (
                <>
                  <h3 className="text-sm font-semibold text-foreground">
                    {(selNode as GraphNode).name}
                    {(selNode as GraphNode).is_self ? '（我）' : ''}
                  </h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {[
                      (selNode as GraphNode).org_name,
                      (selNode as GraphNode).event_count ? `${(selNode as GraphNode).event_count} 条记录` : '尚无记录',
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </p>
                  <Link to={`/persons/${selNode.id}`} className="mt-2 inline-block text-sm text-primary hover:underline">
                    打开人物详情 →
                  </Link>
                  <div className="mt-3 space-y-1 text-sm">
                    <div className="text-xs font-medium text-muted-foreground">相关关系</div>
                    {[...visible.relEdges, ...visible.posEdges]
                      .filter((e) => e.from === selNode.id || e.to === selNode.id)
                      .map((e) => (
                        <button
                          key={e.id}
                          onClick={() => {
                            setSelNodeId('');
                            setSelEdgeId(e.id);
                          }}
                          className="block w-full truncate rounded px-2 py-1 text-left text-muted-foreground hover:bg-muted"
                        >
                          {e.kind === 'position'
                            ? `任职 ${nodeName(e.to)}${e.type ? ` · ${e.type}` : ''}`
                            : `${nodeName(e.from)} — ${e.type} — ${nodeName(e.to)}${e.end_date ? '（已结束）' : ''}`}
                        </button>
                      ))}
                  </div>
                </>
              )}
            </div>
          ) : null}

          {/* 关系表单 */}
          {form ? (
            <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
              <h3 className="mb-3 text-sm font-semibold text-foreground">{form.id ? '编辑关系' : '添加关系'}</h3>
              <div className="space-y-2">
                <div className="flex gap-2">
                  <select
                    value={form.from_person_id}
                    onChange={(e) => setForm({ ...form, from_person_id: e.target.value })}
                    className={`${controlClass} text-muted-foreground`}
                    title="关系的主体（箭头起点）"
                  >
                    <option value="">从谁…</option>
                    {nodes.map((node) => (
                      <option key={node.id} value={node.id}>
                        {node.name}
                        {node.is_self ? '（我）' : ''}
                      </option>
                    ))}
                  </select>
                  <select
                    value={form.direction}
                    onChange={(e) => setForm({ ...form, direction: e.target.value as RelationshipForm['direction'] })}
                    className={`${controlClass} w-28 text-muted-foreground`}
                  >
                    <option value="directed">→ 方向</option>
                    <option value="undirected">— 双向</option>
                  </select>
                  <select
                    value={form.to_person_id}
                    onChange={(e) => setForm({ ...form, to_person_id: e.target.value })}
                    className={`${controlClass} text-muted-foreground`}
                    title="关系的客体（箭头终点）"
                  >
                    <option value="">到谁…</option>
                    {nodes.map((node) => (
                      <option key={node.id} value={node.id}>
                        {node.name}
                        {node.is_self ? '（我）' : ''}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <input
                    value={form.relation_type}
                    onChange={(e) => setForm({ ...form, relation_type: e.target.value })}
                    list="rel-type-options"
                    placeholder="关系类型，如 上级 / 朋友"
                    className={`${controlClass} flex-1`}
                  />
                  <datalist id="rel-type-options">
                    {edgeTypes.map((t) => (
                      <option key={t} value={t} />
                    ))}
                  </datalist>
                  <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                    <input type="checkbox" checked={!!form.confirmed} onChange={(e) => setForm({ ...form, confirmed: e.target.checked ? 1 : 0 })} />
                    已确认
                  </label>
                </div>
                <div className="flex gap-2">
                  <input
                    type="date"
                    value={form.start_date}
                    onChange={(e) => setForm({ ...form, start_date: e.target.value })}
                    className={`${controlClass} flex-1`}
                    title="开始日期（可空=未知）"
                  />
                  <input
                    type="date"
                    value={form.end_date}
                    onChange={(e) => setForm({ ...form, end_date: e.target.value })}
                    className={`${controlClass} flex-1`}
                    title="结束日期（空=进行中）"
                  />
                </div>
                <textarea
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                  placeholder="备注（可空）"
                  className={`${controlClass} min-h-16`}
                />
                <div className="flex justify-end gap-2">
                  <Button variant="outline" size="sm" onClick={() => setForm(null)}>
                    取消
                  </Button>
                  <Button size="sm" disabled={saving} onClick={() => void submitForm()}>
                    {saving ? '保存中…' : '保存'}
                  </Button>
                </div>
              </div>
            </div>
          ) : null}

          {!selEdge && !selNode && !form ? (
            <div className="rounded-xl border border-dashed border-border bg-card p-6 text-center text-sm text-muted-foreground">
              点击节点查看人物，点击边查看关系详情与来源记录；双击关系边可直接编辑。
            </div>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
