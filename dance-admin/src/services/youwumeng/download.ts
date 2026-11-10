import { request } from '@umijs/max';

/**
 * 文件下载辅助：带鉴权头请求文件流并触发浏览器保存。
 * 真实后端导出接口返回文件流（csv/xlsx）。
 */
export async function downloadFile(url: string, params?: Record<string, unknown>): Promise<void> {
  const res = (await request(url, {
    method: 'GET',
    params,
    responseType: 'blob',
    skipErrorHandler: true,
  } as never)) as unknown as Blob;

  if (res.type.includes('application/json')) {
    const text = await res.text();
    const parsed = JSON.parse(text) as { message?: string };
    throw new Error(parsed.message || '导出失败');
  }

  const objectUrl = URL.createObjectURL(res);
  const a = document.createElement('a');
  a.href = objectUrl;
  a.download = `export-${Date.now()}`;
  a.click();
  URL.revokeObjectURL(objectUrl);
}
