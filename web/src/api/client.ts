import type { APIResponse, AppConfig, LLMConfig } from './types';

const BASE_URL = '/api';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${url}`, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  });

  const data: APIResponse<T> = await response.json();

  if (!data.ok) {
    throw new Error(data.error?.message || 'Unknown error');
  }

  return data.data;
}

// Person API
export const personApi = {
  list: () => request<any[]>('/persons'),
  get: (id: string) => request<any>(`/persons/${id}`),
  create: (person: Partial<any>) =>
    request<any>('/persons', {
      method: 'POST',
      body: JSON.stringify(person),
    }),
  update: (id: string, person: Partial<any>) =>
    request<any>(`/persons/${id}`, {
      method: 'PUT',
      body: JSON.stringify(person),
    }),
  delete: (id: string) =>
    request<void>(`/persons/${id}`, {
      method: 'DELETE',
    }),
};

// Event API
export const eventApi = {
  listByPerson: (personId: string, limit?: number) =>
    request<any[]>(`/persons/${personId}/events${limit ? `?limit=${limit}` : ''}`),
  get: (id: string) => request<any>(`/events/${id}`),
  create: (event: Partial<any>) =>
    request<any>('/events', {
      method: 'POST',
      body: JSON.stringify(event),
    }),
  delete: (id: string) =>
    request<void>(`/events/${id}`, {
      method: 'DELETE',
    }),
};

// Trait API
export const traitApi = {
  listByPerson: (personId: string) =>
    request<any[]>(`/persons/${personId}/traits`),
  verify: (id: string, verified: number) =>
    request<void>(`/traits/${id}/verify`, {
      method: 'PUT',
      body: JSON.stringify({ verified }),
    }),
};

// AI API
export const aiApi = {
  getAdvice: (req: { person_id: string; question: string }) =>
    request<any>('/ai/advice', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  getWeeklyReport: () => request<any>('/ai/report/weekly'),
};

export const configApi = {
  get: () => request<AppConfig>('/config'),
  update: (llmConfig: Partial<LLMConfig>) =>
    request<AppConfig>('/config', {
      method: 'PUT',
      body: JSON.stringify(llmConfig),
    }),
};