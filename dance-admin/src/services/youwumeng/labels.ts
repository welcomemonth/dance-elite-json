/**
 * 枚举 → 中文展示映射（统一放在服务端语义之后的一层薄展示转换，前端不存业务规则）。
 * 与 docs/优舞盟报名功能/API接口设计.md 附录 B 枚举速查对齐。
 */

import type {
  AdminRole,
  AgeLevel,
  Award,
  ChargeType,
  CsTicketStatus,
  CsTicketType,
  DanceType,
  EventLevel,
  EventStatus,
  IdType,
  MissingItem,
  OrderSource,
  OrderStatus,
  RefundTicketStatus,
  SeasonStatus,
  WaitlistStatus,
} from './typings';

export const EVENT_STATUS_LABELS: Record<EventStatus, string> = {
  draft: '草稿',
  unpublished: '未发布',
  registering: '报名中',
  reg_closed: '报名截止',
  ongoing: '比赛中',
  finished: '已结束',
  offline: '已下架',
};

export const EVENT_LEVEL_LABELS: Record<EventLevel, string> = {
  grade_a: '甲级赛',
  super: '超级赛',
  final: '年度总决赛',
};

export const AGE_LEVEL_OPTIONS: AgeLevel[] = ['U11', 'U13', 'U15'];

export const DANCE_TYPE_LABELS: Record<DanceType, string> = {
  classical: '古典舞',
  folk: '民族民间舞',
};

export const ORDER_STATUS_LABELS: Record<OrderStatus, string> = {
  paid: '已支付',
  canceled: '已取消',
  refunding: '退款处理中',
  refunded: '已退款',
};

export const ORDER_SOURCE_LABELS: Record<OrderSource, string> = {
  miniapp: '小程序',
  manual: '后台人工',
};

export const REFUND_TICKET_STATUS_LABELS: Record<RefundTicketStatus, string> = {
  pending: '待处理',
  processing: '处理中',
  approved: '已同意',
  rejected: '已驳回',
  refund_failed: '退款失败',
};

export const WAITLIST_STATUS_LABELS: Record<WaitlistStatus, string> = {
  waiting: '候补中',
  notified: '已通知',
  converted: '已转报名',
  given_up: '已放弃',
  removed: '已移出',
};

export const ID_TYPE_LABELS: Record<IdType, string> = {
  passport: '护照',
  hk_macau: '港澳台证件',
  other: '其他',
};

export const CHARGE_TYPE_LABELS: Record<ChargeType, string> = {
  offline_paid: '线下已收款',
  free: '免费',
};

export const MISSING_ITEM_LABELS: Record<MissingItem, string> = {
  photo: '证件照',
  audio: '音频',
  program_name: '节目名称',
  contact_phone: '联系电话',
};

export const CS_TICKET_TYPE_LABELS: Record<CsTicketType, string> = {
  refund: '退款诉求',
  no_id_card: '无身份证号报名',
  waitlist: '候补咨询',
  other: '其他',
};

export const CS_TICKET_STATUS_LABELS: Record<CsTicketStatus, string> = {
  pending: '待处理',
  processing: '处理中',
  done: '已完成',
};

export const ADMIN_ROLE_LABELS: Record<AdminRole, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
  cs: '客服',
};

export const SEASON_STATUS_LABELS: Record<SeasonStatus, string> = {
  upcoming: '未开始',
  ongoing: '进行中',
  archived: '已归档',
};

export const AWARD_LABELS: Record<Award, string> = {
  champion: '冠军',
  runner_up: '亚军',
  third_place: '季军',
  elite_dancer: '菁英舞者',
  special_gold: '特金奖',
  gold: '金奖',
  silver: '银奖',
};

export const AWARD_OPTIONS = Object.entries(AWARD_LABELS).map(([value, label]) => ({
  value,
  label,
}));

/** 榜单标识：U11·古典舞 等 */
export const boardLabel = (level: string, danceType: string): string =>
  `${level}·${DANCE_TYPE_LABELS[danceType as DanceType] ?? danceType}`;
