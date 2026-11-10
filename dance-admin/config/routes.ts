/**
 * @name umi 的路由配置
 * @description 只支持 path,component,routes,redirect,wrappers,name,icon,access 的配置
 * 菜单结构对齐 docs/后台管理/优舞盟后台管理页面与功能设计.md 0.3 节。
 * name 直接写中文（未配置 i18n key 时按原文展示）；不带 name 的路由不进左侧导航。
 */
export default [
  {
    path: '/user',
    layout: false,
    routes: [
      { path: '/user/login', name: '登录', component: './user/login' },
      { path: '/user', redirect: '/user/login' },
    ],
  },
  {
    path: '/audio-review/:token',
    layout: false,
    component: './audio-review',
  },
  {
    path: '/dashboard',
    name: '数据看板',
    icon: 'dashboard',
    component: './dashboard',
  },
  {
    path: '/event',
    name: '赛事管理',
    icon: 'trophy',
    routes: [
      { path: '/event', redirect: '/event/list' },
      { path: '/event/list', name: '赛事列表', component: './event/list' },
      { path: '/event/create', component: './event/edit' },
      { path: '/event/edit/:id', component: './event/edit' },
      { path: '/event/detail/:id', component: './event/detail' },
    ],
  },
  {
    path: '/order',
    name: '订单中心',
    icon: 'profile',
    routes: [
      { path: '/order', redirect: '/order/list' },
      { path: '/order/list', name: '订单管理', component: './order/list' },
      { path: '/order/detail/:orderNo', component: './order/detail' },
      { path: '/order/refunds', name: '退款工单', component: './refund/list' },
      { path: '/order/refunds/:id', component: './refund/detail' },
      { path: '/order/manual', name: '人工报名', component: './manual/list' },
      { path: '/order/manual/create', component: './manual/create' },
    ],
  },
  {
    path: '/operation',
    name: '报名运营',
    icon: 'orderedList',
    routes: [
      { path: '/operation', redirect: '/operation/waitlist' },
      { path: '/operation/waitlist', name: '候补管理', component: './waitlist/list' },
      { path: '/operation/materials', name: '资料催收', component: './materials/list' },
    ],
  },
  {
    path: '/rank',
    name: '榜单中心',
    icon: 'crown',
    routes: [
      { path: '/rank', redirect: '/rank/seasons' },
      { path: '/rank/seasons', name: '赛季管理', component: './rank/seasons' },
      { path: '/rank/scores', name: '成绩录入', component: './rank/scores' },
      { path: '/rank/leaderboards', name: '积分与榜单', component: './rank/leaderboards' },
      { path: '/rank/promotion', name: '晋级管理', component: './rank/promotion' },
      { path: '/rank/institutions', name: '机构年度榜', component: './rank/institutions' },
      { path: '/rank/prizes', name: '奖金管理', component: './rank/prizes' },
    ],
  },
  {
    path: '/user-content',
    name: '用户与内容',
    icon: 'user',
    routes: [
      { path: '/user-content', redirect: '/user-content/players' },
      { path: '/user-content/players', name: '选手/用户管理', component: './user-content/players' },
      { path: '/user-content/contents', name: '内容管理', component: './user-content/contents' },
      { path: '/user-content/tickets', name: '客服工单', component: './user-content/tickets' },
    ],
  },
  {
    path: '/system',
    name: '系统管理',
    icon: 'setting',
    routes: [
      { path: '/system', redirect: '/system/accounts' },
      { path: '/system/accounts', name: '账号与权限', component: './system/accounts' },
      { path: '/system/logs', name: '操作日志', component: './system/logs' },
    ],
  },
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    component: './exception/404',
    path: '/*',
  },
];
