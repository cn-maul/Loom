import type {
  AdviceRequest,
  AdviceResponse,
  AppConfig,
  APIResponse,
  ConfigUpdateResult,
  Event,
  IngestResult,
  LLMConfig,
  Organization,
  Person,
  PersonWithActivity,
  ReindexResult,
  Trait,
  WeeklyReport,
} from './types';

const BASE_URL = '/api';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${url}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });

  // Error bodies are JSON, but a crashed proxy or a raw 404 page is not, so a
  // failed parse must still surface as a readable error rather than a SyntaxError.
  const text = await response.text();
  let parsed: APIResponse<T> | null = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as APIResponse<T>;
    } catch {
      parsed = null;
    }
  }

  if (!parsed || !parsed.ok) {
    throw new Error(parsed?.error?.message ?? `请求失败（HTTP ${response.status}）`);
  }
  return parsed.data;
}

export const personApi = {
  list: () => request<PersonWithActivity[]>('/persons'),
  get: (id: string) => request<Person>(`/persons/${id}`),
  create: (person: Partial<Person>) => request<Person>('/persons', { method: 'POST', body: JSON.stringify(person) }),
  update: (id: string, person: Partial<Person>) =>
    request<Person>(`/persons/${id}`, { method: 'PUT', body: JSON.stringify(person) }),
  delete: (id: string) => request<null>(`/persons/${id}`, { method: 'DELETE' }),
  events: (id: string, limit?: number) =>
    request<Event[]>(`/persons/${id}/events${limit ? `?limit=${limit}` : ''}`),
  traits: (id: string) => request<Trait[]>(`/persons/${id}/traits`),
};

export const organizationApi = {
  list: () => request<Organization[]>('/organizations'),
  create: (org: Partial<Organization>) => request<Organization>('/organizations', { method: 'POST', body: JSON.stringify(org) }),
  update: (id: string, org: Partial<Organization>) =>
    request<Organization>(`/organizations/${id}`, { method: 'PUT', body: JSON.stringify(org) }),
  delete: (id: string) => request<null>(`/organizations/${id}`, { method: 'DELETE' }),
};

export const eventApi = {
  get: (id: string) => request<Event>(`/events/${id}`),
  create: (event: { person_id: string; event_date: string; raw_text: string }) =>
    request<IngestResult>('/events', { method: 'POST', body: JSON.stringify(event) }),
  delete: (id: string) => request<null>(`/events/${id}`, { method: 'DELETE' }),
};

export const traitApi = {
  verify: (id: string, verified: number) =>
    request<null>(`/traits/${id}/verify`, { method: 'PUT', body: JSON.stringify({ verified }) }),
};

export const aiApi = {
  advice: (req: AdviceRequest) => request<AdviceResponse>('/ai/advice', { method: 'POST', body: JSON.stringify(req) }),
  weeklyReport: (personId?: string) =>
    request<WeeklyReport>(`/ai/report/weekly${personId ? `?person_id=${encodeURIComponent(personId)}` : ''}`),
  reindex: () => request<ReindexResult>('/ai/reindex', { method: 'POST' }),
  embeddingStatus: () => request<{ available: boolean }>('/ai/embeddings/status'),
};

export const configApi = {
  get: () => request<AppConfig>('/config'),
  update: (llmConfig: Partial<LLMConfig>) =>
    request<ConfigUpdateResult>('/config', { method: 'PUT', body: JSON.stringify(llmConfig) }),
};
