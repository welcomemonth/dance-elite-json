import { request } from '@umijs/max';
import { downloadFile } from './download';
import type { AgeLevel, DanceType, MissingItem, MissingMaterialEntry, PagedList } from './typings';

export interface MissingMaterialParams {
  event_id?: number;
  level?: AgeLevel;
  dance_type?: DanceType;
  missing_item?: MissingItem;
  deadline_state?: 'open' | 'closed';
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/materials/missing */
export async function listMissingMaterials(
  params: MissingMaterialParams,
): Promise<PagedList<MissingMaterialEntry>> {
  return request('/api/v1/admin/materials/missing', { method: 'GET', params });
}

/** POST /api/v1/admin/materials/missing/{registration_id}/contacted */
export async function markMaterialContacted(
  registrationId: number,
  note?: string,
): Promise<{ registration_id: number; contacted: boolean }> {
  return request(`/api/v1/admin/materials/missing/${registrationId}/contacted`, {
    method: 'POST',
    data: { note },
  });
}

/** 导出催收名单（含电话），export=true 文件流 */
export async function exportMissingMaterials(
  params: Omit<MissingMaterialParams, 'page' | 'page_size'>,
): Promise<void> {
  return downloadFile('/api/v1/admin/materials/missing', { ...params, export: true });
}

/** GET /api/v1/admin/events/{event_id}/registrations/export 导出选手名单 */
export async function exportRegistrations(
  eventId: number,
  params?: { include_canceled?: boolean; columns?: string },
): Promise<void> {
  return downloadFile(`/api/v1/admin/events/${eventId}/registrations/export`, params);
}
