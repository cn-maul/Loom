export interface Organization {
  id: string;
  name: string;
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

export interface EventPromise {
  who: string;
  what: string;
  deadline: string;
}

export interface Event {
  id: string;
  person_id: string;
  raw_text: string;
  event_date: string;
  summary: string;
  my_feeling: string;
  their_reaction: string;
  promises: EventPromise[];
  created_at: string;
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
}

export interface AdviceRequest {
  person_id: string;
  question: string;
}

export interface Strategy {
  name: string;
  script: string;
  pros: string;
  cons: string;
}

export interface AdviceResponse {
  situation: string;
  other_perspective: string;
  risks: string[];
  strategies: Strategy[];
  evidence_event_ids: string[];
  follow_up: string;
  vector_used: boolean;
}

export interface WeeklySlice {
  person_id: string;
  person_name: string;
  events: string[];
  promises: string[];
}

export interface WeeklyReport {
  start: string;
  end: string;
  event_count: number;
  summary: string;
  persons: WeeklySlice[];
  generated_by: string;
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
