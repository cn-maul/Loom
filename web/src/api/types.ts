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
  /** male | female | ''，用户显式选择，不是 AI 推断。 */
  gender: string;
  is_self: number;
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
  /** Non-fatal pipeline complaints (skipped indexing, failed profile refresh). */
  pipeline_warnings: string[];
  /** 1 when a human curated the stored result; a retry then needs force. */
  manually_edited: number;
  edited_at?: string;
  participants: EventParticipant[];
}

/** Query for GET /events. */
export interface EventListParams {
  person_id?: string;
  /** 组织过滤：按出席人（主角或参与人）的归属组织筛选；'none' 表示未归属。 */
  org_id?: string;
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
  /** True when extraction was queued: the record is stored, AI 结果稍后到达。 */
  async?: boolean;
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
  rerank_endpoint?: string;
  rerank_api_key?: string;
  rerank_model?: string;
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

// The "what happens with my data" panel. All facts the server already knows;
// none of them include the token or keys themselves, only whether they are set.
export interface PrivacyInfo {
  listens_on: string;
  ai_endpoint: string;
  ai_endpoint_external: boolean;
  sends_raw_text: boolean;
  allow_remote: boolean;
  auth_required: boolean;
  backup_encrypted: boolean;
  config_has_secret: boolean;
}

export interface AuditEntry {
  id: string;
  action: string;
  path: string;
  status: number;
  created_at: string;
}

export interface AuditList {
  entries: AuditEntry[];
}

export interface APIResponse<T> {
  ok: boolean;
  data: T;
  error?: {
    code: string;
    message: string;
  };
}
