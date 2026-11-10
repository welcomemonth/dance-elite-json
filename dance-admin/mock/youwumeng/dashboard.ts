/**
 * 数据看板 mock：指标卡/待办/赛事报名进度/近 30 日趋势。
 */
import type { Request, Response } from 'express';
import { csTickets, events, orders, refundTickets, trend, waitlists } from './_db';
import { ok } from './_utils';

export default {
  'GET /api/v1/admin/dashboard': (_req: Request, res: Response) => {
    const paidOrders = orders.filter((o) => o.status === 'paid' || o.status === 'refunding');
    const today = new Date().toISOString().slice(0, 10);
    const todayOrders = paidOrders.filter((o) => (o.paid_at ?? '').startsWith(today));
    const activeEvents = events.filter((e) => ['registering', 'reg_closed', 'ongoing'].includes(e.status));
    return ok(res, {
      metrics: {
        today_registrations: todayOrders.length,
        today_revenue: todayOrders.reduce((s, o) => s + o.amount, 0),
        total_registrations: paidOrders.length,
        total_revenue: paidOrders.reduce((s, o) => s + o.amount, 0),
        pending_refund_tickets: refundTickets.filter((t) => t.status === 'pending' || t.status === 'refund_failed').length,
        waitlist_total: waitlists.filter((w) => w.status === 'waiting').length,
        incomplete_materials: 41,
        active_events: activeEvents.length,
      },
      todos: {
        pending_refunds: refundTickets.filter((t) => t.status === 'pending').length,
        new_cs_tickets: csTickets.filter((t) => t.status === 'pending').length,
        closing_events: events
          .filter((e) => e.status === 'registering')
          .map((e) => ({
            event_id: e.event_id,
            event_name: e.name,
            days_left: Math.max(0, Math.ceil((new Date(e.timeline.reg_end_at).getTime() - Date.now()) / 86400000)),
          }))
          .filter((e) => e.days_left <= 7),
      },
      event_progress: events
        .filter((e) => e.status === 'registering')
        .flatMap((e) => e.groups.map((g) => ({
          event_id: e.event_id,
          event_name: e.name,
          level: `${g.level}·${g.dance_type === 'classical' ? '古典舞' : '民族民间舞'}`,
          dance_type: g.dance_type,
          registered: g.registered,
          quota: g.quota,
          waitlist_count: waitlists.filter((w) => w.event_id === e.event_id && w.level === g.level && w.dance_type === g.dance_type && w.status === 'waiting').length,
        }))),
      trend,
    });
  },
};
