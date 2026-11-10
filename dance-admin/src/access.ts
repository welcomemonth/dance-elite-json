import type { Admin } from '@/services/youwumeng/typings';

/**
 * @see https://umijs.org/docs/max/access
 * 权限矩阵见 docs/后台管理/优舞盟后台管理页面与功能设计.md 0.2 节。
 */
export default function access(initialState: { currentUser?: Admin } | undefined) {
  const { currentUser } = initialState ?? {};
  const role = currentUser?.role;
  const isAdmin = role === 'admin' || role === 'super_admin';
  return {
    /** 管理员及以上（客服只读） */
    canAdmin: isAdmin,
    /** 超级管理员（账号管理等） */
    isSuperAdmin: role === 'super_admin',
    /** 明文查看手机号等敏感权限位 */
    canRevealPhone: !!currentUser?.permissions?.includes('reveal_phone'),
  };
}
