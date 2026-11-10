import { useRequest } from '@umijs/max';

/**
 * 业务数据加载 hook：包装 service 层函数以适配 umi useRequest 的
 * `{ data: T }` 约定（其 formatResult 会取 `.data`）。
 * 返回的 data/run 均为解包后的业务类型。
 */
export function useService<T, P extends unknown[] = []>(
  service: (...args: P) => Promise<T>,
  options?: {
    manual?: boolean;
    ready?: boolean;
    refreshDeps?: unknown[];
    onSuccess?: (data: T, params: P) => void;
    onError?: (err: Error, params: P) => void;
  },
): {
  data: T | undefined;
  loading: boolean;
  error: Error | undefined;
  run: (...args: P) => Promise<T>;
  refresh: () => void;
  mutate: (data?: T | ((old?: T) => T | undefined)) => void;
} {
  return useRequest(
    async (...args: P) => ({ data: await service(...args) }),
    options as never,
  ) as never;
}
