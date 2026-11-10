/**
 * 用户与选手 mock：微信用户列表/选手档案列表与详情/合并。
 */
import type { Request, Response } from 'express';
import { orders, players, wxUsers } from './_db';
import { fail, includesKeyword, maskIdCard, ok, paginate, toInt } from './_utils';

export default {
  'GET /api/v1/admin/users': (req: Request, res: Response) => {
    let list = wxUsers;
    const q = req.query;
    if (q.status) list = list.filter((u) => u.status === q.status);
    if (q.keyword) list = list.filter((u) => includesKeyword(q.keyword, [u.nickname, u.phone_masked]));
    return ok(res, paginate(req, list));
  },

  'GET /api/v1/admin/players': (req: Request, res: Response) => {
    let list = players;
    const q = req.query;
    if (q.keyword) {
      list = list.filter((p) => includesKeyword(q.keyword, [p.name, p.player_no]));
    }
    const brief = list.map((p) => ({
      player_id: p.player_id,
      player_no: p.player_no,
      name: p.name,
      id_type: p.id_type,
      id_card_masked: p.id_card_no ? maskIdCard(p.id_card_no) : null,
      birth_date: p.birth_date,
      gender: p.gender,
      institution: p.institution,
      merged_into: p.merged_into,
      registration_count: orders.filter((o) => o.player_id === p.player_id).length,
      result_count: p.player_id <= 8 ? (p.player_id % 3) + 1 : 0,
    }));
    return ok(res, paginate(req, brief));
  },

  'GET /api/v1/admin/players/:id': (req: Request, res: Response) => {
    const player = players.find((p) => p.player_id === toInt(req.params.id));
    if (!player) return fail(res, 40401, '选手不存在');
    const myOrders = orders.filter((o) => o.player_id === player.player_id);
    return ok(res, {
      player_id: player.player_id,
      player_no: player.player_no,
      name: player.name,
      id_type: player.id_type,
      id_card_masked: player.id_card_no ? maskIdCard(player.id_card_no) : null,
      birth_date: player.birth_date,
      gender: player.gender,
      institution: player.institution,
      merged_into: player.merged_into,
      registration_count: myOrders.length,
      result_count: player.player_id <= 8 ? (player.player_id % 3) + 1 : 0,
      registrations: myOrders.map((o) => ({
        order_no: o.order_no,
        event_name: o.event_name,
        level: o.level,
        dance_type: o.dance_type,
        status: o.status,
        created_at: o.created_at,
      })),
      results_summary: player.player_id <= 8
        ? [{ award: '金奖', event_name: '国舞菁英2026全国少儿中国舞独舞冠军赛·10月超级赛', points: 360, created_at: '2026-10-06T18:00:00+08:00' }]
        : [],
      points_summary: {
        season_name: '2026 赛季',
        total_points: player.player_id <= 8 ? (player.player_id % 5) * 120 : 0,
        event_count: player.player_id <= 8 ? (player.player_id % 3) + 1 : 0,
      },
      duplicates: players
        .filter((p) => p.merged_into === player.player_id)
        .map((p) => ({ player_id: p.player_id, player_no: p.player_no, name: p.name, created_at: p.created_at })),
    });
  },

  'POST /api/v1/admin/players/:id/merge': (req: Request, res: Response) => {
    const player = players.find((p) => p.player_id === toInt(req.params.id));
    const target = players.find((p) => p.player_id === toInt(req.body?.target_player_id));
    if (!player || !target) return fail(res, 40401, '选手不存在');
    player.merged_into = target.player_id;
    return ok(res, { player_id: player.player_id, merged_into: target.player_id });
  },
};
