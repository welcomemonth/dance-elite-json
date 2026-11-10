/**
 * 优舞盟后台管理端 API 类型定义
 * 字段与 docs/优舞盟报名功能/API接口设计.md 第二部分（/api/v1/admin）对齐。
 * 榜单中心（赛季/成绩/积分/晋级/机构/奖金）接口文档尚未覆盖，
 * 以下方「榜单中心」一节标注（注释 *待定*）的类型为准，后续接入真实后端时以此契约为准。
 */

// ---------- 通用 ----------

/** 统一响应包装 { code, message, data } */
export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

/** 统一分页结构 */
export interface PagedList<T> {
  list: T[];
  total: number;
  page: number;
  page_size: number;
}

// ---------- 认证 ----------

export type AdminRole = 'super_admin' | 'admin' | 'cs';

export interface Admin {
  id: number;
  username: string;
  role: AdminRole;
  name: string;
  /** 敏感操作权限位（如明文查看手机号），缺省视为无 */
  permissions?: string[];
}

export interface LoginParams {
  username: string;
  password: string;
}

export interface LoginResult {
  token: string;
  expires_in: number;
  admin: Admin;
}

// ---------- 数据看板 ----------

export interface DashboardMetrics {
  today_registrations: number;
  today_revenue: number;
  total_registrations: number;
  total_revenue: number;
  pending_refund_tickets: number;
  waitlist_total: number;
  incomplete_materials: number;
  active_events: number;
}

export interface ClosingEvent {
  event_id: number;
  event_name: string;
  days_left: number;
}

export interface DashboardTodos {
  pending_refunds: number;
  new_cs_tickets: number;
  closing_events: ClosingEvent[];
}

export interface EventProgress {
  event_id: number;
  event_name: string;
  level: string;
  dance_type: string;
  registered: number;
  quota: number;
  waitlist_count: number;
}

export interface TrendPoint {
  date: string;
  registrations: number;
  revenue: number;
}

export interface Dashboard {
  metrics: DashboardMetrics;
  todos: DashboardTodos;
  event_progress: EventProgress[];
  trend: TrendPoint[];
}

// ---------- 赛事 ----------

export type EventStatus =
  | 'draft'
  | 'unpublished'
  | 'registering'
  | 'reg_closed'
  | 'ongoing'
  | 'finished'
  | 'offline';

export type EventLevel = 'grade_a' | 'super' | 'final';

export type AgeLevel = 'U11' | 'U13' | 'U15';

export type DanceType = 'classical' | 'folk';

export interface EventTimeline {
  reg_start_at: string;
  reg_end_at: string;
  lottery_at: string;
  material_check_at: string;
  start_at: string;
}

/** 组别配置粒度 = 赛事 × 组别 × 舞种 */
export interface EventGroup {
  group_id: number;
  level: AgeLevel;
  dance_type: DanceType;
  quota: number;
  registered: number;
  /** 该组别是否开放报名（关闭组别后 false） */
  enabled: boolean;
}

export interface EventBrief {
  event_id: number;
  name: string;
  season_id: number;
  level: EventLevel;
  city: string;
  venue: string;
  event_date: string;
  fee: number;
  cover_url: string;
  status: EventStatus;
  timeline: EventTimeline;
  groups: EventGroup[];
  refund_policy: string;
  work_duration_limit: string;
  process_desc: string;
  honor_desc: string;
  exclude_from_prize: boolean;
  audio_review_token: string | null;
  created_at: string;
  updated_at: string;
}

export interface EventGroupInput {
  group_id?: number;
  level: AgeLevel;
  dance_type: DanceType;
  quota: number;
}

export interface EventInput {
  name: string;
  season_id: number;
  level: EventLevel;
  city?: string;
  venue: string;
  event_date: string;
  fee: number;
  timeline: EventTimeline;
  groups: EventGroupInput[];
  refund_policy?: string;
  work_duration_limit?: string;
  process_desc?: string;
  honor_desc?: string;
  exclude_from_prize?: boolean;
  publish?: boolean;
}

/** 赛事详情 = 列表项结构（含完整组别与进度），后续可独立扩展 */
export type EventDetail = EventBrief;

export interface AudioReviewTokenResult {
  url: string;
  token: string;
}

// ---------- 订单 ----------

export type OrderStatus = 'paid' | 'canceled' | 'refunding' | 'refunded';

export type OrderSource = 'miniapp' | 'manual';

export interface OrderBrief {
  order_no: string;
  transaction_id: string;
  event_id: number;
  event_name: string;
  level: AgeLevel;
  dance_type: DanceType;
  player_name: string;
  /** 脱敏证件号（系统仅存哈希，物理上无明文） */
  id_card_masked: string | null;
  amount: number;
  status: OrderStatus;
  source: OrderSource;
  paid_at: string | null;
  created_at: string;
}

export type PaymentFlowType = 'pay' | 'refund';

export interface PaymentFlow {
  type: PaymentFlowType;
  prepay_id: string;
  transaction_id: string;
  refund_id: string;
  amount: number;
  status: string;
  notified_at: string;
}

export interface RegistrationDetail {
  registration_id: number;
  event_name: string;
  level: AgeLevel;
  dance_type: DanceType;
  player_name: string;
  id_card_masked: string | null;
  id_type: string;
  institution: string;
  contact_phone: string;
  material_deadline: string;
}

export interface MaterialsInfo {
  photo_url: string | null;
  audio_url: string | null;
  audio_name: string | null;
  program_name: string | null;
  contact_phone: string | null;
  updated_at: string | null;
  /** 超过「核对舞蹈音频」时间后为 true，资料整体只读 */
  readonly: boolean;
}

export interface OperationLogEntry {
  action: string;
  from_status: string | null;
  to_status: string | null;
  operator: string;
  created_at: string;
}

export interface OrderRemark {
  id: number;
  content: string;
  operator: string;
  created_at: string;
}

export interface RefundTicketBrief {
  ticket_id: number;
  ticket_no: string;
  status: RefundTicketStatus;
}

export interface OrderDetail {
  order: OrderBrief & {
    cancel_reason: string | null;
    registration_id: number;
  };
  registration: RegistrationDetail;
  payments: PaymentFlow[];
  refund_ticket: RefundTicketBrief | null;
  materials: MaterialsInfo;
  operation_logs: OperationLogEntry[];
  remarks: OrderRemark[];
}

// ---------- 退款工单 ----------

export type RefundTicketStatus =
  | 'pending'
  | 'processing'
  | 'approved'
  | 'rejected'
  | 'refund_failed';

export interface RefundTicketBrief {
  ticket_id: number;
  ticket_no: string;
  order_no: string;
  event_id: number;
  event_name: string;
  player_name: string;
  /** 诉求摘要 */
  reason: string;
  applicant: string;
  status: RefundTicketStatus;
  handler_name: string | null;
  created_at: string;
  handled_at: string | null;
}

export interface RefundTicketDetail extends RefundTicketBrief {
  /** 客服登记内容原文 */
  content: string;
  /** 订单摘要 */
  order: OrderBrief;
  /** 政策阶段：lottery_before 抽签前 / lottery_after 抽签后 */
  policy_stage: 'lottery_before' | 'lottery_after';
  /** 按政策自动计算的可退金额（分） */
  suggest_amount: number;
  paid_amount: number;
  /** 审核/处理留痕 */
  history: { action: string; operator: string; created_at: string; detail: string }[];
}

// ---------- 人工报名 ----------

export type IdType = 'passport' | 'hk_macau' | 'other';

export type ChargeType = 'offline_paid' | 'free';

export interface ManualRegistrationInput {
  event_id: number;
  group_id: number;
  player_name: string;
  id_type: IdType;
  id_card_no: string;
  nationality?: string;
  institution?: string;
  contact_phone: string;
  charge_type: ChargeType;
  amount?: number;
  cs_ticket_id?: number;
  remark?: string;
}

// ---------- 候补 ----------

export type WaitlistStatus =
  | 'waiting'
  | 'notified'
  | 'converted'
  | 'given_up'
  | 'removed';

export interface WaitlistEntry {
  waitlist_id: number;
  position_no: number;
  event_id: number;
  event_name: string;
  level: AgeLevel;
  dance_type: DanceType;
  player_name: string;
  id_card_masked: string | null;
  institution: string;
  /** 联系电话（明文，仅本接口与导出） */
  phone: string;
  status: WaitlistStatus;
  created_at: string;
  notified_at: string | null;
  note: string | null;
}

export interface WaitlistList extends PagedList<WaitlistEntry> {
  /** 该组别当前释放出的名额数（队列顶部提示用） */
  released_quota: number;
}

// ---------- 资料催收 ----------

export type MissingItem = 'photo' | 'audio' | 'program_name' | 'contact_phone';

export interface MissingMaterialEntry {
  registration_id: number;
  order_no: string;
  event_id: number;
  event_name: string;
  level: AgeLevel;
  dance_type: DanceType;
  player_name: string;
  contact_phone: string;
  completed: number;
  /** 总项数（8） */
  total: number;
  missing_items: MissingItem[];
  material_deadline: string;
  material_updated_at: string | null;
  contacted: boolean;
  contact_note: string | null;
}

// ---------- 用户与选手 ----------

export interface WxUser {
  id: number;
  nickname: string;
  avatar_url: string;
  phone_masked: string;
  registration_count: number;
  status: 'active' | 'blocked';
  created_at: string;
  last_login_at: string;
}

export interface PlayerBrief {
  player_id: number;
  player_no: string;
  name: string;
  id_type: 'id_card' | 'passport' | 'hk_macau' | 'other';
  id_card_masked: string | null;
  birth_date: string;
  gender: 'male' | 'female';
  institution: string;
  /** 重复档案合并指向 */
  merged_into: number | null;
  registration_count: number;
  result_count: number;
}

export interface PlayerRegistration {
  order_no: string;
  event_name: string;
  level: AgeLevel;
  dance_type: DanceType;
  status: OrderStatus;
  created_at: string;
}

export interface PlayerResultSummary {
  award: string;
  event_name: string;
  points: number;
  created_at: string;
}

export interface PlayerDetail extends PlayerBrief {
  registrations: PlayerRegistration[];
  results_summary: PlayerResultSummary[];
  points_summary: { season_name: string; total_points: number; event_count: number };
  /** 同证件号的重复档案（可手动合并） */
  duplicates: { player_id: number; player_no: string; name: string; created_at: string }[];
}

// ---------- 客服工单 ----------

export type CsTicketType = 'refund' | 'no_id_card' | 'waitlist' | 'other';

export type CsTicketStatus = 'pending' | 'processing' | 'done';

export interface CsTicket {
  ticket_id: number;
  type: CsTicketType;
  user_name: string;
  phone: string;
  content: string;
  status: CsTicketStatus;
  created_by: string;
  related_order_no: string | null;
  remark: string | null;
  created_at: string;
  updated_at: string;
}

// ---------- 内容管理 ----------

export type ContentKey =
  | 'points_rules_desc'
  | 'event_faq'
  | 'official_account'
  | 'announcement';

export interface ContentValue {
  text: string;
  image_url?: string;
  online_at?: string;
  offline_at?: string;
}

export interface ContentEntry {
  key: ContentKey;
  value: ContentValue;
  updated_at: string;
}

// ---------- 系统管理 ----------

export interface AdminAccount {
  admin_id: number;
  username: string;
  name: string;
  role: AdminRole;
  status: 'active' | 'disabled';
  last_login_at: string | null;
  created_at: string;
}

export interface LogEntry {
  log_id: number;
  admin_name: string;
  action: string;
  target_type: string;
  target_id: number;
  detail: Record<string, unknown>;
  created_at: string;
}

// ---------- 榜单中心（*待定*：API 文档尚未覆盖，前后端以本契约为准） ----------

export type SeasonStatus = 'upcoming' | 'ongoing' | 'archived';

export interface Season {
  season_id: number;
  name: string;
  start_date: string;
  end_date: string;
  status: SeasonStatus;
  /** 适用榜单（默认 3 组别 × 2 舞种 = 6） */
  boards: { level: AgeLevel; dance_type: DanceType }[];
  created_at: string;
}

export type Award = 'champion' | 'runner_up' | 'third_place' | 'elite_dancer' | 'special_gold' | 'gold' | 'silver';

export interface ScoreEntry {
  entry_id: number;
  player_id: number;
  player_no: string;
  player_name: string;
  id_card_masked: string | null;
  award: Award | null;
  advanced_to_final: boolean;
  points: number;
  remark: string | null;
  published: boolean;
}

/** 成绩录入：解析预览行（Excel 导入或在线录入后、确认前的结构） */
export interface ScorePreviewRow {
  row_no: number;
  player_name: string;
  id_card_masked: string | null;
  award: Award | null;
  advanced_to_final: boolean;
  match_status: 'matched' | 'unmatched';
  player_id: number | null;
  remark: string | null;
}

export interface ScorePreview {
  total_rows: number;
  matched: number;
  unmatched: number;
  rows: ScorePreviewRow[];
}

export interface ScoreRules {
  /** 名次基础分表 */
  base_points: Record<Award, number>;
  /** 级别权重 */
  level_weights: { grade_a: number; super: number; final: number };
  /** 人数系数分档（按榜单参赛人数） */
  size_bands: { min_players: number; factor: number }[];
}

export interface LeaderboardRow {
  rank: number;
  player_id: number;
  player_no: string;
  player_name: string;
  /** 展示名（真名 / 化名 / 编号），null = 真名 */
  display_name: string | null;
  institution: string;
  total_points: number;
  event_count: number;
  hidden: boolean;
  direct_qualification: boolean;
}

export interface Leaderboard {
  level: AgeLevel;
  dance_type: DanceType;
  rows: LeaderboardRow[];
  published_at: string | null;
}

/** 直通卡 */
export interface QualificationCard {
  card_id: number;
  player_id: number;
  player_no: string;
  player_name: string;
  board: string;
  source: 'auto' | 'manual';
  reason: string | null;
  granted_at: string;
}

export interface FinalsRosterRow {
  player_id: number;
  player_no: string;
  player_name: string;
  institution: string;
  source: 'direct_card' | 'points_top20';
  total_points: number;
  rank: number;
  manual: boolean;
}

export interface InstitutionRankRow {
  rank: number;
  institution: string;
  total_points: number;
  player_count: number;
  honor: 'annual_elite' | 'annual_top10' | null;
  prize_amount: number;
}

export interface PrizeRules {
  grade_a: { ranks: number[]; threshold: number; above: number; below: number };
  super: { ranks: number[]; threshold: number; above: [number, number]; below: [number, number] };
  final: { amounts: number[] };
  institution_top3: number;
}

export interface PrizeStatementItem {
  item_id: number;
  event_id: number;
  event_name: string;
  player_id: number | null;
  player_name: string;
  institution: string | null;
  award: string;
  amount: number;
  adjusted_amount: number;
  adjust_reason: string | null;
  paid: boolean;
  paid_at: string | null;
  payee: string | null;
  remark: string | null;
  exclude_from_prize: boolean;
}

export interface AudioReviewEntry {
  player_name: string;
  program_name: string;
  audio_url: string;
  updated_at: string;
}

export interface AudioReviewData {
  event_name: string;
  entries: AudioReviewEntry[];
}
