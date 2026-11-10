import { request } from '@umijs/max';
import type { PagedList, PlayerBrief, PlayerDetail, WxUser } from './typings';

export interface UserListParams {
  keyword?: string;
  status?: 'active' | 'blocked';
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/users（微信维度） */
export async function listUsers(params: UserListParams): Promise<PagedList<WxUser>> {
  return request('/api/v1/admin/users', { method: 'GET', params });
}

export interface PlayerListParams {
  keyword?: string;
  page?: number;
  page_size?: number;
}

/** GET /api/v1/admin/players（证件号维度聚合） */
export async function listPlayers(params: PlayerListParams): Promise<PagedList<PlayerBrief>> {
  return request('/api/v1/admin/players', { method: 'GET', params });
}

/** GET /api/v1/admin/players/{player_id} */
export async function getPlayer(playerId: number): Promise<PlayerDetail> {
  return request(`/api/v1/admin/players/${playerId}`, { method: 'GET' });
}

/** POST /api/v1/admin/players/{player_id}/merge（*待定*：合并重复档案，敏感操作留痕） */
export async function mergePlayer(
  playerId: number,
  targetPlayerId: number,
): Promise<{ player_id: number; merged_into: number }> {
  return request(`/api/v1/admin/players/${playerId}/merge`, {
    method: 'POST',
    data: { target_player_id: targetPlayerId },
  });
}
