import { request } from '@umijs/max';
import type { Dashboard } from './typings';

/** GET /api/v1/admin/dashboard */
export async function getDashboard(): Promise<Dashboard> {
  return request('/api/v1/admin/dashboard', { method: 'GET' });
}
