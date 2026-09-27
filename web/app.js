(function () {
  'use strict';

  // ---- State ----
  var state = {
    audioList: [],      // [{ name, url }]
    searchKeyword: '',  // 当前搜索关键词
    currentAudio: null, // { name, url } 正在播放的音频
    viewMode: 'list',   // 'list' | 'player'
  };

  // ---- DOM ----
  var listView = document.getElementById('list-view');
  var playerView = document.getElementById('player-view');
  var searchInput = document.getElementById('search-input');
  var audioListEl = document.getElementById('audio-list');
  var listLoading = document.getElementById('list-loading');
  var listEmpty = document.getElementById('list-empty');
  var listNoMatch = document.getElementById('list-no-match');
  var listError = document.getElementById('list-error');
  var playerTitle = document.getElementById('player-title');
  var audioEl = document.getElementById('audio');
  var playerError = document.getElementById('player-error');
  var backBtn = document.getElementById('back-btn');

  // ---- Helpers ----

  // 关键词过滤：contains 匹配，英文不区分大小写，中文正常匹配
  function filterAudios() {
    var kw = state.searchKeyword.trim().toLowerCase();
    if (!kw) {
      return state.audioList.slice();
    }
    return state.audioList.filter(function (audio) {
      return audio.name.toLowerCase().indexOf(kw) !== -1;
    });
  }

  function hideAllListHints() {
    audioListEl.hidden = true;
    listLoading.hidden = true;
    listEmpty.hidden = true;
    listNoMatch.hidden = true;
    listError.hidden = true;
  }

  function renderList() {
    audioListEl.innerHTML = '';
    hideAllListHints();

    if (state.audioList.length === 0) {
      listEmpty.hidden = false;
      return;
    }

    var filtered = filterAudios();
    if (filtered.length === 0) {
      listNoMatch.hidden = false;
      return;
    }

    audioListEl.hidden = false;
    filtered.forEach(function (audio) {
      var li = document.createElement('li');
      li.className = 'audio-item';
      li.textContent = audio.name; // 用 textContent，避免文件名中的特殊字符注入 HTML
      li.addEventListener('click', function () {
        openPlayer(audio);
      });
      audioListEl.appendChild(li);
    });
  }

  function showListError() {
    audioListEl.innerHTML = '';
    hideAllListHints();
    listError.hidden = false;
  }

  function loadAudios() {
    fetch('/api/contest/audios')
      .then(function (res) {
        if (!res.ok) {
          throw new Error('HTTP ' + res.status);
        }
        return res.json();
      })
      .then(function (data) {
        state.audioList = Array.isArray(data) ? data : [];
        renderList();
      })
      .catch(function () {
        showListError();
      });
  }

  function showView() {
    if (state.viewMode === 'player') {
      listView.classList.add('hidden');
      playerView.classList.remove('hidden');
    } else {
      listView.classList.remove('hidden');
      playerView.classList.add('hidden');
    }
  }

  function showPlayerError(msg) {
    playerError.textContent = msg;
    playerError.hidden = false;
  }

  function hidePlayerError() {
    playerError.hidden = true;
  }

  // ---- View transitions ----

  function openPlayer(audio) {
    state.currentAudio = audio;
    state.viewMode = 'player';
    playerTitle.textContent = audio.name;
    hidePlayerError();

    // 重置播放器：从 00:00 开始，不自动播放
    audioEl.src = audio.url;
    audioEl.load();
    try {
      audioEl.currentTime = 0;
    } catch (e) {
      // 部分浏览器在 load 完成前设置 currentTime 会抛错，忽略即可
    }

    showView();
  }

  function backToList() {
    // 停止音频，重置播放器状态
    audioEl.pause();
    audioEl.removeAttribute('src');
    audioEl.load();
    hidePlayerError();

    state.currentAudio = null;
    state.viewMode = 'list';
    showView();

    // 搜索关键词保留在输入框中，renderList 按 state.searchKeyword 恢复搜索结果
    renderList();
  }

  // ---- Events ----

  searchInput.addEventListener('input', function () {
    state.searchKeyword = searchInput.value;
    renderList();
  });

  backBtn.addEventListener('click', backToList);

  // 音频加载 / 播放错误
  audioEl.addEventListener('error', function () {
    if (!state.currentAudio) {
      return;
    }
    var audio = state.currentAudio;

    // 用 HEAD 探测区分「文件不存在(404)」与「播放失败」
    fetch(audio.url, { method: 'HEAD' })
      .then(function (res) {
        if (state.currentAudio === audio) {
          showPlayerError(res.status === 404 ? '音频文件不存在' : '音频播放失败');
        }
      })
      .catch(function () {
        if (state.currentAudio === audio) {
          showPlayerError('音频播放失败');
        }
      });
  });

  // 音频能正常播放时清除错误提示
  audioEl.addEventListener('playing', hidePlayerError);

  // ---- Init ----
  showView();
  loadAudios();
})();
