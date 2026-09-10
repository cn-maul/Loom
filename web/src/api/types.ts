export interface Person {
  id: string;
  name: string;
  relation: string;
  importance: number;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface Event {
  id: string;
  person_id: string;
  raw_text: string;
  event_date: string;
  summary: string;
  my_feeling: string;
  their_reaction: string;
  promises: string;
  created_at: string;
}

export interface Trait {
  id: string;
  person_id: string;
  trait_key: string;
  trait_value: string;
  confidence: number;
  source_event_ids: string;
  verified: number;
  updated_at: string;
}

export interface AdviceRequest {
  person_id: string;
  question: string;
}

export interface AdviceResponse {
  situation: string;
  other_perspective: string;
  risks: string[];
  strategies: {
    name: string;
    script: string;
    pros: string;
    cons: string;
  }[];
  evidence_event_ids: string[];
  follow_up: string;
}

export interface LLMConfig {
  endpoint: string;
  protocol: string;
  api_key: string;
  extract_model: string;
  advice_model: string;
  embed_model: string;
  embed_dim: number;
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

export interface APIResponse<T> {
  ok: boolean;
  data: T;
  error?: {
    code: string;
    message: string;
  };
}