import type { RequestConfig } from '@umijs/max';
import { history } from '@umijs/max';
import { message } from 'antd';
import { getToken } from '@/services/youwumeng/auth';
import type { ApiResponse } from '@/services/youwumeng/typings';

/**
 * 错误处理方案：与优舞盟后端统一响应包装 { code, message, data } 对齐。
 * code === 0 成功；业务错误按 code 提示；40101 未登录跳转登录页。
 * @doc https://umijs.org/docs/max/request
 */
/** 业务错误抛出：后端 HTTP 200 + code != 0 时抛 BizError（供 errorHandler 处理） */
const throwBizError = (res: unknown): void => {
  const { code, message, data } = res as unknown as ApiResponse<unknown>;
  if (code !== 0) {
    const error = new Error(message) as Error & {
      name: string;
      info: { code: number; message: string; data: unknown };
    };
    error.name = 'BizError';
    error.info = { code, message, data };
    throw error;
  }
};

export const errorConfig: RequestConfig = {
  errorConfig: {
    errorThrower: throwBizError,
    // 错误接收及处理
    errorHandler: (error: unknown, opts: { skipErrorHandler?: boolean }) => {
      if (opts?.skipErrorHandler) throw error;
      const err = error as {
        name?: string;
        info?: { code: number; message: string };
        response?: { status?: number };
      };

      if (err?.name === 'BizError' && err.info) {
        const { code, message: msg } = err.info;
        if (code === 40101) {
          // 未登录/登录过期 → 回登录页
          const { pathname, search, hash } = history.location;
          history.replace(
            `/user/login?redirect=${encodeURIComponent(pathname + search + hash)}`,
          );
          return;
        }
        message.error(msg);
        return;
      }
      if (err?.response) {
        message.error(`请求失败：HTTP ${err.response.status}`);
        return;
      }
      message.error('网络异常，请重试');
    },
  },

  // 请求拦截器：注入 Bearer token
  requestInterceptors: [
    (config: Record<string, unknown>) => {
      const token = getToken();
      if (token) {
        config.headers = {
          ...((config.headers ?? {}) as Record<string, string>),
          Authorization: `Bearer ${token}`,
        };
      }
      return config;
    },
  ],

  // 业务错误在此抛出（后端约定 code !== 0），成功时解包 data，
  // 使 service 层拿到的即为业务数据（与声明的返回类型一致）。
  responseInterceptors: [
    ((response: { data: unknown }) => {
      const body = response.data;
      if (body && typeof body === 'object' && 'code' in body) {
        throwBizError(body);
        return { ...response, data: (body as ApiResponse<unknown>).data };
      }
      return response;
    }) as never,
  ],
};
