import { request } from '@umijs/max';
import type { AdminAccount, AdminRole, LogEntry, PagedList } from './typings';

/** GET /api/v1/admin/accounts（super_admin 专属） */
export async function listAccounts(params?: {
  keyword?: string;
  page?: number;
  page_size?: number;
}): Promise<PagedList<AdminAccount>> {
  return request('/api/v1/admin/accounts', { method: 'GET', params });
}

export interface AccountInput {
  username: string;
  password: string;
  role: AdminRole;
  name: string;
}

/** POST /api/v1/admin/accounts */
export async function createAccount(
  data: AccountInput,
): Promise<{ admin_id: number; username: string; role: string }> {
  return request('/api/v1/admin/accounts', { method: 'POST', data });
}

/** PUT /api/v1/admin/accounts/{admin_id}（停用/重置密码/调整角色） */
export async function updateAccount(
  adminId: number,
  data: { status?: 'active' | 'disabled'; password?: string; role?: AdminRole },
): Promise<{ admin_id: number }> {
  return request(`/api/v1/admin/accounts/${adminId}`, { method: 'PUT', data });
}

export interface LogListParams {
  admin_id?: number;
  action?: string;
  target_type?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/logs */
export async function listLogs(params: LogListParams): Promise<PagedList<LogEntry>> {
  return request('/api/v1/admin/logs', { method: 'GET', params });
}
