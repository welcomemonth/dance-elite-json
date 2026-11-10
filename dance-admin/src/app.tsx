import type { Settings as LayoutSettings } from '@ant-design/pro-components';
import type { RequestConfig, RunTimeLayoutConfig } from '@umijs/max';
import { history, Link } from '@umijs/max';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import React from 'react';

// Initialize dayjs plugins globally
dayjs.extend(relativeTime);

import { AvatarDropdown, ErrorBoundary, OfflineBanner } from '@/components';
import { clearSession, getStoredAdmin } from '@/services/youwumeng/auth';
import { ADMIN_ROLE_LABELS } from '@/services/youwumeng/labels';
import type { Admin } from '@/services/youwumeng/typings';
import defaultSettings from '../config/defaultSettings';
import { errorConfig } from './requestErrorConfig';

const loginPath = '/user/login';

/**
 * @see https://umijs.org/docs/api/runtime-config#getinitialstate
 * 会话完全保存在本地（JWT + 管理员信息，见 services/youwumeng/auth.ts）；
 * 后端无 /me 校验接口，token 过期由请求 40101 统一拦截回登录页。
 * */
export async function getInitialState(): Promise<{
  settings?: Partial<LayoutSettings>;
  currentUser?: Admin;
  loading?: boolean;
}> {
  const currentUser = getStoredAdmin();
  return {
    currentUser,
    settings: defaultSettings as Partial<LayoutSettings>,
  };
}

// ProLayout 支持的api https://procomponents.ant.design/components/layout
export const layout: RunTimeLayoutConfig = ({ initialState }) => {
  return {
    menuItemRender: (item, dom) => {
      if (item.path) {
        return (
          <Link to={item.path} prefetch>
            {dom}
          </Link>
        );
      }
      return dom;
    },
    actionsRender: () => [],
    avatarProps: {
      title: initialState?.currentUser
        ? `${initialState.currentUser.name}（${ADMIN_ROLE_LABELS[initialState.currentUser.role]}）`
        : '',
      render: (_, avatarChildren) => (
        <AvatarDropdown>{avatarChildren}</AvatarDropdown>
      ),
    },
    footerRender: () => null,
    onPageChange: () => {
      const { location } = history;
      // 未登录访问业务页面 → 重定向登录页
      if (!initialState?.currentUser && location.pathname !== loginPath) {
        clearSession();
        history.replace(
          `${loginPath}?redirect=${encodeURIComponent(location.pathname + location.search + location.hash)}`,
        );
      }
    },
    menuHeaderRender: undefined,
    childrenRender: (children) => <>{children}</>,
    ...initialState?.settings,
  };
};

/**
 * @name request 配置
 * 前端请求一律使用相对路径（/api/v1/admin/**、/h5/**）：
 * - mock 开发：`pnpm start`，由 mock/ 目录拦截；
 * - 真实后端：`pnpm dev`（MOCK=none），由 config/proxy.ts 代理到 Go 服务。
 * @doc https://umijs.org/docs/max/request
 */
export const request: RequestConfig = {
  ...errorConfig,
};

export function rootContainer(container: React.ReactNode) {
  return (
    <>
      <OfflineBanner />
      <ErrorBoundary>{container}</ErrorBoundary>
    </>
  );
}
