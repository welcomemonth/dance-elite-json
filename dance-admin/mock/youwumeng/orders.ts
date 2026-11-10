/**
 * 订单中心 mock：列表（默认不含已取消）/详情/明文查看/导出。
 * 覆盖情况：全状态订单、miniapp/manual 来源、无证件号（null 脱敏）、
 * cs 无明文查看权限（40301）、导出文件流。
 */
import type { Request, Response } from 'express';
import { admins, events, orders, refundTickets } from './_db';
import { fail, includesKeyword, maskIdCard, maskPhone, ok, paginate, toInt } from './_utils';

const isCs = (req: Request) => String(req.headers.authorization ?? '').includes('-cs-');

function orderToBrief(o: (typeof orders)[number]) {
  return {
    order_no: o.order_no,
    transaction_id: o.transaction_id,
    event_id: o.event_id,
    event_name: o.event_name,
    level: o.level,
    dance_type: o.dance_type,
    player_name: o.player_name,
    id_card_masked: o.id_card_no ? maskIdCard(o.id_card_no) : null,
    amount: o.amount,
    status: o.status,
    source: o.source,
    paid_at: o.paid_at,
    created_at: o.created_at,
  };
}

export default {
  'GET /api/v1/admin/orders': (req: Request, res: Response) => {
    if (req.query.export === 'true') {
      res.setHeader('Content-Type', 'text/csv; charset=utf-8');
      res.setHeader('Content-Disposition', 'attachment; filename="orders.csv"');
      return res.send('﻿订单号,姓名,金额,状态\nMOCK,示例,98000,paid\n');
    }
    let list = orders;
    const q = req.query;
    if (q.event_id) list = list.filter((o) => o.event_id === toInt(q.event_id));
    if (q.level) list = list.filter((o) => o.level === q.level);
    if (q.dance_type) list = list.filter((o) => o.dance_type === q.dance_type);
    if (q.source) list = list.filter((o) => o.source === q.source);
    if (q.status) {
      list = list.filter((o) => o.status === q.status);
    } else if (q.include_canceled !== 'true') {
      list = list.filter((o) => o.status !== 'canceled');
    }
    if (q.paid_from) list = list.filter((o) => o.paid_at && o.paid_at >= String(q.paid_from));
    if (q.paid_to) list = list.filter((o) => o.paid_at && o.paid_at <= String(q.paid_to));
    if (q.keyword) {
      list = list.filter((o) => includesKeyword(q.keyword, [o.player_name, o.order_no, o.transaction_id]));
    }
    list = [...list].sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    return ok(res, paginate(req, list.map(orderToBrief)));
  },

  'GET /api/v1/admin/orders/:orderNo': (req: Request, res: Response) => {
    const order = orders.find((o) => o.order_no === req.params.orderNo);
    if (!order) return fail(res, 40401, '订单不存在');
    const event = events.find((e) => e.event_id === order.event_id);
    const ticket = refundTickets.find((t) => t.order_no === order.order_no);
    const readonly = event ? new Date(event.timeline.material_check_at).getTime() < Date.now() : false;
    return ok(res, {
      order: { ...orderToBrief(order), cancel_reason: order.cancel_reason, registration_id: order.registration_id },
      registration: {
        registration_id: order.registration_id,
        event_name: order.event_name,
        level: order.level,
        dance_type: order.dance_type,
        player_name: order.player_name,
        id_card_masked: order.id_card_no ? maskIdCard(order.id_card_no) : null,
        id_type: order.id_type,
        institution: order.institution,
        contact_phone: maskPhone(order.contact_phone),
        material_deadline: event?.timeline.material_check_at ?? '',
      },
      payments: [
        {
          type: order.status === 'refunded' ? 'refund' : 'pay',
          prepay_id: order.status === 'canceled' ? '' : 'wx201410272009395522657a690389285100',
          transaction_id: order.transaction_id,
          refund_id: order.status === 'refunded' ? '5000001234202610211234567890' : '',
          amount: order.amount,
          status: order.status === 'canceled' ? 'CLOSED' : 'SUCCESS',
          notified_at: order.paid_at ?? order.created_at,
        },
      ],
      refund_ticket: ticket
        ? { ticket_id: ticket.ticket_id, ticket_no: ticket.ticket_no, status: ticket.status }
        : null,
      materials: {
        photo_url: order.photo_url,
        audio_url: order.audio_url,
        audio_name: order.audio_url ? `${order.program_name ?? '伴奏'}.mp3` : null,
        program_name: order.program_name,
        contact_phone: order.contact_phone,
        updated_at: order.materials_updated_at,
        readonly,
      },
      operation_logs: [
        { action: '创建订单', from_status: null, to_status: 'created', operator: order.source === 'manual' ? '管理员录入' : '系统', created_at: order.created_at },
        ...(order.status === 'canceled'
          ? [{ action: '取消订单', from_status: 'created', to_status: 'canceled', operator: '系统', created_at: order.created_at }]
          : [{ action: order.source === 'manual' ? '后台人工确认' : '支付成功', from_status: 'created', to_status: 'paid', operator: order.source === 'manual' ? '管理员录入' : '微信支付', created_at: order.paid_at ?? order.created_at }]),
        ...(order.status === 'refunding' || order.status === 'refunded'
          ? [{ action: '申请退款', from_status: 'paid', to_status: 'refunding', operator: '客服登记', created_at: order.created_at }]
          : []),
        ...(order.status === 'refunded'
          ? [{ action: '退款完成', from_status: 'refunding', to_status: 'refunded', operator: '管理员', created_at: order.created_at }]
          : []),
      ],
      remarks: order.remarks,
    });
  },

  'POST /api/v1/admin/orders/:orderNo/reveal-phone': (req: Request, res: Response) => {
    const order = orders.find((o) => o.order_no === req.params.orderNo);
    if (!order) return fail(res, 40401, '订单不存在');
    // 校验 mock token 中的管理员是否持有 reveal_phone 权限位（cs 一律拒绝 → 40301）
    const token = String(req.headers.authorization ?? '');
    const admin = admins.find((a) => token.endsWith(`-${a.admin_id}`));
    if (!admin?.permissions.includes('reveal_phone')) {
      return fail(res, 40301, '无明文查看权限（需单独授权，操作将留痕）');
    }
    return ok(res, { contact_phone: order.contact_phone });
  },

  'POST /api/v1/admin/manual-registrations': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const body = req.body ?? {};
    const event = events.find((e) => e.event_id === toInt(body.event_id));
    if (!event) return fail(res, 40401, '赛事不存在');
    const group = event.groups.find((g) => g.group_id === toInt(body.group_id));
    if (!group) return fail(res, 40401, '组别不存在');
    if (event.status !== 'registering' && event.status !== 'unpublished') {
      return fail(res, 41006, '赛事不在可报名状态');
    }
    if (group.registered >= group.quota || !group.enabled) {
      return fail(res, 41005, '名额已满或组别已关闭');
    }
    if (!body.player_name || !body.id_type || !body.id_card_no || !body.contact_phone) {
      return fail(res, 40001, '参数缺失');
    }
    group.registered += 1;
    const registration_id = 6000 + orders.length;
    const orderNo = `20261${String(1010100000 + orders.length * 137)}`;
    orders.unshift({
      order_no: orderNo,
      transaction_id: '',
      event_id: event.event_id,
      event_name: event.name,
      level: group.level,
      dance_type: group.dance_type,
      player_id: 0,
      player_name: body.player_name,
      id_card_no: body.id_card_no,
      id_type: body.id_type,
      amount: body.charge_type === 'free' ? 0 : toInt(body.amount),
      status: 'paid',
      source: 'manual',
      institution: body.institution ?? '',
      contact_phone: body.contact_phone,
      cancel_reason: null,
      registration_id,
      program_name: null,
      audio_url: null,
      photo_url: null,
      paid_at: new Date().toISOString(),
      created_at: new Date().toISOString(),
      materials_updated_at: null,
      remarks: body.remark ? [{ id: 1, content: body.remark, operator: '管理员录入', created_at: new Date().toISOString() }] : [],
    });
    return ok(res, { registration_id, order_no: orderNo });
  },
};
