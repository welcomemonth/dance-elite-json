/**
 * 系统管理 mock：账号管理（super_admin 专属）/操作日志。
 */
import type { Request, Response } from 'express';
import { admins, logActions } from './_db';
import { fail, includesKeyword, ok, paginate, toInt } from './_utils';

const isSuper = (req: Request) => String(req.headers.authorization ?? '').includes('super_admin');

export default {
  'GET /api/v1/admin/accounts': (req: Request, res: Response) => {
    let list = admins;
    if (req.query.keyword) {
      list = list.filter((a) => includesKeyword(req.query.keyword, [a.username, a.name]));
    }
    return ok(res, paginate(req, list.map((a) => ({
      admin_id: a.admin_id,
      username: a.username,
      name: a.name,
      role: a.role,
      status: a.status,
      last_login_at: a.last_login_at,
      created_at: a.created_at,
    }))));
  },

  'POST /api/v1/admin/accounts': (req: Request, res: Response) => {
    if (!isSuper(req)) return fail(res, 40301, '仅超级管理员可管理账号');
    const body = req.body ?? {};
    if (!body.username || !body.password || !body.role || !body.name) {
      return fail(res, 40001, '参数缺失');
    }
    if (String(body.password).length < 8) return fail(res, 40001, '密码过弱（至少 8 位）');
    if (admins.some((a) => a.username === body.username)) {
      return fail(res, 40001, '用户名已存在');
    }
    const admin_id = Math.max(...admins.map((a) => a.admin_id)) + 1;
    admins.push({
      admin_id,
      username: body.username,
      password: body.password,
      name: body.name,
      role: body.role,
      status: 'active',
      permissions: [],
      last_login_at: '',
      created_at: new Date().toISOString(),
    });
    return ok(res, { admin_id, username: body.username, role: body.role });
  },

  'PUT /api/v1/admin/accounts/:id': (req: Request, res: Response) => {
    if (!isSuper(req)) return fail(res, 40301, '仅超级管理员可管理账号');
    const admin = admins.find((a) => a.admin_id === toInt(req.params.id));
    if (!admin) return fail(res, 40401, '账号不存在');
    const { status, password, role } = req.body ?? {};
    if (status) admin.status = status;
    if (role) admin.role = role;
    if (password) {
      if (String(password).length < 8) return fail(res, 40001, '密码过弱（至少 8 位）');
      admin.password = password;
    }
    return ok(res, { admin_id: admin.admin_id });
  },

  'GET /api/v1/admin/logs': (req: Request, res: Response) => {
    let list = [...logActions].sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    const q = req.query;
    if (q.admin_id) list = list.filter((l) => l.admin_name === admins.find((a) => a.admin_id === toInt(q.admin_id))?.name);
    if (q.action) list = list.filter((l) => l.action === q.action);
    if (q.target_type) list = list.filter((l) => l.target_type === q.target_type);
    if (q.created_from) list = list.filter((l) => l.created_at >= String(q.created_from));
    if (q.created_to) list = list.filter((l) => l.created_at <= String(q.created_to));
    return ok(res, paginate(req, list.map((l, i) => ({ log_id: 9000 - i, ...l }))));
  },
};
