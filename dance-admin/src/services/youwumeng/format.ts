/** 展示格式化工具 */

/** 金额：分 → 元（保留两位，千分位） */
export function fenToYuan(fen: number): string {
  return (fen / 100).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

/** 金额渲染：分 → ¥1,234.00 */
export function formatMoney(fen: number): string {
  return `¥${fenToYuan(fen)}`;
}

/** 日期时间（ISO8601 本地时间）→ YYYY-MM-DD HH:mm */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '-';
  return iso.slice(0, 16).replace('T', ' ');
}

/** 日期（ISO8601）→ YYYY-MM-DD */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '-';
  return iso.slice(0, 10);
}
