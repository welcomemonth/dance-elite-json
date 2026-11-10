/**
 * 认证 mock：POST /api/v1/admin/auth/login
 * 测试账号：admin/Admin@12345（超级管理员）、ops/Ops@12345（管理员）、cs01/Cs@12345（客服）
 */
import type { Request, Response } from 'express';
import { admins } from './_db';
import { fail, ok } from './_utils';

export default {
  'POST /api/v1/admin/auth/login': (req: Request, res: Response) => {
    const { username, password } = req.body ?? {};
    if (!username || !password) {
      return fail(res, 40001, '账号或密码为空');
    }
    const admin = admins.find((a) => a.username === username);
    if (!admin || admin.password !== password) {
      return fail(res, 40102, '账号或密码错误');
    }
    if (admin.status === 'disabled') {
      return fail(res, 40302, '账号已停用');
    }
    admin.last_login_at = new Date().toISOString();
    return ok(res, {
      token: `mock-token-${admin.role}-${admin.admin_id}`,
      expires_in: 7200,
      admin: {
        id: admin.admin_id,
        username: admin.username,
        role: admin.role,
        name: admin.name,
        permissions: admin.permissions,
      },
    });
  },
};
