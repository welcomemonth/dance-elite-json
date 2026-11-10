import { request } from '@umijs/max';
import { downloadFile } from './download';
import type {
  AgeLevel,
  DanceType,
  ManualRegistrationInput,
  WaitlistList,
  WaitlistStatus,
} from './typings';

export interface WaitlistListParams {
  event_id?: number;
  level?: AgeLevel;
  dance_type?: DanceType;
  status?: WaitlistStatus;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/waitlists */
export async function listWaitlists(params: WaitlistListParams): Promise<WaitlistList> {
  return request('/api/v1/admin/waitlists', { method: 'GET', params });
}

/** POST /api/v1/admin/waitlists/{waitlist_id}/notify */
export async function notifyWaitlist(
  waitlistId: number,
  note: string,
): Promise<{ waitlist_id: number; status: string }> {
  return request(`/api/v1/admin/waitlists/${waitlistId}/notify`, { method: 'POST', data: { note } });
}

/** POST /api/v1/admin/waitlists/{waitlist_id}/remove */
export async function removeWaitlist(
  waitlistId: number,
  reason: string,
): Promise<{ waitlist_id: number; status: string }> {
  return request(`/api/v1/admin/waitlists/${waitlistId}/remove`, { method: 'POST', data: { reason } });
}

/** POST /api/v1/admin/waitlists/{waitlist_id}/convert */
export async function convertWaitlist(
  waitlistId: number,
  orderNo: string,
): Promise<{ waitlist_id: number; status: string }> {
  return request(`/api/v1/admin/waitlists/${waitlistId}/convert`, {
    method: 'POST',
    data: { order_no: orderNo },
  });
}

/** 导出当前队列（含电话），*待定*：export=true 文件流 */
export async function exportWaitlists(params: Omit<WaitlistListParams, 'page' | 'page_size'>): Promise<void> {
  return downloadFile('/api/v1/admin/waitlists', { ...params, export: true });
}

/** POST /api/v1/admin/manual-registrations */
export async function createManualRegistration(
  data: ManualRegistrationInput,
): Promise<{ registration_id: number; order_no: string }> {
  return request('/api/v1/admin/manual-registrations', { method: 'POST', data });
}
