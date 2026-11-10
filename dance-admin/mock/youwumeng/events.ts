/**
 * 赛事管理 mock：列表/详情/创建/编辑/发布/下架/名额/关闭组别/音频审核链接/名单导出。
 * 覆盖情况：全状态赛事、满员/关闭组别、404、名额只能调大、cs 无写权限。
 */
import type { Request, Response } from 'express';
import { events } from './_db';
import type { MockEvent } from './_db';
import { fail, includesKeyword, ok, paginate, randomToken, toInt } from './_utils';

const canWrite = (req: Request) => !String(req.headers.authorization ?? '').includes('-cs-');

export default {
  'GET /api/v1/admin/events': (req: Request, res: Response) => {
    let list = events;
    const { status, level, keyword } = req.query;
    if (status) list = list.filter((e) => e.status === status);
    if (level) list = list.filter((e) => e.level === level);
    if (keyword) list = list.filter((e) => includesKeyword(keyword, [e.name, e.city]));
    return ok(res, paginate(req, list));
  },

  'GET /api/v1/admin/events/:id': (req: Request, res: Response) => {
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    return ok(res, event);
  },

  'POST /api/v1/admin/events': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const body = req.body ?? {};
    const nodes = body.timeline ?? {};
    const seq = ['reg_start_at', 'reg_end_at', 'lottery_at', 'material_check_at', 'start_at'];
    for (let i = 1; i < seq.length; i += 1) {
      if (!(nodes[seq[i - 1]] < nodes[seq[i]])) {
        return fail(res, 40001, '时间线顺序错误');
      }
    }
    if (!Array.isArray(body.groups) || body.groups.length === 0) {
      return fail(res, 40001, '至少配置 1 个组别');
    }
    const event_id = Math.max(...events.map((e) => e.event_id)) + 1;
    const event: MockEvent = {
      event_id,
      name: body.name,
      season_id: body.season_id,
      level: body.level,
      city: body.city ?? '',
      venue: body.venue ?? '',
      event_date: body.event_date,
      fee: body.fee,
      cover_url: '',
      status: body.publish ? 'registering' : 'draft',
      timeline: body.timeline,
      groups: body.groups.map((g: { level: string; dance_type: string; quota: number }, i: number) => ({
        group_id: event_id * 10 + i,
        level: g.level,
        dance_type: g.dance_type,
        quota: g.quota,
        registered: 0,
        enabled: true,
      })),
      refund_policy: body.refund_policy ?? '',
      work_duration_limit: body.work_duration_limit ?? '',
      process_desc: body.process_desc ?? '',
      honor_desc: body.honor_desc ?? '',
      exclude_from_prize: body.exclude_from_prize ?? false,
      audio_review_token: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    events.unshift(event);
    return ok(res, { event_id, status: event.status });
  },

  'PUT /api/v1/admin/events/:id': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    if (event.status === 'finished') return fail(res, 41012, '赛事状态不允许编辑');
    const body = req.body ?? {};
    const published = !['draft', 'unpublished'].includes(event.status);
    if (published) {
      // 已发布：时间线与级别不可改，名额只能调大
      if (body.timeline && JSON.stringify(body.timeline) !== JSON.stringify(event.timeline)) {
        return fail(res, 40001, '已发布赛事不可修改时间线，如需变更请下架后新建');
      }
      if (body.level && body.level !== event.level) {
        return fail(res, 40001, '已发布赛事不可修改级别');
      }
    }
    Object.assign(event, {
      name: body.name ?? event.name,
      city: body.city ?? event.city,
      venue: body.venue ?? event.venue,
      event_date: body.event_date ?? event.event_date,
      fee: body.fee ?? event.fee,
      timeline: body.timeline ?? event.timeline,
      refund_policy: body.refund_policy ?? event.refund_policy,
      work_duration_limit: body.work_duration_limit ?? event.work_duration_limit,
      process_desc: body.process_desc ?? event.process_desc,
      honor_desc: body.honor_desc ?? event.honor_desc,
      exclude_from_prize: body.exclude_from_prize ?? event.exclude_from_prize,
      updated_at: new Date().toISOString(),
    });
    if (Array.isArray(body.groups)) {
      for (const g of body.groups) {
        const target = event.groups.find((x) => x.group_id === g.group_id);
        if (!target) continue;
        if (g.quota !== undefined && g.quota < target.registered) {
          return fail(res, 40001, `名额不得小于当前已支付人数（${target.registered}）`);
        }
        if (g.quota !== undefined) target.quota = g.quota;
      }
    }
    return ok(res, { event_id: event.event_id, status: event.status });
  },

  'POST /api/v1/admin/events/:id/publish': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    if (event.groups.length === 0) return fail(res, 40001, '发布校验未通过：无组别');
    event.status = 'registering';
    event.updated_at = new Date().toISOString();
    return ok(res, { event_id: event.event_id, status: event.status });
  },

  'POST /api/v1/admin/events/:id/offline': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    event.status = 'offline';
    event.updated_at = new Date().toISOString();
    return ok(res, { event_id: event.event_id, status: event.status });
  },

  'PUT /api/v1/admin/events/:id/groups/:gid/quota': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    const group = event?.groups.find((g) => g.group_id === toInt(req.params.gid));
    if (!event || !group) return fail(res, 40401, '赛事或组别不存在');
    const quota = toInt(req.body?.quota);
    if (quota <= group.registered) {
      return fail(res, 40001, `名额不得小于当前已支付人数（${group.registered}）`);
    }
    group.quota = quota;
    return ok(res, { group_id: group.group_id, quota, registered: group.registered, remaining: quota - group.registered });
  },

  'POST /api/v1/admin/events/:id/groups/:gid/close': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    const group = event?.groups.find((g) => g.group_id === toInt(req.params.gid));
    if (!event || !group) return fail(res, 40401, '赛事或组别不存在');
    if (event.status === 'finished') return fail(res, 40001, '已结束赛事不可操作');
    group.enabled = false;
    return ok(res, { group_id: group.group_id, enabled: false });
  },

  'POST /api/v1/admin/events/:id/groups/:gid/reopen': (req: Request, res: Response) => {
    if (!canWrite(req)) return fail(res, 40301, '无权限');
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    const group = event?.groups.find((g) => g.group_id === toInt(req.params.gid));
    if (!event || !group) return fail(res, 40401, '赛事或组别不存在');
    group.enabled = true;
    return ok(res, { group_id: group.group_id, enabled: true });
  },

  'POST /api/v1/admin/events/:id/audio-review-token': (req: Request, res: Response) => {
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    event.audio_review_token = randomToken();
    return ok(res, {
      url: `${req.protocol}://${req.get('host')}/audio-review/${event.audio_review_token}`,
      token: event.audio_review_token,
    });
  },

  'GET /api/v1/admin/events/:id/registrations/export': (req: Request, res: Response) => {
    if (String(req.headers.authorization ?? '').includes('-cs-')) {
      return fail(res, 40301, '客服不可导出');
    }
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    res.setHeader('Content-Type', 'text/csv; charset=utf-8');
    res.setHeader('Content-Disposition', `attachment; filename="registrations-${event.event_id}.csv"`);
    res.send(`﻿订单号,姓名,组别,舞种\nMOCK,示例,U11,classical\n`);
  },
};
