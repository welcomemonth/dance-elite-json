/**
 * 优舞盟 mock 数据仓库：内存数据集，被各 mock 处理器共享。
 * 数据刻意覆盖各种情况：全状态赛事、全状态订单、满员/关闭组别、
 * 候补队列、资料缺失组合、重复选手档案、待处理/失败退款工单等。
 * 所有接口路径与 docs/优舞盟报名功能/API接口设计.md 第二部分一致。
 */

const T = (s: string) => `${s}+08:00`;

type SeasonStatus = 'upcoming' | 'ongoing' | 'archived';

// ---------- 账号 ----------

export interface MockAdmin {
  admin_id: number;
  username: string;
  password: string;
  name: string;
  role: 'super_admin' | 'admin' | 'cs';
  status: 'active' | 'disabled';
  permissions: string[];
  last_login_at: string;
  created_at: string;
}

export const admins: MockAdmin[] = [
  {
    admin_id: 1, username: 'admin', password: 'Admin@12345', name: '张三',
    role: 'super_admin', status: 'active', permissions: ['reveal_phone'],
    last_login_at: T('2026-11-10T09:12:00'), created_at: T('2026-01-05T10:00:00'),
  },
  {
    admin_id: 2, username: 'ops', password: 'Ops@12345', name: '李四',
    role: 'admin', status: 'active', permissions: ['reveal_phone'],
    last_login_at: T('2026-11-09T18:30:00'), created_at: T('2026-03-12T10:00:00'),
  },
  {
    admin_id: 3, username: 'cs01', password: 'Cs@12345', name: '客服小李',
    role: 'cs', status: 'active', permissions: [],
    last_login_at: T('2026-11-10T08:45:00'), created_at: T('2026-06-01T10:00:00'),
  },
];

// ---------- 赛季 ----------

export const seasons: { season_id: number; name: string; start_date: string; end_date: string; status: SeasonStatus; boards: { level: 'U11' | 'U13' | 'U15'; dance_type: 'classical' | 'folk' }[]; created_at: string }[] = [
  {
    season_id: 1, name: '2026 赛季', start_date: '2026-01-01', end_date: '2026-12-31',
    status: 'ongoing',
    boards: [
      { level: 'U11', dance_type: 'classical' }, { level: 'U11', dance_type: 'folk' },
      { level: 'U13', dance_type: 'classical' }, { level: 'U13', dance_type: 'folk' },
      { level: 'U15', dance_type: 'classical' }, { level: 'U15', dance_type: 'folk' },
    ],
    created_at: T('2026-01-01T00:00:00'),
  },
  {
    season_id: 2, name: '2025 赛季', start_date: '2025-01-01', end_date: '2025-12-31',
    status: 'archived',
    boards: [
      { level: 'U11', dance_type: 'classical' }, { level: 'U11', dance_type: 'folk' },
      { level: 'U13', dance_type: 'classical' }, { level: 'U13', dance_type: 'folk' },
      { level: 'U15', dance_type: 'classical' }, { level: 'U15', dance_type: 'folk' },
    ],
    created_at: T('2025-01-01T00:00:00'),
  },
];

// ---------- 赛事 ----------

let groupSeq = 100;

interface MockGroup {
  group_id: number;
  level: 'U11' | 'U13' | 'U15';
  dance_type: 'classical' | 'folk';
  quota: number;
  registered: number;
  enabled: boolean;
}

function makeGroup(level: 'U11' | 'U13' | 'U15', dance_type: 'classical' | 'folk', quota: number, registered: number, enabled = true): MockGroup {
  groupSeq += 1;
  return { group_id: groupSeq, level, dance_type, quota, registered, enabled };
}

export interface MockEvent {
  event_id: number;
  name: string;
  season_id: number;
  level: 'grade_a' | 'super' | 'final';
  city: string;
  venue: string;
  event_date: string;
  fee: number;
  cover_url: string;
  status: 'draft' | 'unpublished' | 'registering' | 'reg_closed' | 'ongoing' | 'finished' | 'offline';
  timeline: {
    reg_start_at: string; reg_end_at: string; lottery_at: string;
    material_check_at: string; start_at: string;
  };
  groups: MockGroup[];
  refund_policy: string;
  work_duration_limit: string;
  process_desc: string;
  honor_desc: string;
  exclude_from_prize: boolean;
  audio_review_token: string | null;
  created_at: string;
  updated_at: string;
}

const DEFAULT_REFUND_POLICY = '抽签前可无偿退赛，全额原路退回；抽签后个人原因退赛扣除 50% 报名费；伤病凭医院诊断证明可顺延不扣费。所有退款原路退回。';

export const events: MockEvent[] = [
  {
    event_id: 1,
    name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛',
    season_id: 1, level: 'super', city: '上海', venue: '上海国际舞蹈中心',
    event_date: '2026-10-06', fee: 128000, cover_url: '/cover.jpg',
    status: 'finished',
    timeline: {
      reg_start_at: T('2026-08-01T09:00:00'), reg_end_at: T('2026-09-01T18:00:00'),
      lottery_at: T('2026-09-05T10:00:00'), material_check_at: T('2026-09-30T18:00:00'),
      start_at: T('2026-10-06T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 40), makeGroup('U11', 'folk', 40, 36),
      makeGroup('U13', 'classical', 40, 40), makeGroup('U13', 'folk', 40, 40),
      makeGroup('U15', 'classical', 30, 30), makeGroup('U15', 'folk', 30, 27),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '冠军授予菁英舞者称号并直通年度总决赛；各奖项按奖金规则发放。',
    exclude_from_prize: false,
    audio_review_token: 'a8K2mP9x',
    created_at: T('2026-07-15T10:00:00'), updated_at: T('2026-10-06T20:00:00'),
  },
  {
    event_id: 2,
    name: '2026 上海甲级赛',
    season_id: 1, level: 'grade_a', city: '上海', venue: '上海戏剧学院实验剧场',
    event_date: '2026-12-20', fee: 98000, cover_url: '/cover.jpg',
    status: 'registering',
    timeline: {
      reg_start_at: T('2026-11-01T09:00:00'), reg_end_at: T('2026-12-05T18:00:00'),
      lottery_at: T('2026-12-08T10:00:00'), material_check_at: T('2026-12-15T18:00:00'),
      start_at: T('2026-12-20T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 40), makeGroup('U11', 'folk', 40, 28),
      makeGroup('U13', 'classical', 40, 35), makeGroup('U13', 'folk', 40, 40),
      makeGroup('U15', 'classical', 30, 30, false), // 关闭组别示例
      makeGroup('U15', 'folk', 30, 12),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '冠军直通年度总决赛。',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-10-20T10:00:00'), updated_at: T('2026-11-08T15:00:00'),
  },
  {
    event_id: 3,
    name: '2026 北京甲级赛',
    season_id: 1, level: 'grade_a', city: '北京', venue: '北京舞蹈学院剧场',
    event_date: '2027-01-10', fee: 98000, cover_url: '',
    status: 'unpublished',
    timeline: {
      reg_start_at: T('2026-11-20T09:00:00'), reg_end_at: T('2026-12-20T18:00:00'),
      lottery_at: T('2026-12-23T10:00:00'), material_check_at: T('2027-01-05T18:00:00'),
      start_at: T('2027-01-10T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 0), makeGroup('U11', 'folk', 40, 0),
      makeGroup('U13', 'classical', 40, 0), makeGroup('U13', 'folk', 40, 0),
      makeGroup('U15', 'classical', 30, 0), makeGroup('U15', 'folk', 30, 0),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '冠军直通年度总决赛。',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-11-01T10:00:00'), updated_at: T('2026-11-01T10:00:00'),
  },
  {
    event_id: 4,
    name: '2027 杭州甲级赛（春季）',
    season_id: 1, level: 'grade_a', city: '杭州', venue: '杭州大剧院',
    event_date: '2027-03-15', fee: 98000, cover_url: '',
    status: 'draft',
    timeline: {
      reg_start_at: T('2027-01-10T09:00:00'), reg_end_at: T('2027-02-10T18:00:00'),
      lottery_at: T('2027-02-13T10:00:00'), material_check_at: T('2027-03-08T18:00:00'),
      start_at: T('2027-03-15T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 0), makeGroup('U11', 'folk', 40, 0),
      makeGroup('U13', 'classical', 40, 0), makeGroup('U15', 'classical', 30, 0),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-11-05T10:00:00'), updated_at: T('2026-11-05T10:00:00'),
  },
  {
    event_id: 5,
    name: '国舞菁英2026年度总决赛',
    season_id: 1, level: 'final', city: '上海', venue: '上海国际舞蹈中心大剧场',
    event_date: '2027-01-24', fee: 0, cover_url: '/cover.jpg',
    status: 'reg_closed',
    timeline: {
      reg_start_at: T('2026-10-10T09:00:00'), reg_end_at: T('2026-11-05T18:00:00'),
      lottery_at: T('2026-11-08T10:00:00'), material_check_at: T('2027-01-17T18:00:00'),
      start_at: T('2027-01-24T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 30, 22), makeGroup('U11', 'folk', 30, 25),
      makeGroup('U13', 'classical', 30, 30), makeGroup('U13', 'folk', 30, 28),
      makeGroup('U15', 'classical', 20, 20), makeGroup('U15', 'folk', 20, 15),
    ],
    refund_policy: '总决赛为邀请制，报名资格以晋级名单为准。',
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '决赛 → 当天颁奖',
    honor_desc: '各榜单前 8 名按奖金规则发放。',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-09-01T10:00:00'), updated_at: T('2026-11-06T10:00:00'),
  },
  {
    event_id: 6,
    name: '2026 广州超级赛',
    season_id: 1, level: 'super', city: '广州', venue: '广州大剧院',
    event_date: '2026-12-05', fee: 128000, cover_url: '',
    status: 'offline',
    timeline: {
      reg_start_at: T('2026-10-01T09:00:00'), reg_end_at: T('2026-11-01T18:00:00'),
      lottery_at: T('2026-11-04T10:00:00'), material_check_at: T('2026-11-30T18:00:00'),
      start_at: T('2026-12-05T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 31), makeGroup('U11', 'folk', 40, 26),
      makeGroup('U13', 'classical', 40, 33), makeGroup('U13', 'folk', 40, 29),
      makeGroup('U15', 'classical', 30, 21), makeGroup('U15', 'folk', 30, 18),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '前 2 名直通年度总决赛。',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-08-20T10:00:00'), updated_at: T('2026-11-02T09:00:00'),
  },
  {
    event_id: 7,
    name: '2026 成都甲级赛',
    season_id: 1, level: 'grade_a', city: '成都', venue: '四川大剧院',
    event_date: '2026-11-15', fee: 98000, cover_url: '',
    status: 'ongoing',
    timeline: {
      reg_start_at: T('2026-09-10T09:00:00'), reg_end_at: T('2026-10-15T18:00:00'),
      lottery_at: T('2026-10-18T10:00:00'), material_check_at: T('2026-11-10T18:00:00'),
      start_at: T('2026-11-15T09:00:00'),
    },
    groups: [
      makeGroup('U11', 'classical', 40, 34), makeGroup('U11', 'folk', 40, 30),
      makeGroup('U13', 'classical', 40, 38), makeGroup('U13', 'folk', 40, 35),
      makeGroup('U15', 'classical', 30, 24), makeGroup('U15', 'folk', 30, 22),
    ],
    refund_policy: DEFAULT_REFUND_POLICY,
    work_duration_limit: '作品时长 ≤3 分钟',
    process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
    honor_desc: '冠军直通年度总决赛。',
    exclude_from_prize: false,
    audio_review_token: null,
    created_at: T('2026-08-25T10:00:00'), updated_at: T('2026-11-14T08:00:00'),
  },
];

// ---------- 选手档案 ----------

const playerNames = [
  '张小雨', '李思远', '王梦琪', '陈子涵', '刘欣怡', '赵若曦', '孙艺涵', '周语桐',
  '吴诗涵', '郑紫萱', '冯浩宇', '褚雨泽', '卫梓萱', '蒋一诺', '沈梦瑶',
];

export interface MockPlayer {
  player_id: number;
  player_no: string;
  name: string;
  id_type: 'id_card' | 'passport' | 'hk_macau' | 'other';
  id_card_no: string | null;
  birth_date: string;
  gender: 'male' | 'female';
  institution: string;
  merged_into: number | null;
  created_at: string;
}

export const players: MockPlayer[] = playerNames.map((name, i) => {
  const foreign = i === 9; // Emma 场景：无大陆身份证
  return {
    player_id: i + 1,
    player_no: `D${String(26001 + i)}`,
    name: foreign ? 'Emma Wilson' : name,
    id_type: foreign ? 'passport' : 'id_card',
    id_card_no: foreign ? 'P1234567' : `3101${String(20120101 + i * 701).slice(0, 4)}${String(1000 + i * 37).slice(1)}${String(1000 + i * 91).slice(1)}`,
    birth_date: foreign ? '2013-04-12' : `201${(i % 5) + 2}-${String((i % 12) + 1).padStart(2, '0')}-15`,
    gender: i % 6 === 4 ? 'male' : 'female',
    institution: ['星舞艺术', '小天鹅舞蹈', '蓓蕾舞蹈中心', '彩虹舞蹈工作室', ''][i % 5],
    merged_into: null,
    created_at: T(`2026-0${(i % 9) + 1}-1${i % 9}T10:00:00`),
  };
});

// 重复档案示例：同证件号不同微信报名产生两条档案（id 16 指向 6）
players.push({
  ...players[5], player_id: 16, player_no: 'D26116', merged_into: 6,
  created_at: T('2026-09-11T10:00:00'),
});

const institutions = ['星舞艺术', '小天鹅舞蹈', '蓓蕾舞蹈中心', '彩虹舞蹈工作室', '舞动童年'];

// ---------- 订单 ----------

export interface MockOrder {
  order_no: string;
  transaction_id: string;
  event_id: number;
  event_name: string;
  level: 'U11' | 'U13' | 'U15';
  dance_type: 'classical' | 'folk';
  player_id: number;
  player_name: string;
  id_card_no: string | null;
  id_type: string;
  amount: number;
  status: 'paid' | 'canceled' | 'refunding' | 'refunded';
  source: 'miniapp' | 'manual';
  institution: string;
  contact_phone: string;
  cancel_reason: string | null;
  registration_id: number;
  program_name: string | null;
  audio_url: string | null;
  photo_url: string | null;
  paid_at: string | null;
  created_at: string;
  materials_updated_at: string | null;
  remarks: { id: number; content: string; operator: string; created_at: string }[];
}

let orderSeq = 0;
let regSeq = 5000;

function makeOrder(partial: Partial<MockOrder>): MockOrder {
  orderSeq += 1;
  regSeq += 1;
  const player = players[(orderSeq - 1) % players.length];
  const created = partial.created_at ?? T(`2026-11-${String((orderSeq % 9) + 1).padStart(2, '0')}T${String(9 + (orderSeq % 8)).padStart(2, '0')}:30:00`);
  const paid = partial.status !== 'canceled';
  return {
    order_no: `20261${String(1010100000 + orderSeq * 137)}`,
    transaction_id: `42000012342026${String(11010000 + orderSeq * 61)}`,
    event_id: 2,
    event_name: '2026 上海甲级赛',
    level: ['U11', 'U13', 'U15'][orderSeq % 3] as 'U11' | 'U13' | 'U15',
    dance_type: orderSeq % 2 === 0 ? 'classical' : 'folk',
    player_id: player.player_id,
    player_name: player.name,
    id_card_no: player.id_card_no,
    id_type: player.id_type,
    amount: 98000,
    status: 'paid',
    source: 'miniapp',
    institution: player.institution || institutions[orderSeq % institutions.length],
    contact_phone: `138${String(12340000 + orderSeq * 137).slice(0, 8)}`,
    cancel_reason: null,
    registration_id: regSeq,
    program_name: orderSeq % 4 === 0 ? null : ['春闺梦', '扇舞丹青', '爱莲说', '罗敷行', '胭脂扣'][orderSeq % 5],
    audio_url: orderSeq % 3 === 0 ? null : `/static/materials/${regSeq}/audio.mp3`,
    photo_url: orderSeq % 5 === 0 ? null : `/static/materials/${regSeq}/photo.jpg`,
    paid_at: paid ? created : null,
    created_at: created,
    materials_updated_at: orderSeq % 3 === 0 ? null : created,
    remarks: [],
    ...partial,
  };
}

export const orders: MockOrder[] = [];

// 赛事 2（上海甲级赛，报名中）主力数据：各状态全覆盖
const statuses: MockOrder['status'][] = [
  'paid', 'paid', 'paid', 'paid', 'paid', 'paid', 'paid', 'paid', 'paid', 'paid',
  'paid', 'paid', 'refunding', 'refunding', 'refunded', 'refunded', 'canceled', 'canceled', 'canceled',
];
statuses.forEach((status, i) => {
  orders.push(makeOrder({
    status,
    level: (['U11', 'U13', 'U15'] as const)[i % 3],
    dance_type: i % 2 === 0 ? 'classical' : 'folk',
    cancel_reason: status === 'canceled' ? (i % 2 === 0 ? '用户主动取消' : '支付超时关闭') : null,
  }));
});

// 历史赛事 1（已结束）少量订单 + 1 笔人工报名（Emma，免费）
for (let i = 0; i < 8; i += 1) {
  orders.push(makeOrder({
    event_id: 1, event_name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛',
    amount: 128000,
    created_at: T(`2026-08-${String((i % 4) + 1).padStart(2, '0')}T10:00:00`),
  }));
}
orders.push(makeOrder({
  event_id: 5, event_name: '国舞菁英2026年度总决赛',
  player_id: 10, amount: 0, source: 'manual', status: 'paid',
  program_name: '天鹅湖', audio_url: null, photo_url: '/static/materials/5101/photo.jpg',
  created_at: T('2026-10-12T14:00:00'),
}));

// ---------- 退款工单 ----------

export interface MockRefundTicket {
  ticket_id: number;
  ticket_no: string;
  order_no: string;
  event_id: number;
  event_name: string;
  player_name: string;
  reason: string;
  content: string;
  applicant: string;
  status: 'pending' | 'processing' | 'approved' | 'rejected' | 'refund_failed';
  handler_name: string | null;
  reject_reason: string | null;
  refund_amount: number | null;
  created_at: string;
  handled_at: string | null;
}

export const refundTickets: MockRefundTicket[] = [
  {
    ticket_id: 901, ticket_no: 'TK20261102001', order_no: orders[12].order_no,
    event_id: 2, event_name: '2026 上海甲级赛', player_name: orders[12].player_name,
    reason: '孩子伤病无法参赛', content: '家长来电：孩子上周骨折，附医院诊断证明，申请全额退款。',
    applicant: '客服小李', status: 'pending', handler_name: null, reject_reason: null, refund_amount: null,
    created_at: T('2026-11-02T10:20:00'), handled_at: null,
  },
  {
    ticket_id: 902, ticket_no: 'TK20261105002', order_no: orders[0].order_no,
    event_id: 2, event_name: '2026 上海甲级赛', player_name: orders[0].player_name,
    reason: '个人原因退赛', content: '用户小程序客服会话留言：时间安排冲突，申请退赛。',
    applicant: '用户本人', status: 'pending', handler_name: null, reject_reason: null, refund_amount: null,
    created_at: T('2026-11-05T15:40:00'), handled_at: null,
  },
  {
    // 批准该工单会模拟微信支付退款 API 失败（41014 → refund_failed），用于演示失败重试
    ticket_id: 903, ticket_no: 'TK20261108003', order_no: orders[13].order_no,
    event_id: 2, event_name: '2026 上海甲级赛', player_name: orders[13].player_name,
    reason: '重复报名，申请退费', content: '同一选手误报两个组别，申请退掉本单。',
    applicant: '客服小李', status: 'pending', handler_name: null, reject_reason: null, refund_amount: null,
    created_at: T('2026-11-08T09:10:00'), handled_at: null,
  },
  {
    ticket_id: 904, ticket_no: 'TK20261020004', order_no: orders[20].order_no,
    event_id: 1, event_name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛', player_name: orders[20].player_name,
    reason: '行程冲突', content: '比赛时间与学校活动冲突。', applicant: '用户本人',
    status: 'approved', handler_name: '张三', reject_reason: null, refund_amount: 64000,
    created_at: T('2026-10-20T11:00:00'), handled_at: T('2026-10-21T10:00:00'),
  },
  {
    ticket_id: 905, ticket_no: 'TK20261022005', order_no: orders[21].order_no,
    event_id: 1, event_name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛', player_name: orders[21].player_name,
    reason: '个人原因退赛', content: '不想参加了。', applicant: '用户本人',
    status: 'rejected', handler_name: '李四', reject_reason: '已过退费期（抽签后超过 7 天）',
    refund_amount: null,
    created_at: T('2026-10-22T14:30:00'), handled_at: T('2026-10-22T16:00:00'),
  },
  {
    ticket_id: 906, ticket_no: 'TK20261025006', order_no: orders[22].order_no,
    event_id: 1, event_name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛', player_name: orders[22].player_name,
    reason: '伤病退赛', content: '腿部扭伤，申请全额退。', applicant: '客服小李',
    status: 'refund_failed', handler_name: '张三', reject_reason: null, refund_amount: 128000,
    created_at: T('2026-10-25T10:00:00'), handled_at: T('2026-10-25T11:30:00'),
  },
];

// ---------- 候补 ----------

export interface MockWaitlist {
  waitlist_id: number;
  event_id: number;
  event_name: string;
  level: 'U11' | 'U13' | 'U15';
  dance_type: 'classical' | 'folk';
  player_name: string;
  id_card_no: string | null;
  institution: string;
  phone: string;
  status: 'waiting' | 'notified' | 'converted' | 'given_up' | 'removed';
  created_at: string;
  notified_at: string | null;
  note: string | null;
  order_no: string | null;
}

export const waitlists: MockWaitlist[] = [
  { waitlist_id: 301, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '林可欣', id_card_no: '3101201605052266', institution: '星舞艺术', phone: '13811112222', status: 'waiting', created_at: T('2026-11-01T09:00:00'), notified_at: null, note: null, order_no: null },
  { waitlist_id: 302, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '黄雨桐', id_card_no: '3101201708123344', institution: '小天鹅舞蹈', phone: '13933334444', status: 'waiting', created_at: T('2026-11-02T10:30:00'), notified_at: null, note: null, order_no: null },
  { waitlist_id: 303, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '徐沐宸', id_card_no: null, institution: '', phone: '13755556666', status: 'waiting', created_at: T('2026-11-03T08:15:00'), notified_at: null, note: null, order_no: null },
  { waitlist_id: 304, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '马若彤', id_card_no: '3101201603108877', institution: '蓓蕾舞蹈中心', phone: '13677778888', status: 'notified', created_at: T('2026-10-28T14:00:00'), notified_at: T('2026-11-04T11:00:00'), note: 'confirmed_intent', order_no: null },
  { waitlist_id: 305, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '高梓萱', id_card_no: '3101201709214455', institution: '彩虹舞蹈工作室', phone: '13599990000', status: 'converted', created_at: T('2026-10-26T16:00:00'), notified_at: T('2026-10-27T10:00:00'), note: '已完成支付', order_no: orders[3].order_no },
  { waitlist_id: 306, event_id: 2, event_name: '2026 上海甲级赛', level: 'U11', dance_type: 'classical', player_name: '罗一帆', id_card_no: '3101201607021122', institution: '', phone: '13122223333', status: 'removed', created_at: T('2026-10-25T09:30:00'), notified_at: null, note: '多次联系不上', order_no: null },
  { waitlist_id: 307, event_id: 2, event_name: '2026 上海甲级赛', level: 'U13', dance_type: 'folk', player_name: '唐艺昕', id_card_no: '3101201404156677', institution: '星舞艺术', phone: '13344445555', status: 'waiting', created_at: T('2026-11-06T10:00:00'), notified_at: null, note: null, order_no: null },
  { waitlist_id: 308, event_id: 2, event_name: '2026 上海甲级赛', level: 'U13', dance_type: 'folk', player_name: '韩雪儿', id_card_no: '3101201506082233', institution: '小天鹅舞蹈', phone: '13266667777', status: 'waiting', created_at: T('2026-11-07T15:00:00'), notified_at: null, note: null, order_no: null },
];

/** 各组别释放名额数（队列顶部提示用） */
export const releasedQuotas: Record<string, number> = {
  '2:U11:classical': 2,
};

// ---------- 用户（微信维度） ----------

export const wxUsers = Array.from({ length: 14 }, (_, i) => ({
  id: i + 1,
  nickname: `舞者家长${String(i + 1).padStart(2, '0')}`,
  avatar_url: '/logo.svg',
  phone_masked: `138****${String(1000 + i * 111).slice(1)}`,
  registration_count: (i * 7) % 5,
  status: i === 12 ? 'blocked' as const : 'active' as const,
  created_at: T(`2026-0${(i % 9) + 1}-1${i % 9}T09:00:00`),
  last_login_at: T(`2026-11-0${(i % 9) + 1}T2${i % 9}:00:00`),
}));

// ---------- 客服工单 ----------

export const csTickets = [
  { ticket_id: 501, type: 'refund', user_name: '张小雨', phone: '13812340001', content: '孩子伤病需退赛，已上传诊断证明。', status: 'pending', created_by: '客服小李', related_order_no: orders[12].order_no, remark: null, created_at: T('2026-11-02T10:25:00'), updated_at: T('2026-11-02T10:25:00') },
  { ticket_id: 502, type: 'no_id_card', user_name: 'Emma Wilson', phone: '13812340002', content: '外籍选手无大陆身份证号，申请人工报名通道。', status: 'processing', created_by: '客服小李', related_order_no: null, remark: '已联系管理员创建人工报名', created_at: T('2026-11-08T14:00:00'), updated_at: T('2026-11-09T09:00:00') },
  { ticket_id: 503, type: 'waitlist', user_name: '林可欣', phone: '13811112222', content: 'U11 古典舞已满，想确认候补排第几。', status: 'done', created_by: '客服小李', related_order_no: null, remark: '告知当前候补第 1 位', created_at: T('2026-11-03T09:10:00'), updated_at: T('2026-11-03T11:00:00') },
  { ticket_id: 504, type: 'other', user_name: '王梦琪', phone: '13812340003', content: '咨询比赛当天化妆间安排。', status: 'pending', created_by: '客服小李', related_order_no: null, remark: null, created_at: T('2026-11-09T08:30:00'), updated_at: T('2026-11-09T08:30:00') },
  { ticket_id: 505, type: 'refund', user_name: '李思远', phone: '13812340004', content: '重复报名申请退一单。', status: 'done', created_by: 'admin', related_order_no: orders[13].order_no, remark: '已转退款工单', created_at: T('2026-11-07T16:00:00'), updated_at: T('2026-11-08T09:05:00') },
];

// ---------- 内容配置 ----------

export const contents: Record<string, { value: Record<string, unknown>; updated_at: string }> = {
  points_rules_desc: {
    value: { text: '积分规则：冠军 100 分、亚军 80 分、季军 60 分、菁英舞者 50 分、特金奖 40 分、金奖 30 分、银奖 20 分。超级赛积分权重高于甲级赛。每名选手取本年度积分最高的 3 场计入总榜。' },
    updated_at: T('2026-10-01T10:00:00'),
  },
  event_faq: {
    value: { text: 'Q：报名后可以修改资料吗？\nA：资料可在「核对舞蹈音频」时间前随时补充和修改。\n\nQ：如何退款？\nA：小程序内无自助退款，请联系客服登记，管理员审核后原路退回。' },
    updated_at: T('2026-09-15T10:00:00'),
  },
  official_account: {
    value: { text: '扫码关注「优舞盟」公众号，获取赛事通知与榜单更新。', image_url: '/logo.svg' },
    updated_at: T('2026-08-01T10:00:00'),
  },
  announcement: {
    value: { text: '2026 年度总决赛将于 2027 年 1 月 24 日在上海国际舞蹈中心举行。', online_at: T('2026-11-01T00:00:00'), offline_at: T('2027-01-31T00:00:00') },
    updated_at: T('2026-11-01T00:00:00'),
  },
};

// ---------- 操作日志 ----------

export const logActions = [
  { admin_name: '张三', action: 'publish_event', target_type: 'event', target_id: 2, detail: { name: '2026 上海甲级赛' }, created_at: T('2026-10-20T15:00:00') },
  { admin_name: '张三', action: 'refund', target_type: 'refund_ticket', target_id: 904, detail: { order_no: orders[20]?.order_no, amount: 64000, before: 'paid', after: 'refunded' }, created_at: T('2026-10-21T10:00:00') },
  { admin_name: '李四', action: 'reject_refund', target_type: 'refund_ticket', target_id: 905, detail: { reason: '已过退费期' }, created_at: T('2026-10-22T16:00:00') },
  { admin_name: '张三', action: 'quota_change', target_type: 'event_group', target_id: 101, detail: { before: 30, after: 40 }, created_at: T('2026-10-25T11:00:00') },
  { admin_name: '李四', action: 'close_group', target_type: 'event_group', target_id: 105, detail: { event_id: 2, group: 'U15·古典舞' }, created_at: T('2026-11-05T14:00:00') },
  { admin_name: '张三', action: 'reveal_phone', target_type: 'order', target_id: 1, detail: { order_no: orders[0]?.order_no }, created_at: T('2026-11-06T10:12:00') },
  { admin_name: 'admin', action: 'login', target_type: 'admin', target_id: 1, detail: {}, created_at: T('2026-11-10T09:12:00') },
  { admin_name: '张三', action: 'gen_audio_review_token', target_type: 'event', target_id: 1, detail: {}, created_at: T('2026-09-28T10:00:00') },
  { admin_name: '张三', action: 'offline_event', target_type: 'event', target_id: 6, detail: { name: '2026 广州超级赛' }, created_at: T('2026-11-02T09:00:00') },
  { admin_name: '客服小李', action: 'cs_ticket', target_type: 'cs_ticket', target_id: 501, detail: { type: 'refund' }, created_at: T('2026-11-02T10:25:00') },
];

// ---------- 榜单中心（待定接口的数据） ----------

export const scoreRules = {
  base_points: {
    champion: 100, runner_up: 80, third_place: 60, elite_dancer: 50,
    special_gold: 40, gold: 30, silver: 20,
  },
  level_weights: { grade_a: 1.0, super: 1.2, final: 1.5 },
  size_bands: [
    { min_players: 40, factor: 1.2 },
    { min_players: 20, factor: 1.1 },
    { min_players: 0, factor: 1.0 },
  ],
};

const awards = ['champion', 'runner_up', 'third_place', 'elite_dancer', 'special_gold', 'gold', 'silver'] as const;

export const leaderboardRows = (level: string, danceType: string) =>
  Array.from({ length: 12 }, (_, i) => ({
    rank: i + 1,
    player_id: i + 1,
    player_no: `D${String(26001 + i)}`,
    player_name: playerNames[i],
    display_name: i === 4 ? '小小舞蹈家' : null,
    institution: institutions[i % institutions.length],
    total_points: Math.max(0, 320 - i * 23 + ((level.charCodeAt(2) + danceType.length + i) % 17)),
    event_count: Math.min(3, 1 + (i % 3)),
    hidden: i === 10,
    direct_qualification: i < 2,
  })).sort((a, b) => b.total_points - a.total_points)
    .map((r, i) => ({ ...r, rank: i + 1 }));

export const qualificationCards = [
  { card_id: 1, player_id: 1, player_no: 'D26001', player_name: '张小雨', board: 'U11·古典舞', source: 'auto' as const, reason: null, granted_at: T('2026-10-07T10:00:00') },
  { card_id: 2, player_id: 3, player_no: 'D26003', player_name: '王梦琪', board: 'U13·古典舞', source: 'auto' as const, reason: null, granted_at: T('2026-10-07T10:00:00') },
  { card_id: 3, player_id: 10, player_no: 'D26010', player_name: 'Emma Wilson', board: 'U11·民族民间舞', source: 'manual' as const, reason: '国际邀请赛特邀', granted_at: T('2026-11-01T10:00:00') },
];

export const finalsRoster = Array.from({ length: 22 }, (_, i) => ({
  player_id: i + 1,
  player_no: `D${String(26001 + i)}`,
  player_name: playerNames[i % playerNames.length],
  institution: institutions[i % institutions.length],
  source: i < 3 ? 'direct_card' as const : 'points_top20' as const,
  total_points: Math.max(0, 300 - i * 13),
  rank: i + 1,
  manual: i === 21,
}));

export const institutionRanking = institutions.map((name, i) => ({
  rank: i + 1,
  institution: name,
  total_points: 860 - i * 110,
  player_count: 8 - i,
  honor: i < 3 ? 'annual_elite' as const : (i < 10 ? 'annual_top10' as const : null),
  prize_amount: i < 3 ? 1000000 : 0,
}));

export const prizeRules = {
  grade_a: { ranks: [1], threshold: 40, above: 200000, below: 100000 },
  super: { ranks: [1, 2], threshold: 40, above: [300000, 150000], below: [200000, 100000] },
  final: { amounts: [1000000, 300000, 200000, 100000, 100000, 100000, 100000, 100000] },
  institution_top3: 1000000,
};

export const prizeStatements = Array.from({ length: 14 }, (_, i) => {
  const isInst = i >= 11;
  return {
    item_id: i + 1,
    event_id: i < 8 ? 1 : 2,
    event_name: i < 8 ? '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛' : '2026 上海甲级赛',
    player_id: isInst ? null : i + 1,
    player_name: isInst ? '' : playerNames[i],
    institution: isInst ? institutions[i - 11] : institutions[i % institutions.length],
    award: isInst ? '年度菁英机构奖' : ['冠军', '亚军', '季军', '菁英舞者', '特金奖', '金奖', '银奖', '冠军'][i % 8],
    amount: isInst ? 1000000 : [300000, 150000, 100000, 50000, 40000, 30000, 20000, 200000][i % 8],
    adjusted_amount: 0,
    adjust_reason: null as string | null,
    paid: i % 3 === 0,
    paid_at: i % 3 === 0 ? T(`2026-10-${String((i % 9) + 1).padStart(2, '0')}T15:00:00`) : null,
    payee: i % 3 === 0 ? (isInst ? '机构负责人' : '家长代领') : null,
    remark: null as string | null,
    exclude_from_prize: false,
  };
});

/** 看板趋势：近 30 日 */
export const trend = Array.from({ length: 30 }, (_, i) => {
  const day = new Date(2026, 10, i + 1);
  const date = `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`;
  const registrations = 3 + ((i * 7919) % 17);
  return { date, registrations, revenue: registrations * (i % 4 === 0 ? 128000 : 98000) };
});
