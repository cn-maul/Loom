import type {
  AdoptRequest,
  AdviceRequest,
  AdviceSession,
  AppConfig,
  APIResponse,
  ConfigUpdateResult,
  Event,
  EventListParams,
  EventParticipant,
  FollowUp,
  FollowUpPostponement,
  GraphData,
  IngestResult,
  LLMConfig,
  Organization,
  OrgPositionLink,
  Person,
  PersonListParams,
  PersonWithActivity,
  ReindexResult,
  Report,
  ReportRequest,
  ReportSnapshot,
  Relationship,
  RelationshipLink,
  Trait,
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

/** Same as request, but also returns the X-Total-Count header a paged list
 *  carries, so the caller can show the whole match count next to one page. */
async function requestPaged<T>(url: string, init?: RequestInit): Promise<{ data: T; total: number }> {
  const response = await fetch(`${BASE_URL}${url}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
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
  const total = Number.parseInt(response.headers.get('X-Total-Count') ?? '', 10);
  return { data: parsed.data, total: Number.isNaN(total) ? 0 : total };
}

export const personApi = {
  // Server-side search, filter, sort and paging; with no params the whole
  // list comes back in the default (most recent contact) order.
  list: (params?: PersonListParams) => {
    const search = new URLSearchParams();
    if (params?.q) search.set('q', params.q);
    if (params?.org_id) search.set('org_id', params.org_id);
    if (params?.relation) search.set('relation', params.relation);
    if (params?.sort) search.set('sort', params.sort);
    if (params?.limit) search.set('limit', String(params.limit));
    if (params?.offset) search.set('offset', String(params.offset));
    const query = search.toString();
    return request<PersonWithActivity[]>(`/persons${query ? `?${query}` : ''}`);
  },
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
  /** Active organisations only by default; pickers must not offer archived ones. */
  list: (includeArchived?: boolean) =>
    request<Organization[]>(`/organizations${includeArchived ? '?include_archived=true' : ''}`),
  create: (org: Partial<Organization>) => request<Organization>('/organizations', { method: 'POST', body: JSON.stringify(org) }),
  update: (id: string, org: Partial<Organization>) =>
    request<Organization>(`/organizations/${id}`, { method: 'PUT', body: JSON.stringify(org) }),
  archive: (id: string) => request<Organization>(`/organizations/${id}/archive`, { method: 'POST' }),
  restore: (id: string) => request<Organization>(`/organizations/${id}/restore`, { method: 'POST' }),
  delete: (id: string) => request<null>(`/organizations/${id}`, { method: 'DELETE' }),
  members: (id: string, currentOnly?: boolean) =>
    request<OrgPositionLink[]>(`/organizations/${id}/members${currentOnly ? '?current=true' : ''}`),
  addMember: (personId: string, pos: { org_id: string; role?: string; start_date?: string }) =>
    request<OrgPositionLink>(`/persons/${personId}/positions`, { method: 'POST', body: JSON.stringify(pos) }),
  // The positions PUT replaces the whole stint, so person/org/role must travel
  // along with the new end_date rather than being patched.
  endMembership: (pos: OrgPositionLink, endDate: string) =>
    request<OrgPositionLink>(`/positions/${pos.id}`, {
      method: 'PUT',
      body: JSON.stringify({
        person_id: pos.person_id,
        org_id: pos.org_id,
        role: pos.role,
        start_date: pos.start_date,
        end_date: endDate,
        notes: pos.notes,
      }),
    }),
  removeMembership: (positionId: string) => request<null>(`/positions/${positionId}`, { method: 'DELETE' }),
};

export const positionApi = {
  /** Full stint history for one person, current postings first. */
  byPerson: (personId: string) => request<OrgPositionLink[]>(`/persons/${personId}/positions`),
  create: (personId: string, pos: { org_id: string; role?: string; start_date?: string; end_date?: string; notes?: string }) =>
    request<OrgPositionLink>(`/persons/${personId}/positions`, { method: 'POST', body: JSON.stringify(pos) }),
  // Like the org-side helper: the PUT replaces the whole posting, so every
  // field has to travel along even when only one of them changed.
  update: (id: string, pos: Partial<OrgPositionLink> & { person_id: string; org_id: string }) =>
    request<OrgPositionLink>(`/positions/${id}`, { method: 'PUT', body: JSON.stringify(pos) }),
  remove: (id: string) => request<null>(`/positions/${id}`, { method: 'DELETE' }),
};

export const eventApi = {
  get: (id: string) => request<Event>(`/events/${id}`),
  create: (event: { person_id: string; event_date: string; raw_text: string }) =>
    request<IngestResult>('/events', { method: 'POST', body: JSON.stringify(event) }),
  /** Filtered record list; the match count comes back alongside the page. */
  list: (params: EventListParams = {}) => {
    const search = new URLSearchParams();
    if (params.person_id) search.set('person_id', params.person_id);
    if (params.from) search.set('from', params.from);
    if (params.to) search.set('to', params.to);
    if (params.q) search.set('q', params.q);
    if (params.status) search.set('status', params.status);
    if (params.limit) search.set('limit', String(params.limit));
    if (params.offset) search.set('offset', String(params.offset));
    const query = search.toString();
    return requestPaged<Event[]>(`/events${query ? `?${query}` : ''}`);
  },
  // An edit writes the extraction fields a human is allowed to change; the
  // server marks the record manually edited so a retry cannot overwrite it.
  update: (id: string, event: Partial<Event>) =>
    request<Event>(`/events/${id}`, { method: 'PUT', body: JSON.stringify(event) }),
  delete: (id: string) => request<null>(`/events/${id}`, { method: 'DELETE' }),
  participants: (id: string) => request<EventParticipant[]>(`/events/${id}/participants`),
  setParticipants: (id: string, participants: { person_id: string; role?: string }[]) =>
    request<EventParticipant[]>(`/events/${id}/participants`, {
      method: 'PUT',
      body: JSON.stringify({ participants }),
    }),
  removeParticipant: (id: string, personId: string) =>
    request<EventParticipant[]>(`/events/${id}/participants/${personId}`, { method: 'DELETE' }),
  // force is the explicit "yes, overwrite my correction" for a record whose
  // stored extraction was written by hand.
  retryExtract: (id: string, force = false) =>
    request<IngestResult>(`/events/${id}/extract`, { method: 'POST', body: JSON.stringify({ force }) }),
};

export const traitApi = {
  verify: (id: string, verified: number) =>
    request<null>(`/traits/${id}/verify`, { method: 'PUT', body: JSON.stringify({ verified }) }),
};

export const aiApi = {
  reindex: () => request<ReindexResult>('/ai/reindex', { method: 'POST' }),
  embeddingStatus: () => request<{ available: boolean }>('/ai/embeddings/status'),
};

// Reports are snapshots: generating stores one, and the history serves every
// older window exactly as it read at the time.
export const reportApi = {
  generate: (req: ReportRequest = {}) => request<Report>('/reports', { method: 'POST', body: JSON.stringify(req) }),
  list: (personId?: string) => {
    const query = personId ? `?person_id=${encodeURIComponent(personId)}` : '';
    return request<ReportSnapshot[]>(`/reports${query}`);
  },
  get: (id: string) => request<Report>(`/reports/${id}`),
  remove: (id: string) => request<null>(`/reports/${id}`, { method: 'DELETE' }),
};

// Advice is stored server-side, so the history survives a reload: generating
// returns the session, and every later action addresses it by id.
export const adviceApi = {
  generate: (req: AdviceRequest) => request<AdviceSession>('/advice', { method: 'POST', body: JSON.stringify(req) }),
  list: (personId?: string, limit?: number) => {
    const params = new URLSearchParams();
    if (personId) params.set('person_id', personId);
    if (limit) params.set('limit', String(limit));
    const query = params.toString();
    return request<AdviceSession[]>(`/advice${query ? `?${query}` : ''}`);
  },
  get: (id: string) => request<AdviceSession>(`/advice/${id}`),
  remove: (id: string) => request<null>(`/advice/${id}`, { method: 'DELETE' }),
  adopt: (id: string, req: AdoptRequest) =>
    request<AdviceSession>(`/advice/${id}/adopt`, { method: 'POST', body: JSON.stringify(req) }),
};

// Follow-ups are closed over HTTP one action at a time: complete, postpone and
// wait each return the refreshed item, so the list can patch a single row
// without refetching everything.
export const followUpApi = {
  list: (personId?: string, status?: string) => {
    const params = new URLSearchParams();
    if (personId) params.set('person_id', personId);
    if (status) params.set('status', status);
    const query = params.toString();
    return request<FollowUp[]>(`/follow-ups${query ? `?${query}` : ''}`);
  },
  byPerson: (personId: string, status?: string) => {
    const query = status ? `?status=${encodeURIComponent(status)}` : '';
    return request<FollowUp[]>(`/persons/${personId}/follow-ups${query}`);
  },
  get: (id: string) => request<FollowUp>(`/follow-ups/${id}`),
  create: (item: Partial<FollowUp>) =>
    request<FollowUp>('/follow-ups', { method: 'POST', body: JSON.stringify(item) }),
  postpone: (id: string, dueDate: string, reason?: string) =>
    request<FollowUp>(`/follow-ups/${id}/postpone`, {
      method: 'POST',
      body: JSON.stringify({ due_date: dueDate, reason }),
    }),
  complete: (id: string, completionNote?: string) =>
    request<FollowUp>(`/follow-ups/${id}/complete`, {
      method: 'POST',
      body: JSON.stringify(completionNote ? { completion_note: completionNote } : {}),
    }),
  wait: (id: string) => request<FollowUp>(`/follow-ups/${id}/wait`, { method: 'POST' }),
  cancel: (id: string) => request<FollowUp>(`/follow-ups/${id}/cancel`, { method: 'POST' }),
  postponements: (id: string) => request<FollowUpPostponement[]>(`/follow-ups/${id}/postponements`),
};

export const configApi = {
  get: () => request<AppConfig>('/config'),
  update: (llmConfig: Partial<LLMConfig>) =>
    request<ConfigUpdateResult>('/config', { method: 'PUT', body: JSON.stringify(llmConfig) }),
};

// The graph endpoint ships the whole canvas in one payload; filtering happens
// client-side because the dataset is personal-CRM sized.
export const graphApi = {
  get: () => request<GraphData>('/graph'),
};

export const relationshipApi = {
  types: () => request<string[]>('/relationships/types'),
  /** active=true keeps only the relationships that have not ended yet. */
  byPerson: (personId: string, active?: boolean) =>
    request<RelationshipLink[]>(`/persons/${personId}/relationships${active ? '?active=true' : ''}`),
  create: (rel: Partial<Relationship>) =>
    request<Relationship>('/relationships', { method: 'POST', body: JSON.stringify(rel) }),
  update: (id: string, rel: Partial<Relationship>) =>
    request<Relationship>(`/relationships/${id}`, { method: 'PUT', body: JSON.stringify(rel) }),
  remove: (id: string) => request<null>(`/relationships/${id}`, { method: 'DELETE' }),
};
