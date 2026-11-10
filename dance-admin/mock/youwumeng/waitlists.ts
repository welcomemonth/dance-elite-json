/**
 * 候补管理 mock：队列列表（按提交时间排序带序号）/通知/移出/转报名/导出。
 */
import type { Request, Response } from 'express';
import { orders, releasedQuotas, waitlists } from './_db';
import { fail, maskIdCard, ok, paginate, toInt } from './_utils';

const isCs = (req: Request) => String(req.headers.authorization ?? '').includes('-cs-');

export default {
  'GET /api/v1/admin/waitlists': (req: Request, res: Response) => {
    if (req.query.export === 'true') {
      res.setHeader('Content-Type', 'text/csv; charset=utf-8');
      res.setHeader('Content-Disposition', 'attachment; filename="waitlists.csv"');
      return res.send('﻿序号,姓名,电话,状态\n1,示例,13800000000,waiting\n');
    }
    let list = [...waitlists];
    const q = req.query;
    if (q.event_id) list = list.filter((w) => w.event_id === toInt(q.event_id));
    if (q.level) list = list.filter((w) => w.level === q.level);
    if (q.dance_type) list = list.filter((w) => w.dance_type === q.dance_type);
    if (q.status) list = list.filter((w) => w.status === q.status);
    // 队列内按提交时间先后排序并编号（通知顺序依据）
    list.sort((a, b) => (a.created_at > b.created_at ? 1 : -1));
    const withPosition = list.map((w, i) => ({
      waitlist_id: w.waitlist_id,
      position_no: i + 1,
      event_id: w.event_id,
      event_name: w.event_name,
      level: w.level,
      dance_type: w.dance_type,
      player_name: w.player_name,
      id_card_masked: w.id_card_no ? maskIdCard(w.id_card_no) : null,
      institution: w.institution,
      phone: w.phone,
      status: w.status,
      created_at: w.created_at,
      notified_at: w.notified_at,
      note: w.note,
    }));
    const key = `${q.event_id ?? '2'}:${q.level ?? 'U11'}:${q.dance_type ?? 'classical'}`;
    return ok(res, {
      ...paginate(req, withPosition),
      released_quota: releasedQuotas[key] ?? 0,
    });
  },

  'POST /api/v1/admin/waitlists/:id/notify': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const entry = waitlists.find((w) => w.waitlist_id === toInt(req.params.id));
    if (!entry) return fail(res, 40401, '候补记录不存在');
    if (entry.status !== 'waiting') return fail(res, 41013, '状态不允许操作');
    const note = String(req.body?.note ?? '').trim();
    if (!note) return fail(res, 40001, '请填写通知结果');
    entry.status = 'notified';
    entry.note = note;
    entry.notified_at = new Date().toISOString();
    return ok(res, { waitlist_id: entry.waitlist_id, status: 'notified' });
  },

  'POST /api/v1/admin/waitlists/:id/remove': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const entry = waitlists.find((w) => w.waitlist_id === toInt(req.params.id));
    if (!entry) return fail(res, 40401, '候补记录不存在');
    if (entry.status === 'converted') return fail(res, 41013, '已转报名不可移出');
    const reason = String(req.body?.reason ?? '').trim();
    if (!reason) return fail(res, 40001, '移出原因必填');
    entry.status = 'removed';
    entry.note = reason;
    return ok(res, { waitlist_id: entry.waitlist_id, status: 'removed' });
  },

  'POST /api/v1/admin/waitlists/:id/convert': (req: Request, res: Response) => {
    if (isCs(req)) return fail(res, 40301, '无权限');
    const entry = waitlists.find((w) => w.waitlist_id === toInt(req.params.id));
    if (!entry) return fail(res, 40401, '候补记录不存在');
    if (entry.status !== 'waiting' && entry.status !== 'notified') {
      return fail(res, 41013, '状态不允许操作');
    }
    const orderNo = String(req.body?.order_no ?? '').trim();
    const order = orders.find((o) => o.order_no === orderNo);
    if (!order) return fail(res, 40001, '订单不存在');
    entry.status = 'converted';
    entry.order_no = orderNo;
    return ok(res, { waitlist_id: entry.waitlist_id, status: 'converted' });
  },
};
