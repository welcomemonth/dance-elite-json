/**
 * 音频审核 H5 数据接口（免登录，URL token 鉴权）
 * 对应 docs/优舞盟报名功能/API接口设计.md 第 7 节
 */

export interface AudioReviewEntry {
  /** 选手姓名 */
  player_name: string;
  /** 节目名称 */
  program_name: string;
  /** 音频播放地址 */
  audio_url: string;
  /** 音频最近更新时间（ISO 8601 本地时间） */
  updated_at: string;
}

export interface AudioReviewData {
  /** 赛事名称 */
  event_name: string;
  /** 已上传音频的有效报名 */
  entries: AudioReviewEntry[];
}

interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

/**
 * GET /h5/audio-review/{token}
 * token 无效或赛事未生成时返回 40401
 */
export async function fetchAudioReview(token: string): Promise<AudioReviewData> {
  const res = await fetch(`/h5/audio-review/${encodeURIComponent(token)}`);
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`);
  }
  const body = (await res.json()) as ApiResponse<AudioReviewData>;
  if (body.code !== 0) {
    throw new Error(body.message || `code ${body.code}`);
  }
  return body.data;
}
