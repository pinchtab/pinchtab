'use strict';

const screencastSockets = {};
let liveModalContext = null;

function closeScreencastSockets() {
  Object.values(screencastSockets).forEach((s) => s.close());
  Object.keys(screencastSockets).forEach((k) => delete screencastSockets[k]);
}

function getScreencastParams() {
  return '&quality=' + screencastSettings.quality + '&maxWidth=' + screencastSettings.maxWidth + '&fps=' + screencastSettings.fps;
}

function makeTileKey(tabId, instancePort) {
  const portPart = String(instancePort || 'main').replace(/[^a-zA-Z0-9_-]/g, '_');
  const tabPart = String(tabId || '').replace(/[^a-zA-Z0-9_-]/g, '_');
  return portPart + '_' + tabPart;
}

function connectScreencast(stream) {
  const tabId = stream.tabId;
  const baseUrl = stream.baseUrl || null;
  const key = stream.key;
  const wsProto = baseUrl ? 'ws:' : (location.protocol === 'https:' ? 'wss:' : 'ws:');
  const wsHost = baseUrl ? baseUrl.replace(/^https?:\/\//, '') : location.host;
  const wsUrl = wsProto + '//' + wsHost + '/screencast?tabId=' + encodeURIComponent(tabId) + getScreencastParams();

  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  screencastSockets[key] = socket;

  const canvas = document.getElementById('canvas-' + key);
  if (!canvas) return;
  const ctx2d = canvas.getContext('2d');

  let frameCount = 0;
  let lastFpsTime = Date.now();
  const statusEl = document.getElementById('status-' + key);
  const fpsEl = document.getElementById('fps-' + key);
  const sizeEl = document.getElementById('size-' + key);

  socket.onopen = () => {
    if (statusEl) statusEl.className = 'tile-status streaming';
  };
  socket.onmessage = (evt) => {
    const blob = new Blob([evt.data], { type: 'image/jpeg' });
    const url = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      ctx2d.drawImage(img, 0, 0);
      URL.revokeObjectURL(url);
    };
    img.src = url;
    frameCount++;
    const now = Date.now();
    if (now - lastFpsTime >= 1000) {
      if (fpsEl) fpsEl.textContent = frameCount + ' fps';
      if (sizeEl) sizeEl.textContent = (evt.data.byteLength / 1024).toFixed(0) + ' KB/frame';
      frameCount = 0;
      lastFpsTime = now;
    }
  };
  socket.onerror = () => {
    if (statusEl) statusEl.className = 'tile-status error';
  };
  socket.onclose = () => {
    if (statusEl) statusEl.className = 'tile-status error';
  };
}

function renderLiveEmpty(grid, messageHtml) {
  if (!grid) return;
  grid.classList.add('empty');
  grid.innerHTML = '<div class="empty-state"><img src="/dashboard/pinchtab-headed-192.png" alt="" class="empty-icon">' + messageHtml + '</div>';
}

function renderLiveTiles(grid, html) {
  if (!grid) return;
  grid.classList.remove('empty');
  grid.innerHTML = html;
}

function renderStreams(grid, streams) {
  renderLiveTiles(grid, streams.map((s) => `
      <div class="screen-tile" id="tile-${s.key}">
        <div class="tile-header">
          <span>
            <span class="tile-id">${esc(s.label)}</span>
            <span class="tile-status connecting" id="status-${s.key}"></span>
          </span>
          <span class="tile-url" id="url-${s.key}">${esc(s.url || 'about:blank')}</span>
        </div>
        <canvas id="canvas-${s.key}" width="800" height="600"></canvas>
        <div class="tile-footer">
          <span id="fps-${s.key}">—</span>
          <span id="size-${s.key}">—</span>
        </div>
      </div>
    `).join(''));
  streams.forEach((s) => connectScreencast(s));
}

function showLiveModal(title) {
  closeScreencastSockets();
  const body = `
    <div class="live-popup">
      <div class="live-toolbar">
        <button class="refresh-btn" id="live-refresh-btn">Refresh Tabs</button>
        <span id="live-tab-count" class="live-count">Loading...</span>
      </div>
      <div id="screencast-grid" class="screencast-grid empty">
        <div class="empty-state"><span class="spinner"></span>Loading live tabs...</div>
      </div>
    </div>
  `;
  showModal(title, body, '<button class="secondary" onclick="closeModal()">Close</button>', {
    wide: true,
    onClose: () => {
      closeScreencastSockets();
      liveModalContext = null;
    }
  });
  const refreshBtn = document.getElementById('live-refresh-btn');
  if (refreshBtn) refreshBtn.onclick = () => refreshLiveModal();
}

async function refreshLiveModal() {
  if (!liveModalContext) return;
  closeScreencastSockets();

  const grid = document.getElementById('screencast-grid');
  const countEl = document.getElementById('live-tab-count');
  if (!grid || !countEl) return;

  try {
    const result = await liveModalContext.load();
    if (!result.ok) {
      countEl.textContent = 'Unavailable';
      renderLiveEmpty(grid, 'Live view unavailable (' + result.status + ').');
      return;
    }
    countEl.textContent = result.countLabel;
    if (!Array.isArray(result.streams) || result.streams.length === 0) {
      renderLiveEmpty(grid, result.emptyMessage || 'No tabs open.');
      return;
    }
    renderStreams(grid, result.streams);
  } catch (e) {
    countEl.textContent = 'Retrying';
    renderLiveEmpty(grid, 'Live request failed.<br>Retrying...');
    console.error('Failed to load live tabs', e);
  }
}

async function loadMainStreams() {
  const res = await fetch('/screencast/tabs');
  if (res.status === 503) {
    return {
      ok: true,
      countLabel: 'No running instance',
      streams: [],
      emptyMessage: 'No running instance.'
    };
  }
  if (!res.ok) return { ok: false, status: res.status };
  const tabs = await res.json();
  if (!Array.isArray(tabs)) {
    return { ok: true, countLabel: '0 tab(s)', streams: [], emptyMessage: 'No tabs open.' };
  }
  return {
    ok: true,
    countLabel: tabs.length + ' tab(s)',
    streams: tabs.map((t) => ({
      key: makeTileKey(t.id, 'main'),
      tabId: t.id,
      baseUrl: null,
      label: String(t.id || '').substring(0, 8),
      url: t.url || 'about:blank',
    })),
    emptyMessage: 'No tabs open.'
  };
}

async function loadInstanceStreams(id, port) {
  const res = await fetch('/instances/tabs');
  if (res.status === 503) {
    return {
      ok: true,
      countLabel: 'No running instance',
      streams: [],
      emptyMessage: 'No running instance for this profile.'
    };
  }
  if (!res.ok) return { ok: false, status: res.status };

  const tabs = await res.json();
  const selectedTabs = Array.isArray(tabs)
    ? tabs.filter((t) => String(t.instancePort || '') === String(port || ''))
    : [];
  const streams = selectedTabs.map((t) => ({
    key: makeTileKey(t.tabId, t.instancePort),
    tabId: t.tabId,
    baseUrl: 'http://localhost:' + t.instancePort,
    label: (t.instanceName || 'instance') + ':' + String(t.tabId || '').substring(0, 6),
    url: t.url || 'about:blank',
  }));
  return {
    ok: true,
    countLabel: selectedTabs.length + ' tab(s) on ' + id,
    streams,
    emptyMessage: 'No tabs in this profile.'
  };
}

async function viewMainLive() {
  liveModalContext = {
    load: loadMainStreams,
  };
  showLiveModal('LIVE: Main');
  await refreshLiveModal();
}

async function viewInstanceLive(id, port) {
  liveModalContext = {
    load: () => loadInstanceStreams(id, port),
  };
  showLiveModal('LIVE: ' + id);
  await refreshLiveModal();
}

window.addEventListener('beforeunload', () => {
  liveModalContext = null;
  closeScreencastSockets();
});
