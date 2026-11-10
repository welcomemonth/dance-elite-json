/**
 * 客服工单 mock：登记/列表/流转。
 */
import type { Request, Response } from 'express';
import { csTickets } from './_db';
import { fail, includesKeyword, ok, paginate, toInt } from './_utils';

export default {
  'GET /api/v1/admin/cs-tickets': (req: Request, res: Response) => {
    let list = [...csTickets].sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    const q = req.query;
    if (q.status) list = list.filter((t) => t.status === q.status);
    if (q.type) list = list.filter((t) => t.type === q.type);
    if (q.keyword) list = list.filter((t) => includesKeyword(q.keyword, [t.user_name, t.phone, t.content]));
    return ok(res, paginate(req, list));
  },

  'POST /api/v1/admin/cs-tickets': (req: Request, res: Response) => {
    const body = req.body ?? {};
    if (!body.type || !body.content || !body.phone) {
      return fail(res, 40001, '类型、联系方式与诉求内容必填');
    }
    if (!['refund', 'no_id_card', 'waitlist', 'other'].includes(body.type)) {
      return fail(res, 40001, '工单类型非法');
    }
    const ticket_id = Math.max(...csTickets.map((t) => t.ticket_id)) + 1;
    csTickets.unshift({
      ticket_id,
      type: body.type,
      user_name: body.user_name ?? '',
      phone: body.phone,
      content: body.content,
      status: 'pending',
      created_by: '当前账号',
      related_order_no: body.related_order_no ?? null,
      remark: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    });
    return ok(res, { ticket_id, status: 'pending' });
  },

  'PUT /api/v1/admin/cs-tickets/:id': (req: Request, res: Response) => {
    const ticket = csTickets.find((t) => t.ticket_id === toInt(req.params.id));
    if (!ticket) return fail(res, 40401, '工单不存在');
    const { status, remark } = req.body ?? {};
    if (status && !['pending', 'processing', 'done'].includes(status)) {
      return fail(res, 41013, '流转状态不合法');
    }
    if (status) ticket.status = status;
    if (remark) ticket.remark = remark;
    ticket.updated_at = new Date().toISOString();
    return ok(res, { ticket_id: ticket.ticket_id, status: ticket.status });
  },
};
