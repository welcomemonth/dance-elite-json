import { request } from '@umijs/max';
import type { CsTicket, CsTicketStatus, CsTicketType, PagedList } from './typings';

export interface CsTicketListParams {
  status?: CsTicketStatus;
  type?: CsTicketType;
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/cs-tickets */
export async function listCsTickets(params: CsTicketListParams): Promise<PagedList<CsTicket>> {
  return request('/api/v1/admin/cs-tickets', { method: 'GET', params });
}

export interface CsTicketInput {
  type: CsTicketType;
  user_name?: string;
  phone: string;
  content: string;
  related_order_no?: string;
}

/** POST /api/v1/admin/cs-tickets */
export async function createCsTicket(data: CsTicketInput): Promise<{ ticket_id: number; status: string }> {
  return request('/api/v1/admin/cs-tickets', { method: 'POST', data });
}

/** PUT /api/v1/admin/cs-tickets/{ticket_id}（流转：状态 + 跟进备注） */
export async function updateCsTicket(
  ticketId: number,
  data: { status: CsTicketStatus; remark?: string },
): Promise<{ ticket_id: number; status: string }> {
  return request(`/api/v1/admin/cs-tickets/${ticketId}`, { method: 'PUT', data });
}
