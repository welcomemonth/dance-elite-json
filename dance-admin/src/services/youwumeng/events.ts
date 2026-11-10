import { request } from '@umijs/max';
import type {
  AudioReviewTokenResult,
  EventBrief,
  EventDetail,
  EventInput,
  EventStatus,
  EventLevel,
  PagedList,
} from './typings';

export interface EventListParams {
  status?: EventStatus;
  level?: EventLevel;
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/events */
export async function listEvents(params: EventListParams): Promise<PagedList<EventBrief>> {
  return request('/api/v1/admin/events', { method: 'GET', params });
}

/** GET /api/v1/admin/events/{event_id}（含完整组别与进度） */
export async function getEvent(eventId: number): Promise<EventDetail> {
  return request(`/api/v1/admin/events/${eventId}`, { method: 'GET' });
}

/** POST /api/v1/admin/events */
export async function createEvent(data: EventInput): Promise<{ event_id: number; status: string }> {
  return request('/api/v1/admin/events', { method: 'POST', data });
}

/** PUT /api/v1/admin/events/{event_id} */
export async function updateEvent(
  eventId: number,
  data: Omit<EventInput, 'publish'>,
): Promise<{ event_id: number; status: string }> {
  return request(`/api/v1/admin/events/${eventId}`, { method: 'PUT', data });
}

/** POST /api/v1/admin/events/{event_id}/publish */
export async function publishEvent(eventId: number): Promise<{ event_id: number; status: string }> {
  return request(`/api/v1/admin/events/${eventId}/publish`, { method: 'POST' });
}

/** POST /api/v1/admin/events/{event_id}/offline */
export async function offlineEvent(eventId: number): Promise<{ event_id: number; status: string }> {
  return request(`/api/v1/admin/events/${eventId}/offline`, { method: 'POST' });
}

/** PUT /api/v1/admin/events/{event_id}/groups/{group_id}/quota */
export async function setGroupQuota(
  eventId: number,
  groupId: number,
  quota: number,
): Promise<{ group_id: number; quota: number; registered: number; remaining: number }> {
  return request(`/api/v1/admin/events/${eventId}/groups/${groupId}/quota`, {
    method: 'PUT',
    data: { quota },
  });
}

/** POST /api/v1/admin/events/{event_id}/groups/{group_id}/close */
export async function closeGroup(eventId: number, groupId: number): Promise<{ group_id: number; enabled: boolean }> {
  return request(`/api/v1/admin/events/${eventId}/groups/${groupId}/close`, { method: 'POST' });
}

/** POST /api/v1/admin/events/{event_id}/groups/{group_id}/reopen */
export async function reopenGroup(eventId: number, groupId: number): Promise<{ group_id: number; enabled: boolean }> {
  return request(`/api/v1/admin/events/${eventId}/groups/${groupId}/reopen`, { method: 'POST' });
}

/** POST /api/v1/admin/events/{event_id}/audio-review-token */
export async function generateAudioReviewToken(eventId: number): Promise<AudioReviewTokenResult> {
  return request(`/api/v1/admin/events/${eventId}/audio-review-token`, { method: 'POST' });
}
