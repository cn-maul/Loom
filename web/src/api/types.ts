export interface Organization {
  id: string;
  name: string;
  /** Free-text type in the user's words, e.g. 公司 / 学校. */
  kind: string;
  description: string;
  /** Present only when the organisation is archived. */
  archived_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Person {
  id: string;
  name: string;
  relation: string;
  importance: number;
  notes: string;
  org_id: string;
  position: string;
  created_at: string;
  updated_at: string;
}

export interface PersonWithActivity extends Person {
  last_event_date: string;
  event_count: number;
  org_name: string;
}

/** Query for GET /persons. org_id accepts the "none" sentinel for 未归属. */
export interface PersonListParams {
  q?: string;
  org_id?: string;
  relation?: string;
  sort?: 'recent' | 'name' | 'importance' | 'created';
  limit?: number;
  offset?: number;
}

/** 结构化人物关系（person_relationships 表）。direction: directed|undirected */
export interface Relationship {
  id: string;
  from_person_id: string;
  to_person_id: string;
  relation_type: string;
  direction: 'directed' | 'undirected';
  start_date: string;
  end_date: string;
  source_event_id: string;
  confirmed: number;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface RelationshipLink extends Relationship {
  from_person_name: string;
  to_person_name: string;
}

/** GET /api/graph 的一次性画布数据。 */
export interface GraphData {
  nodes: GraphNode[];
  orgs: GraphOrg[];
  edges: GraphEdge[];
  co_attendance: GraphCoLink[];
}

export interface GraphNode {
  id: string;
  name: string;
  is_self: number;
  org_id: string;
  org_name: string;
  importance: number;
  event_count: number;
}

export interface GraphOrg {
  id: string;
  name: string;
  kind: string;
  member_count: number;
}

/** kind: relationship = 人物↔人物；position = 人物→组织（现任任职）。 */
export interface GraphEdge {
  id: string;
  kind: 'relationship' | 'position';
  from: string;
  to: string;
  type: string;
  direction: string;
  start_date: string;
  end_date: string;
  confirmed: number;
  source_event_id: string;
  notes: string;
}

/** 共同经历：两人出现在同一条记录（含锚定人），只是展示，绝不是关系边。 */
export interface GraphCoLink {
  a: string;
  b: string;
  a_name: string;
  b_name: string;
  shared_count: number;
}

export interface EventPromise {
  who: string;
  what: string;
  deadline: string;
}

/** One attendee of a record. The anchored person (person_id) is always in the
 *  list and carries the `primary` role. */
export interface EventParticipant {
  event_id?: string;
  person_id: string;
  person_name?: string;
  /** Free text for how they took part, e.g. primary / 在场 / 旁听. */
  role: string;
}

export interface Event {
  id: string;
  person_id: string;
  raw_text: string;
  event_date: string;
  summary: string;
  /** 见面 / 通话 / 消息 … how the record happened. */
  record_type: string;
  /** 当面 / 微信 … where it happened. */
  channel: string;
  my_feeling: string;
  their_reaction: string;
  promises: EventPromise[];
  created_at: string;
  /** pending | succeeded | failed — what the extraction pipeline managed to do. */
  extraction_status: string;
  extraction_error: string;
  extracted_at?: string;
  /** 1 when a human curated the stored result; a retry then needs force. */
  manually_edited: number;
  edited_at?: string;
  participants: EventParticipant[];
}

/** Query for GET /events. */
export interface EventListParams {
  person_id?: string;
  from?: string;
  to?: string;
  q?: string;
  /** pending | succeeded | failed; omit for every state. */
  status?: string;
  limit?: number;
  offset?: number;
}

export interface IngestReport {
  extracted: boolean;
  vectorized: boolean;
  traits_updated: number;
  warnings: string[];
}

export interface IngestResult {
  event: Event;
  report: IngestReport;
}

export interface Trait {
  id: string;
  person_id: string;
  trait_key: string;
  trait_value: string;
  confidence: number;
  source_event_ids: string[];
  verified: number;
  updated_at: string;
  /** 1 when the records this note was derived from were later edited or deleted. */
  source_stale: number;
  source_stale_reason: string;
}

export interface AdviceRequest {
  person_id: string;
  question: string;
  goal?: string;
}

/** One conclusion together with the records it rests on. An empty evidence list
 *  means the line is reasoning, not something drawn from the timeline. */
export interface AdvicePoint {
  text: string;
  evidence_event_ids: string[];
}

export interface Strategy {
  name: string;
  script: string;
  pros: string;
  cons: string;
  evidence_event_ids: string[];
}

export interface AdviceResponse {
  situation: AdvicePoint;
  other_perspective: AdvicePoint;
  risks: AdvicePoint[];
  strategies: Strategy[];
  follow_up: AdvicePoint;
  evidence_event_ids: string[];
  vector_used: boolean;
  /** vector_used | no_relevant_evidence | retrieval_failed | vector_disabled */
  retrieval_status: string;
}

/** What the evidence looked like when the answer was drawn; staleness is the
 *  difference between this and the records as they are now. */
export interface EvidenceVersion {
  event_id: string;
  content_rev: string;
}

export interface AdviceSession extends AdviceResponse {
  id: string;
  person_id: string;
  question: string;
  goal: string;
  used_event_ids: string[];
  used_trait_ids: string[];
  evidence_version: EvidenceVersion[];
  model: string;
  adopted_strategy_index?: number;
  adopted_strategy_name: string;
  follow_up_ids: string[];
  source_stale: number;
  source_stale_reason: string;
  created_at: string;
  updated_at: string;
}

export interface AdoptRequest {
  strategy_index: number;
  title?: string;
  owner?: string;
  due_text?: string;
  due_date?: string;
}

export const RETRIEVAL_LABEL: Record<string, string> = {
  vector_used: '语义检索 + 近期记录',
  no_relevant_evidence: '仅近期记录（没有检索到相关记录）',
  retrieval_failed: '仅近期记录（语义检索失败）',
  vector_disabled: '仅近期记录（未启用语义检索）',
};

/** One stint of one person at one organisation, with display names attached. */
export interface OrgPositionLink {
  id: string;
  person_id: string;
  org_id: string;
  role: string;
  start_date: string;
  end_date: string;
  notes: string;
  person_name: string;
  org_name: string;
}

export interface FollowUp {
  id: string;
  person_id: string;
  title: string;
  description: string;
  /** Free text for whose move it is, e.g. 我 / 对方. */
  owner: string;
  /** The wording from the record, e.g. 下周三前; kept beside the confirmed date. */
  due_text: string;
  due_date: string;
  status: 'pending' | 'waiting' | 'completed' | 'cancelled';
  source_event_id: string;
  completion_note: string;
  completed_event_id: string;
  source_advice_id: string;
  source_advice_stale: number;
  source_advice_stale_reason: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface FollowUpPostponement {
  id: string;
  follow_up_id: string;
  old_due_date: string;
  new_due_date: string;
  reason: string;
  created_at: string;
}

export const FOLLOW_UP_STATUS_LABEL: Record<FollowUp['status'], string> = {
  pending: '待办',
  waiting: '等待对方',
  completed: '已完成',
  cancelled: '已取消',
};

// One request for one period. week_of replaces start/end with the natural
// week (Monday to Sunday) containing that date.
export interface ReportRequest {
  person_id?: string;
  start?: string;
  end?: string;
  week_of?: string;
}

export interface ReportFollowUpRef {
  id: string;
  person_id: string;
  person_name: string;
  title: string;
  description: string;
  status: string;
  owner: string;
  due_date: string;
  due_text: string;
  days_overdue: number;
  source_event_id: string;
  source_advice_id: string;
}

export interface ReportEventRef {
  id: string;
  person_id: string;
  person_name: string;
  event_date: string;
  summary: string;
  record_type: string;
  extraction_status: string;
}

export interface ReportPromise {
  event_id: string;
  person_id: string;
  person_name: string;
  event_date: string;
  who: string;
  what: string;
  deadline: string;
}

export interface ReportChangeRef {
  kind: string;
  id: string;
  person_id: string;
  person_name: string;
  counterpart_id: string;
  counterpart_name: string;
  description: string;
  date: string;
}

export interface ReportPersonSummary {
  person_id: string;
  person_name: string;
  event_count: number;
  last_event_date: string;
  open_follow_ups: number;
}

export interface Report {
  id: string;
  person_id: string;
  person_name: string;
  start: string;
  end: string;
  days: number;
  status: 'succeeded' | 'failed';
  generated_by: string;
  failure_reason?: string;
  generated_at: string;
  summary: string;
  overdue: ReportFollowUpRef[];
  due_soon: ReportFollowUpRef[];
  waiting: ReportFollowUpRef[];
  carried_over: ReportFollowUpRef[];
  upcoming: ReportFollowUpRef[];
  completed: ReportFollowUpRef[];
  changes: ReportChangeRef[];
  events: ReportEventRef[];
  promises: ReportPromise[];
  persons: ReportPersonSummary[];
  event_count: number;
  promise_count: number;
  open_count: number;
}

// The history row: window, status and headline only. The sections are fetched
// by id so listing a year of reports does not ship a year of payloads.
export interface ReportSnapshot {
  id: string;
  person_id: string;
  person_name: string;
  start: string;
  end: string;
  status: 'succeeded' | 'failed';
  generated_by: string;
  failure_reason?: string;
  summary: string;
  event_count: number;
  generated_at: string;
}

export interface ReindexResult {
  events_indexed: number;
  traits_indexed: number;
  failed: number;
}

export interface LLMConfig {
  endpoint: string;
  protocol: string;
  api_key: string;
  extract_model: string;
  advice_model: string;
  embed_endpoint: string;
  embed_api_key: string;
  embed_model: string;
  embed_dim: number;
  max_tokens: number;
}

export interface ServerConfig {
  port: number;
  host: string;
}

export interface DatabaseConfig {
  path: string;
}

export interface AppConfig {
  server: ServerConfig;
  database: DatabaseConfig;
  llm: LLMConfig;
}

export interface ConfigUpdateResult {
  config: AppConfig;
  reindex_required: boolean;
}

export interface APIResponse<T> {
  ok: boolean;
  data: T;
  error?: {
    code: string;
    message: string;
  };
}
