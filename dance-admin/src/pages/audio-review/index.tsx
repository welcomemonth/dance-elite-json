import { useParams } from '@umijs/max';
import React, { useEffect, useMemo, useState } from 'react';
import { fetchAudioReview, type AudioReviewEntry } from './service';
import './style.css';

/** 列表条目展示名：姓名 · 节目名 */
function displayName(entry: AudioReviewEntry): string {
  return entry.program_name
    ? `${entry.player_name} · ${entry.program_name}`
    : entry.player_name;
}

/** 从音频 URL 提取扩展名（如 MP3/WAV），用于列表徽章展示 */
function extOf(url: string): string {
  const m = url.match(/\.(mp3|wav)$/i);
  return m ? m[1].toUpperCase() : '';
}

const AudioReviewPage: React.FC = () => {
  const { token } = useParams<{ token: string }>();

  const [entries, setEntries] = useState<AudioReviewEntry[]>([]);
  const [eventName, setEventName] = useState('');
  const [loadState, setLoadState] = useState<'loading' | 'error' | 'done'>('loading');
  const [keyword, setKeyword] = useState('');
  const [current, setCurrent] = useState<AudioReviewEntry | null>(null);
  const [playerError, setPlayerError] = useState('');

  useEffect(() => {
    if (!token) {
      setLoadState('error');
      return;
    }
    let cancelled = false;
    fetchAudioReview(token)
      .then((data) => {
        if (cancelled) return;
        setEventName(data.event_name);
        setEntries(data.entries);
        setLoadState('done');
        document.title = `伴奏校对 | ${data.event_name}`;
      })
      .catch(() => {
        if (!cancelled) {
          setLoadState('error');
        }
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  // 关键词过滤：contains 匹配，英文不区分大小写，中文正常匹配
  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    if (!kw) return entries;
    return entries.filter((entry) => displayName(entry).toLowerCase().includes(kw));
  }, [entries, keyword]);

  const backToList = () => {
    setCurrent(null);
    setPlayerError('');
  };

  // 音频加载 / 播放错误：用 HEAD 探测区分「文件不存在(404)」与「播放失败」
  const handleAudioError = (entry: AudioReviewEntry) => {
    fetch(entry.audio_url, { method: 'HEAD' })
      .then((res) => {
        setPlayerError(res.status === 404 ? '音频文件不存在' : '音频播放失败');
      })
      .catch(() => setPlayerError('音频播放失败'));
  };

  const renderListView = () => (
    <div className="audio-review-page">
      <header className="ar-cover">
        <img src="/cover.jpg" alt="顶部图片" />
      </header>

      <div className="ar-search-bar">
        <input
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder="搜索选手姓名或节目名..."
          autoComplete="off"
          spellCheck={false}
        />
      </div>

      {loadState === 'loading' && <div className="ar-hint">加载中...</div>}

      {loadState === 'error' && <div className="ar-hint">加载失败，请刷新重试</div>}

      {loadState === 'done' && entries.length === 0 && (
        <div className="ar-hint">暂无音频</div>
      )}

      {loadState === 'done' && entries.length > 0 && filtered.length === 0 && (
        <div className="ar-hint">暂无匹配</div>
      )}

      {loadState === 'done' && filtered.length > 0 && (
        <>
          <ul className="ar-audio-list">
            {filtered.map((entry) => (
              <li
                key={entry.audio_url}
                className="ar-audio-item"
                role="button"
                tabIndex={0}
                onClick={() => {
                  setPlayerError('');
                  setCurrent(entry);
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setPlayerError('');
                    setCurrent(entry);
                  }
                }}
              >
                <span className="ar-audio-item-icon" aria-hidden="true">
                  <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
                    <path d="M8 5v14l11-7z" />
                  </svg>
                </span>
                <span className="ar-audio-item-name">{displayName(entry)}</span>
                {extOf(entry.audio_url) && (
                  <span className="ar-audio-item-badge">{extOf(entry.audio_url)}</span>
                )}
              </li>
            ))}
          </ul>
          <div className="ar-list-count">
            {keyword.trim() ? `匹配 ${filtered.length} 首` : `共 ${filtered.length} 首 · 点击即可播放`}
          </div>
        </>
      )}
    </div>
  );

  const renderPlayerView = () => (
    <div className="audio-review-page">
      <header className="ar-cover">
        <img src="/cover.jpg" alt="顶部图片" />
      </header>

      <div className="ar-player-body">
        <div className="ar-song-name">{displayName(current!)}</div>

        <audio
          controls
          preload="metadata"
          src={current!.audio_url}
          onError={() => handleAudioError(current!)}
          onPlaying={() => setPlayerError('')}
        />

        {playerError && <div className="ar-hint ar-player-error">{playerError}</div>}

        <button type="button" className="ar-back-btn" onClick={backToList}>
          ← 返回列表
        </button>
      </div>
    </div>
  );

  return current ? renderPlayerView() : renderListView();
};

export default AudioReviewPage;
