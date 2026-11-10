/**
 * mock 通用工具：统一响应包装、分页、延迟。
 * 所有 mock 处理器共用，保证与真实后端行为一致（code/message/data、分页结构）。
 */
import type { Request, Response } from 'express';

/** 统一成功响应 { code: 0, message: 'success', data } */
export function ok<T>(res: Response, data: T, delay = 200): void {
  setTimeout(() => {
    res.json({ code: 0, message: 'success', data });
  }, delay);
}

/** 统一失败响应 */
export function fail(res: Response, code: number, message: string, delay = 200): void {
  setTimeout(() => {
    res.status(200).json({ code, message, data: null });
  }, delay);
}

/** 解析分页参数（page 默认 1，page_size 默认 20、最大 100，与 API 文档一致） */
export function paginate<T>(
  req: Request,
  list: T[],
): { list: T[]; total: number; page: number; page_size: number } {
  const page = Math.max(1, parseInt(String(req.query.page ?? '1'), 10) || 1);
  const pageSize = Math.min(100, Math.max(1, parseInt(String(req.query.page_size ?? '20'), 10) || 20));
  return {
    list: list.slice((page - 1) * pageSize, page * pageSize),
    total: list.length,
    page,
    page_size: pageSize,
  };
}

export function toInt(v: unknown): number {
  return parseInt(String(v), 10);
}

export function includesKeyword(keyword: unknown, fields: (string | null | undefined)[]): boolean {
  const kw = String(keyword ?? '').trim().toLowerCase();
  if (!kw) return true;
  return fields.some((f) => (f ?? '').toLowerCase().includes(kw));
}

/** 8 位大小写字母+数字的不可枚举 token */
export function randomToken(): string {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789';
  let token = '';
  for (let i = 0; i < 8; i += 1) {
    token += chars[Math.floor(Math.random() * chars.length)];
  }
  return token;
}

/** 证件号脱敏：保留前 4 后 4，与文档示例（3101**********3456）一致 */
export function maskIdCard(raw: string): string {
  if (raw.length <= 8) return `${raw.slice(0, 1)}****${raw.slice(-1)}`;
  return `${raw.slice(0, 4)}**********${raw.slice(-4)}`;
}

/** 手机号脱敏：138****5678 */
export function maskPhone(phone: string): string {
  return phone.length === 11 ? `${phone.slice(0, 3)}****${phone.slice(-4)}` : phone;
}
