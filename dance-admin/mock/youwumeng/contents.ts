/**
 * 内容管理 mock：键值内容读写（积分规则说明/赛事 FAQ/公众号/公告）。
 */
import type { Request, Response } from 'express';
import { contents } from './_db';
import { fail, ok } from './_utils';

export default {
  'GET /api/v1/admin/contents/:key': (req: Request, res: Response) => {
    const entry = contents[req.params.key as keyof typeof contents];
    if (!entry) return fail(res, 40401, '内容键未注册');
    return ok(res, { key: req.params.key, ...entry });
  },

  'PUT /api/v1/admin/contents/:key': (req: Request, res: Response) => {
    const entry = contents[req.params.key as keyof typeof contents];
    if (!entry) return fail(res, 40401, '内容键未注册');
    if (String(req.headers.authorization ?? '').includes('-cs-')) {
      return fail(res, 40301, '客服不可修改内容');
    }
    entry.value = req.body?.value ?? entry.value;
    entry.updated_at = new Date().toISOString();
    return ok(res, { key: req.params.key, ...entry });
  },
};
