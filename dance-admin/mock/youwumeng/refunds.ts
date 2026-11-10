/**
 * 退款工单 mock：列表/详情/同意/驳回。
 * 覆盖情况：全状态工单；903 号工单同意时模拟微信支付退款 API 失败（41014 → refund_failed）；
 * 非待处理工单操作返回 41013；cs 无处理权限。
 */
import type { Request, Response } from 'express';
import { events, orders, refundTickets } from './_db';
import { fail, includesKeyword, ok, paginate, toInt } from './_utils';

const isCs = (req: Request) => String(req.headers.authorization ?? '').includes('-cs-');

export default {
  'GET /api/v1/admin/refund-tickets': (req: Request, res: Response) => {
    let list = refundTickets;
    const q = req.query;
    if (q.status) list = list.filter((t) => t.status === q.status);
    if (q.event_id) list = list.filter((t) => t.event_id === toInt(q.event_id));
    if (q.created_from) list = list.filter((t) => t.created_at >= String(q.created_from));
    if (q.created_to) list = list.filter((t) => t.created_at <= String(q.created_to));
    if (q.keyword) {
      list = list.filter((t) => includesKeyword(q.keyword, [t.ticket_no, t.order_no, t.player_name]));
    }
    list = [...list].sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    return ok(res, paginate(req, list.map((t) => ({
      ticket_id: t.ticket_id,
      ticket_no: t.ticket_no,
      order_no: t.order_no,
      event_id: t.event_id,
      event_name: t.event_name,
      player_name: t.player_name,
      reason: t.reason,
      applicant: t.applicant,
      status: t.status,
      handler_name: t.handler_name,
      created_at: t.created_at,
      handled_at: t.handled_at,
    }))));
  },

  'GET /api/v1/admin/refund-tickets/:id': (req: Request, res: Response) => {
    const ticket = refundTickets.find((t) => t.ticket_id === toInt(req.params.id));
    if (!ticket) return fail(res, 40401, '工单不存在');
    const order = orders.find((o) => o.order_no === ticket.order_no);
    const event = events.find((e) => e.event_id === ticket.event_id);
    const lotteryAt = event?.timeline.lottery_at ?? '';
    const stage = new Date(lotteryAt).getTime() > Date.now() ? 'lottery_before' : 'lottery_after';
    const paidAmount = order?.amount ?? 0;
    return ok(res, {
      ...ticket,
      content: ticket.content,
      order: order ? {
        order_no: order.order_no,
        transaction_id: order.transaction_id,
        event_id: order.event_id,
        event_name: order.event_name,
        level: order.level,
        dance_type: order.dance_type,
        player_name: order.player_name,
        id_card_masked: null,
        amount: order.amount,
        status: order.status,
        source: order.source,
        paid_at: order.paid_at,
        created_at: order.created_at,
      } : null,
      policy_stage: stage,
      suggest_amount: stage === 'lottery_before' ? paidAmount : Math.floor(paidAmount / 2),
      paid_amount: paidAmount,
      history: [
        { action: '客服登记', operator: ticket.applicant, created_at: ticket.created_at, detail: ticket.content },
        ...(ticket.handled_at ? [{ action: `处理：${ticket.status}`, operator: ticket.handler_name ?? '', created_at: ticket.handled_at, detail: ticket.reject_reason ?? `退款金额 ${ticket.refund_amount ?? 0} 分` }] : []),
      ],
    });
  },

  'POST /api/v1/admin/refund-tickets/:id/approve': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const ticket = refundTickets.find((t) => t.ticket_id === toInt(req.params.id));
    if (!ticket) return fail(res, 40401, '工单不存在');
    if (ticket.status !== 'pending' && ticket.status !== 'refund_failed') {
      return fail(res, 41013, '工单状态非待处理');
    }
    const amount = toInt(req.body?.amount);
    if (!amount || amount <= 0) return fail(res, 40001, '退款金额非法');
    // 演示失败场景：903 号工单模拟微信支付退款 API 调用失败
    if (ticket.ticket_id === 903) {
      ticket.status = 'refund_failed';
      ticket.handled_at = new Date().toISOString();
      ticket.handler_name = '当前管理员';
      return fail(res, 41014, '微信支付退款 API 调用失败（余额不足/单号异常），工单已置为退款失败，可修改后重试');
    }
    const order = orders.find((o) => o.order_no === ticket.order_no);
    if (order) order.status = 'refunded';
    ticket.status = 'approved';
    ticket.refund_amount = amount;
    ticket.handler_name = '当前管理员';
    ticket.handled_at = new Date().toISOString();
    return ok(res, {
      ticket_no: ticket.ticket_no,
      status: 'approved',
      refund_id: `500000${Date.now()}`,
      order_status: 'refunded',
    });
  },

  'POST /api/v1/admin/refund-tickets/:id/reject': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const ticket = refundTickets.find((t) => t.ticket_id === toInt(req.params.id));
    if (!ticket) return fail(res, 40401, '工单不存在');
    if (ticket.status !== 'pending' && ticket.status !== 'refund_failed') {
      return fail(res, 41013, '工单状态非待处理');
    }
    const reason = String(req.body?.reject_reason ?? '').trim();
    if (!reason) return fail(res, 40001, '驳回原因必填');
    const order = orders.find((o) => o.order_no === ticket.order_no);
    if (order) order.status = 'paid';
    ticket.status = 'rejected';
    ticket.reject_reason = reason;
    ticket.handler_name = '当前管理员';
    ticket.handled_at = new Date().toISOString();
    return ok(res, { ticket_no: ticket.ticket_no, status: 'rejected', order_status: 'paid' });
  },
};
