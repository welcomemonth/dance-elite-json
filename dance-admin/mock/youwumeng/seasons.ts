/**
 * 榜单中心 mock（*待定*接口）：赛季/积分规则/成绩录入/榜单/晋级/机构榜/奖金。
 * 路径与 src/services/youwumeng/seasons.ts 一一对应。
 */
import type { Request, Response } from 'express';
import {
  events,
  finalsRoster,
  institutionRanking,
  leaderboardRows,
  players,
  prizeRules,
  prizeStatements,
  qualificationCards,
  scoreRules,
  seasons,
} from './_db';
import { fail, maskIdCard, ok, paginate, toInt } from './_utils';

export default {
  // ---------- 赛季 ----------
  'GET /api/v1/admin/seasons': (_req: Request, res: Response) => ok(res, seasons),

  'POST /api/v1/admin/seasons': (req: Request, res: Response) => {
    const { name, start_date, end_date } = req.body ?? {};
    if (!name || !start_date || !end_date) return fail(res, 40001, '参数缺失');
    const season_id = Math.max(...seasons.map((s) => s.season_id)) + 1;
    seasons.unshift({
      season_id, name, start_date, end_date,
      status: 'upcoming',
      boards: [
        { level: 'U11', dance_type: 'classical' }, { level: 'U11', dance_type: 'folk' },
        { level: 'U13', dance_type: 'classical' }, { level: 'U13', dance_type: 'folk' },
        { level: 'U15', dance_type: 'classical' }, { level: 'U15', dance_type: 'folk' },
      ],
      created_at: new Date().toISOString(),
    });
    return ok(res, { season_id });
  },

  'PUT /api/v1/admin/seasons/:id': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    Object.assign(season, req.body ?? {});
    return ok(res, { season_id: season.season_id });
  },

  'POST /api/v1/admin/seasons/:id/archive': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    season.status = 'archived';
    return ok(res, { season_id: season.season_id, status: 'archived' });
  },

  // ---------- 积分规则 ----------
  'GET /api/v1/admin/seasons/:id/score-rules': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, scoreRules);
  },

  'PUT /api/v1/admin/seasons/:id/score-rules': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    Object.assign(scoreRules, req.body ?? {});
    return ok(res, scoreRules);
  },

  // ---------- 成绩录入 ----------
  'GET /api/v1/admin/events/:id/scores': (req: Request, res: Response) => {
    const event = events.find((e) => e.event_id === toInt(req.params.id));
    if (!event) return fail(res, 40401, '赛事不存在');
    const rows = players.slice(0, 8).map((p, i) => ({
      entry_id: 7000 + i,
      player_id: p.player_id,
      player_no: p.player_no,
      player_name: p.name,
      id_card_masked: p.id_card_no ? maskIdCard(p.id_card_no) : null,
      award: i < 5 ? ['champion', 'runner_up', 'third_place', 'elite_dancer', 'special_gold'][i] : null,
      advanced_to_final: i < 3,
      points: i < 5 ? [120, 96, 72, 60, 48][i] : 0,
      remark: null,
      published: event.status === 'finished',
    }));
    return ok(res, rows);
  },

  'POST /api/v1/admin/events/:id/scores/preview': (req: Request, res: Response) => {
    const rows = Array.isArray(req.body?.rows) ? req.body.rows : [];
    if (rows.length === 0) return fail(res, 40001, '无成绩数据');
    const parsed = rows.map((r: { player_name?: string; id_card_no?: string }, i: number) => {
      const player = players.find((p) => p.id_card_no && p.id_card_no === r.id_card_no);
      return {
        row_no: i + 1,
        player_name: r.player_name ?? '',
        id_card_masked: r.id_card_no ? maskIdCard(r.id_card_no) : null,
        award: rows[i].award ?? null,
        advanced_to_final: rows[i].advanced_to_final ?? false,
        match_status: player ? 'matched' : 'unmatched',
        player_id: player?.player_id ?? null,
        remark: rows[i].remark ?? null,
      };
    });
    return ok(res, {
      total_rows: parsed.length,
      matched: parsed.filter((r: { match_status: string }) => r.match_status === 'matched').length,
      unmatched: parsed.filter((r: { match_status: string }) => r.match_status === 'unmatched').length,
      rows: parsed,
    });
  },

  'POST /api/v1/admin/events/:id/scores': (req: Request, res: Response) => {
    const rows = Array.isArray(req.body?.rows) ? req.body.rows : [];
    if (rows.length === 0) return fail(res, 40001, '无成绩数据');
    return ok(res, { published: rows.length });
  },

  'PUT /api/v1/admin/scores/:entryId': (req: Request, res: Response) => {
    const before = { award: 'gold', advanced_to_final: false };
    const after = {
      award: req.body?.award ?? before.award,
      advanced_to_final: req.body?.advanced_to_final ?? before.advanced_to_final,
    };
    return ok(res, { entry_id: toInt(req.params.entryId), diff: { before, after } });
  },

  // ---------- 积分与榜单 ----------
  'GET /api/v1/admin/seasons/:id/leaderboards': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, season.boards.map((b) => ({
      level: b.level,
      dance_type: b.dance_type,
      rows: leaderboardRows(b.level, b.dance_type),
      published_at: season.status === 'archived' ? '2026-12-31T10:00:00+08:00' : '2026-11-01T10:00:00+08:00',
    })));
  },

  'PUT /api/v1/admin/seasons/:id/leaderboards/:level/:danceType/players/:pid': (req: Request, res: Response) => {
    return ok(res, { player_id: toInt(req.params.pid) });
  },

  // ---------- 晋级管理 ----------
  'GET /api/v1/admin/seasons/:id/qualification-cards': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, qualificationCards);
  },

  'POST /api/v1/admin/seasons/:id/qualification-cards': (req: Request, res: Response) => {
    const { player_id, board, reason } = req.body ?? {};
    if (!player_id || !board || !reason) return fail(res, 40001, '增补直通卡需填写原因');
    const card_id = Math.max(...qualificationCards.map((c) => c.card_id)) + 1;
    const player = players.find((p) => p.player_id === toInt(player_id));
    qualificationCards.push({
      card_id,
      player_id: toInt(player_id),
      player_no: player?.player_no ?? '',
      player_name: player?.name ?? '',
      board,
      source: 'manual',
      reason,
      granted_at: new Date().toISOString(),
    });
    return ok(res, { card_id });
  },

  'DELETE /api/v1/admin/seasons/:id/qualification-cards/:cardId': (req: Request, res: Response) => {
    const idx = qualificationCards.findIndex((c) => c.card_id === toInt(req.params.cardId));
    if (idx < 0) return fail(res, 40401, '直通卡不存在');
    qualificationCards.splice(idx, 1);
    return ok(res, { card_id: toInt(req.params.cardId) });
  },

  'GET /api/v1/admin/seasons/:id/finals-roster': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, finalsRoster);
  },

  'POST /api/v1/admin/seasons/:id/finals-roster/adjust': (req: Request, res: Response) => {
    const { player_id, action, reason } = req.body ?? {};
    if (!player_id || !action || !reason) return fail(res, 40001, '调整需填写原因');
    return ok(res, { player_id: toInt(player_id) });
  },

  // ---------- 机构年度榜 ----------
  'GET /api/v1/admin/seasons/:id/institution-ranking': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, institutionRanking);
  },

  // ---------- 奖金管理 ----------
  'GET /api/v1/admin/seasons/:id/prize-rules': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    return ok(res, prizeRules);
  },

  'PUT /api/v1/admin/seasons/:id/prize-rules': (req: Request, res: Response) => {
    const season = seasons.find((s) => s.season_id === toInt(req.params.id));
    if (!season) return fail(res, 40401, '赛季不存在');
    Object.assign(prizeRules, req.body ?? {});
    return ok(res, prizeRules);
  },

  'GET /api/v1/admin/prize-statements': (req: Request, res: Response) => {
    let list = prizeStatements;
    if (req.query.event_id) list = list.filter((s) => s.event_id === toInt(req.query.event_id));
    if (req.query.paid === 'true') list = list.filter((s) => s.paid);
    if (req.query.paid === 'false') list = list.filter((s) => !s.paid);
    const page = paginate(req, list);
    return ok(res, {
      ...page,
      summary: {
        total_amount: list.reduce((s, x) => s + (x.adjusted_amount || x.amount), 0),
        paid_amount: list.filter((x) => x.paid).reduce((s, x) => s + (x.adjusted_amount || x.amount), 0),
        unpaid_count: list.filter((x) => !x.paid).length,
      },
    });
  },

  'PUT /api/v1/admin/prize-statements/:itemId': (req: Request, res: Response) => {
    const item = prizeStatements.find((s) => s.item_id === toInt(req.params.itemId));
    if (!item) return fail(res, 40401, '核算单不存在');
    item.adjusted_amount = toInt(req.body?.amount);
    item.adjust_reason = String(req.body?.reason ?? '');
    return ok(res, { item_id: item.item_id });
  },

  'POST /api/v1/admin/prize-statements/:itemId/pay': (req: Request, res: Response) => {
    const item = prizeStatements.find((s) => s.item_id === toInt(req.params.itemId));
    if (!item) return fail(res, 40401, '核算单不存在');
    if (!req.body?.payee) return fail(res, 40001, '请填写领取人/代领人');
    item.paid = true;
    item.paid_at = req.body.paid_at ?? new Date().toISOString();
    item.payee = req.body.payee;
    item.remark = req.body.remark ?? null;
    return ok(res, { item_id: item.item_id, paid: true });
  },
};
