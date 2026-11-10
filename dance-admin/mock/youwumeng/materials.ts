/**
 * 资料催收 mock：未传齐名单（按缺失项筛选）/标记已联系/导出。
 * 数据由订单资料字段推导，覆盖：各种缺失组合、已联系标记、截止后只读。
 */
import type { Request, Response } from 'express';
import { events, orders } from './_db';
import { ok, paginate, toInt } from './_utils';

const ITEM_KEYS = ['photo', 'audio', 'program_name', 'contact_phone'] as const;

type ItemKey = (typeof ITEM_KEYS)[number];

function missingOf(o: (typeof orders)[number]): ItemKey[] {
  const missing: ItemKey[] = [];
  if (!o.photo_url) missing.push('photo');
  if (!o.audio_url) missing.push('audio');
  if (!o.program_name) missing.push('program_name');
  if (!o.contact_phone) missing.push('contact_phone');
  return missing;
}

const contactNotes = new Map<number, string>();

function toEntry(o: (typeof orders)[number]) {
  const event = events.find((e) => e.event_id === o.event_id);
  const missing = missingOf(o);
  return {
    registration_id: o.registration_id,
    order_no: o.order_no,
    event_id: o.event_id,
    event_name: o.event_name,
    level: o.level,
    dance_type: o.dance_type,
    player_name: o.player_name,
    contact_phone: o.contact_phone,
    completed: 4 - missing.length,
    total: 4,
    missing_items: missing,
    material_deadline: event?.timeline.material_check_at ?? '',
    material_updated_at: o.materials_updated_at,
    contacted: contactNotes.has(o.registration_id),
    contact_note: contactNotes.get(o.registration_id) ?? null,
  };
}

export default {
  'GET /api/v1/admin/materials/missing': (req: Request, res: Response) => {
    if (req.query.export === 'true') {
      res.setHeader('Content-Type', 'text/csv; charset=utf-8');
      res.setHeader('Content-Disposition', 'attachment; filename="materials-missing.csv"');
      return res.send('﻿订单号,姓名,电话,缺失项\nMOCK,示例,13800000000,audio\n');
    }
    let list = orders.filter((o) => o.status === 'paid' && missingOf(o).length > 0);
    const q = req.query;
    if (q.event_id) list = list.filter((o) => o.event_id === toInt(q.event_id));
    if (q.level) list = list.filter((o) => o.level === q.level);
    if (q.dance_type) list = list.filter((o) => o.dance_type === q.dance_type);
    if (q.missing_item) list = list.filter((o) => missingOf(o).includes(q.missing_item as ItemKey));
    if (q.deadline_state) {
      list = list.filter((o) => {
        const event = events.find((e) => e.event_id === o.event_id);
        const closed = event ? new Date(event.timeline.material_check_at).getTime() < Date.now() : false;
        return q.deadline_state === 'closed' ? closed : !closed;
      });
    }
    return ok(res, paginate(req, list.map(toEntry)));
  },

  'POST /api/v1/admin/materials/missing/:id/contacted': (req: Request, res: Response) => {
    const registrationId = toInt(req.params.id);
    const order = orders.find((o) => o.registration_id === registrationId);
    if (!order) {
      res.status(200).json({ code: 40401, message: '报名不存在', data: null });
      return;
    }
    contactNotes.set(registrationId, String(req.body?.note ?? '已电话联系'));
    return ok(res, { registration_id: registrationId, contacted: true });
  },
};
