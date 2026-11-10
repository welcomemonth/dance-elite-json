import { request } from '@umijs/max';
import { downloadFile } from './download';
import type {
  AgeLevel,
  DanceType,
  OrderBrief,
  OrderDetail,
  OrderSource,
  OrderStatus,
  PagedList,
} from './typings';

export interface OrderListParams {
  event_id?: number;
  level?: AgeLevel;
  dance_type?: DanceType;
  status?: OrderStatus;
  /** true 时含已取消（对账留档） */
  include_canceled?: boolean;
  source?: OrderSource;
  paid_from?: string;
  paid_to?: string;
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/orders */
export async function listOrders(params: OrderListParams): Promise<PagedList<OrderBrief>> {
  return request('/api/v1/admin/orders', { method: 'GET', params });
}

/** GET /api/v1/admin/orders/{order_no} */
export async function getOrder(orderNo: string): Promise<OrderDetail> {
  return request(`/api/v1/admin/orders/${orderNo}`, { method: 'GET' });
}

/** POST /api/v1/admin/orders/{order_no}/reveal-phone（需明文查看权限位） */
export async function revealPhone(orderNo: string): Promise<{ contact_phone: string }> {
  return request(`/api/v1/admin/orders/${orderNo}/reveal-phone`, { method: 'POST' });
}

/** GET /api/v1/admin/orders?export=true 导出 csv 文件流 */
export async function exportOrders(params: Omit<OrderListParams, 'page' | 'page_size'>): Promise<void> {
  return downloadFile('/api/v1/admin/orders', { ...params, export: true });
}
