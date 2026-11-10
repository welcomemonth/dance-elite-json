import { request } from '@umijs/max';
import type { ContentEntry, ContentKey, ContentValue } from './typings';

/** GET /api/v1/admin/contents/{key} */
export async function getContent(key: ContentKey): Promise<ContentEntry> {
  return request(`/api/v1/admin/contents/${key}`, { method: 'GET' });
}

/** PUT /api/v1/admin/contents/{key} */
export async function putContent(key: ContentKey, value: ContentValue): Promise<ContentEntry> {
  return request(`/api/v1/admin/contents/${key}`, { method: 'PUT', data: { value } });
}
