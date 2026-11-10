import { request } from '@umijs/max';
import type {
  PagedList,
  RefundTicketBrief,
  RefundTicketDetail,
  RefundTicketStatus,
} from './typings';

export interface RefundTicketListParams {
  status?: RefundTicketStatus;
  event_id?: number;
  created_from?: string;
  created_to?: string;
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/refund-tickets */
export async function listRefundTickets(
  params: RefundTicketListParams,
): Promise<PagedList<RefundTicketBrief>> {
  return request('/api/v1/admin/refund-tickets', { method: 'GET', params });
}

/** GET /api/v1/admin/refund-tickets/{ticket_id}（*待定*：详情接口，含政策阶段与建议金额） */
export async function getRefundTicket(ticketId: number): Promise<RefundTicketDetail> {
  return request(`/api/v1/admin/refund-tickets/${ticketId}`, { method: 'GET' });
}

/** POST /api/v1/admin/refund-tickets/{ticket_id}/approve */
export async function approveRefundTicket(
  ticketId: number,
  data: { amount: number; adjust_reason?: string },
): Promise<{ ticket_no: string; status: string; refund_id: string; order_status: string }> {
  return request(`/api/v1/admin/refund-tickets/${ticketId}/approve`, { method: 'POST', data });
}

/** POST /api/v1/admin/refund-tickets/{ticket_id}/reject */
export async function rejectRefundTicket(
  ticketId: number,
  data: { reject_reason: string },
): Promise<{ ticket_no: string; status: string; order_status: string }> {
  return request(`/api/v1/admin/refund-tickets/${ticketId}/reject`, { method: 'POST', data });
}
