/**
 * 音频审核 H5 mock：GET /h5/audio-review/{token}
 * 有效 token：a8K2mP9x（赛事 1 已生成）；其余 → 40401。
 */
import type { Request, Response } from 'express';
import { events } from './_db';
import { fail, ok } from './_utils';

export default {
  'GET /h5/audio-review/:token': (req: Request, res: Response) => {
    const event = events.find((e) => e.audio_review_token === req.params.token);
    if (!event) return fail(res, 40401, '链接无效或赛事未生成审核链接');
    return ok(res, {
      event_name: event.name,
      entries: [
        { player_name: '张小雨', program_name: '春闺梦', audio_url: '/static/materials/5001/audio.mp3', updated_at: '2026-09-20T10:00:00+08:00' },
        { player_name: '李思远', program_name: '扇舞丹青', audio_url: '/static/materials/5002/audio.mp3', updated_at: '2026-09-21T11:00:00+08:00' },
        { player_name: 'Emma Wilson', program_name: '天鹅湖', audio_url: '/static/materials/5101/audio.mp3', updated_at: '2026-09-22T09:30:00+08:00' },
        { player_name: '王梦琪', program_name: '爱莲说', audio_url: '/static/materials/5003/audio.mp3', updated_at: '2026-09-23T15:00:00+08:00' },
        { player_name: '陈子涵', program_name: '罗敷行', audio_url: '/static/materials/5004/audio.mp3', updated_at: '2026-09-24T10:00:00+08:00' },
      ],
    });
  },
};
