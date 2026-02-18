const MAX_EVENTS = 500;
const events = [];
const agents = {};
let selectedAgent = null;
let currentFilter = 'all';

// SSE connection
function connect() {
  const es = new EventSource('/dashboard/events');

  es.addEventListener('init', (e) => {
    const list = JSON.parse(e.data);
    list.forEach(a => { agents[a.agentId] = a; });
    renderAgents();
  });

  es.addEventListener('action', (e) => {
    const evt = JSON.parse(e.data);
    events.unshift(evt);
    if (events.length > MAX_EVENTS) events.pop();

    // Update agent state
    if (!agents[evt.agentId]) {
      agents[evt.agentId] = { agentId: evt.agentId, actionCount: 0, status: 'active' };
    }
    const a = agents[evt.agentId];
    a.lastAction = evt.action;
    a.lastSeen = evt.timestamp;
    a.currentUrl = evt.url || a.currentUrl;
    a.currentTab = evt.tabId || a.currentTab;
    a.profile = evt.profile || a.profile;
    a.status = 'active';
    a.actionCount++;

    renderAgents();
    renderFeed();
  });

  es.onerror = () => {
    es.close();
    setTimeout(connect, 3000);
  };
}

function renderAgents() {
  const el = document.getElementById('agents-list');
  const ids = Object.keys(agents);
  if (ids.length === 0) return;

  el.innerHTML = ids.map(id => {
    const a = agents[id];
    const sel = selectedAgent === id ? 'selected' : '';
    const ago = a.lastSeen ? timeAgo(new Date(a.lastSeen)) : '—';
    return `
      <div class="agent-card ${sel}" onclick="selectAgent('${id}')">
        <div class="agent-header">
          <span class="agent-name">${esc(id)}</span>
          <span class="agent-status ${a.status}">${a.status}</span>
        </div>
        <div class="agent-url">${esc(a.currentUrl || 'No URL yet')}</div>
        <div class="agent-meta">
          <span>${a.profile ? '📁 ' + esc(a.profile) : ''}</span>
          <span>📊 ${a.actionCount} actions</span>
          <span>${ago}</span>
        </div>
      </div>
    `;
  }).join('');
}

function renderFeed() {
  const el = document.getElementById('feed-list');
  let filtered = events;

  if (selectedAgent) {
    filtered = filtered.filter(e => e.agentId === selectedAgent);
  }
  if (currentFilter !== 'all') {
    filtered = filtered.filter(e => e.action.toLowerCase().includes(currentFilter));
  }

  if (filtered.length === 0) {
    el.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No matching events.</div>';
    return;
  }

  el.innerHTML = filtered.slice(0, 200).map(evt => {
    const parts = evt.action.split(' ');
    const method = parts[0] || '';
    const path = parts.slice(1).join(' ');
    const statusClass = evt.status < 400 ? 'ok' : 'err';
    const detail = evt.detail || evt.url || '';
    const t = new Date(evt.timestamp);
    const time = t.toLocaleTimeString();

    return `
      <div class="event-row">
        <span class="time">${time}</span>
        <span class="agent">${esc(evt.agentId)}</span>
        <span class="action-detail">
          <span class="method ${method}">${method}</span>
          ${esc(path)}${detail ? ' <span style="color:#666">— ' + esc(detail) + '</span>' : ''}
        </span>
        <span class="duration">${evt.durationMs}ms</span>
        <span class="status-code ${statusClass}">${evt.status}</span>
      </div>
    `;
  }).join('');
}

function selectAgent(id) {
  selectedAgent = selectedAgent === id ? null : id;
  renderAgents();
  renderFeed();
}

// Filters
document.querySelectorAll('.filter-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    currentFilter = btn.dataset.filter;
    renderFeed();
  });
});

function closeModal() {
  document.getElementById('modal').classList.remove('open');
}

async function resetProfile(name) {
  if (!await appConfirm('Reset profile "' + name + '"? This clears session data, cookies, and cache.', '🔄 Reset Profile')) return;
  await fetch('/profiles/' + name + '/reset', { method: 'POST' });
  closeModal();
  loadProfiles();
}

async function viewAnalytics(name) {
  const modal = document.getElementById('modal');
  const title = document.getElementById('modal-title');
  const body = document.getElementById('modal-body');
  title.textContent = '📊 Analytics: ' + name;

  // Try to get analytics data
  let analytics = null;
  try {
    const res = await fetch('/profiles/' + name + '/analytics');
    if (res.ok) analytics = await res.json();
  } catch(e) {}

  // Get live server stats
  let tabs = [], agents = [];
  try {
    const tabsRes = await fetch('/tabs');
    const tabsData = await tabsRes.json();
    tabs = tabsData.tabs || [];
  } catch(e) {}
  try {
    const agentsRes = await fetch('/dashboard/agents');
    agents = await agentsRes.json() || [];
  } catch(e) {}

  let html = '';

  // Live stats
  html += '<h4 style="color:#888;font-size:12px;margin-bottom:8px">LIVE STATUS</h4>';
  html += '<div style="font-size:13px;color:#aaa;margin-bottom:4px">Tabs open: <span style="color:#e0e0e0">' + tabs.length + '</span></div>';
  html += '<div style="font-size:13px;color:#aaa;margin-bottom:12px">Agents seen: <span style="color:#e0e0e0">' + agents.length + '</span></div>';

  // Agent breakdown
  if (agents.length > 0) {
    html += '<h4 style="color:#888;font-size:12px;margin-bottom:8px">AGENTS</h4>';
    agents.forEach(a => {
      html += '<div style="font-size:12px;color:#aaa;padding:3px 0;display:flex;justify-content:space-between">';
      html += '<span style="color:#f5c542;font-weight:600">' + esc(a.agentId) + '</span>';
      html += '<span>' + a.actionCount + ' actions — ' + esc(a.lastAction || '') + '</span>';
      html += '</div>';
    });
    html += '<div style="margin-bottom:12px"></div>';
  }

  // Tracked analytics if available
  if (analytics && analytics.totalActions > 0) {
    html += '<h4 style="color:#888;font-size:12px;margin-bottom:8px">TRACKED (' + analytics.totalActions + ' actions)</h4>';
    if (analytics.topEndpoints) {
      analytics.topEndpoints.forEach(e => {
        html += '<div style="font-size:12px;color:#aaa;padding:2px 0">' + esc(e.endpoint) + ' — ' + e.count + 'x, avg ' + e.avgMs + 'ms</div>';
      });
    }
    if (analytics.suggestions) {
      html += '<div style="margin-top:8px">';
      analytics.suggestions.forEach(s => {
        html += '<p style="color:#f5c542;font-size:12px;margin-bottom:4px">💡 ' + esc(s) + '</p>';
      });
      html += '</div>';
    }
  } else {
    html += '<p style="color:#555;font-size:12px">No tracked actions yet. Agents need to send <code style="color:#888">X-Profile</code> header to track per-profile analytics.</p>';
  }

  html += '<div class="btn-row" style="margin-top:16px"><button class="secondary" onclick="closeModal()">Close</button></div>';
  body.innerHTML = html;
  modal.classList.add('open');
}

document.getElementById('modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) closeModal();
});

function esc(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

function appConfirm(message, title, isDanger) {
  return new Promise((resolve) => {
    document.getElementById('confirm-title').textContent = title || 'Confirm';
    document.getElementById('confirm-message').textContent = message;
    const okBtn = document.getElementById('confirm-ok');
    okBtn.textContent = 'Confirm';
    okBtn.style.display = '';
    okBtn.className = isDanger !== false ? 'danger' : '';
    document.getElementById('confirm-cancel').textContent = 'Cancel';
    document.getElementById('confirm-modal').classList.add('open');
    const cleanup = () => { document.getElementById('confirm-modal').classList.remove('open'); };
    okBtn.onclick = () => { cleanup(); resolve(true); };
    document.getElementById('confirm-cancel').onclick = () => { cleanup(); resolve(false); };
  });
}
function appAlert(message, title) {
  return new Promise((resolve) => {
    document.getElementById('confirm-title').textContent = title || 'Notice';
    document.getElementById('confirm-message').textContent = message;
    document.getElementById('confirm-ok').style.display = 'none';
    document.getElementById('confirm-cancel').textContent = 'OK';
    document.getElementById('confirm-modal').classList.add('open');
    document.getElementById('confirm-cancel').onclick = () => {
      document.getElementById('confirm-modal').classList.remove('open');
      resolve();
    };
  });
}
function timeAgo(d) {
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (s < 5) return 'just now';
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s/60) + 'm ago';
  return Math.floor(s/3600) + 'h ago';
}

connect();

// ---------------------------------------------------------------------------
// View switching
// ---------------------------------------------------------------------------
let profilesInterval = null;
function switchView(view) {
  document.querySelectorAll('.view-btn').forEach(b => b.classList.remove('active'));
  document.querySelector('[data-view="'+view+'"]').classList.add('active');
  document.getElementById('feed-view').style.display = view === 'feed' ? 'flex' : 'none';
  document.getElementById('profiles-view').style.display = view === 'profiles' ? 'flex' : 'none';
  document.getElementById('live-view').style.display = view === 'live' ? 'flex' : 'none';
  document.getElementById('settings-view').style.display = view === 'settings' ? 'block' : 'none';
  if (view === 'live') refreshTabs();
  if (view === 'profiles') loadProfiles();
  if (view === 'settings') loadSettings();
  // Auto-refresh profiles every 3s while on that view
  if (profilesInterval) { clearInterval(profilesInterval); profilesInterval = null; }
  if (view === 'profiles') {
    profilesInterval = setInterval(loadProfiles, 3000);
  }
}

// ---------------------------------------------------------------------------
// Instances
// ---------------------------------------------------------------------------
async function loadProfiles() {
  try {
    // Fetch profiles, instances, and main server info
    const [profRes, instRes, healthRes, tabsRes] = await Promise.all([
      fetch('/profiles'),
      fetch('/instances'),
      fetch('/health'),
      fetch('/tabs')
    ]);
    const profiles = await profRes.json() || [];
    const instances = await instRes.json() || [];
    const health = await healthRes.json();
    const tabsData = await tabsRes.json();

    // Map running instances by profile name
    const running = {};
    instances.forEach(inst => { if (inst.status === 'running') running[inst.name] = inst; });

    const profileNames = new Set(profiles.map(p => p.name));
    const extraInstances = instances.filter(i => !profileNames.has(i.name));

    const grid = document.getElementById('profiles-grid');
    const cards = [];

    // Main instance card (always first)
    cards.push(renderMainCard(tabsData.tabs ? tabsData.tabs.length : 0));

    // Profile cards
    profiles.forEach(p => {
      cards.push(renderProfileCard(p.name, p.sizeMB, p.source, running[p.name] || null));
    });

    extraInstances.forEach(inst => {
      cards.push(renderProfileCard(inst.name, 0, 'instance', inst.status === 'running' ? inst : null));
    });

    grid.innerHTML = cards.join('');
  } catch (e) {
    console.error('Failed to load profiles', e);
  }
}

function renderMainCard(tabCount) {
  return `
    <div class="inst-card" style="border-color:#f5c542">
      <div class="inst-header">
        <span class="inst-name">🦀 Main</span>
        <span class="inst-badge running">running :${location.port || '9867'}</span>
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">Tabs</span><span class="value">${tabCount}</span></div>
        <div class="inst-row"><span class="label">Port</span><span class="value">${location.port || '9867'}</span></div>
      </div>
      <div class="inst-actions">
        <button onclick="switchView('live')">📺 Live</button>
        <button onclick="resetProfile('main')">🔄 Reset</button>
        <button onclick="viewAnalytics('main')">📊 Analytics</button>
      </div>
    </div>
  `;
}

function renderProfileCard(name, sizeMB, source, inst) {
  const isRunning = inst && inst.status === 'running';
  const statusBadge = isRunning
    ? '<span class="inst-badge running">running :' + inst.port + '</span>'
    : '<span class="inst-badge stopped">stopped</span>';

  return `
    <div class="inst-card">
      <div class="inst-header">
        <span class="inst-name">${esc(name)}</span>
        ${statusBadge}
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">Size</span><span class="value">${sizeMB ? sizeMB.toFixed(0) + ' MB' : '—'}</span></div>
        <div class="inst-row"><span class="label">Source</span><span class="value">${esc(source)}</span></div>
        ${isRunning ? '<div class="inst-row"><span class="label">Mode</span><span class="value">' + (inst.headless ? '🔲 Headless' : '🖥️ Headed') + '</span></div>' : ''}
        ${isRunning ? '<div class="inst-row"><span class="label">Tabs</span><span class="value">' + inst.tabCount + '</span></div>' : ''}
        ${isRunning ? '<div class="inst-row"><span class="label">PID</span><span class="value">' + inst.pid + '</span></div>' : ''}
      </div>
      <div class="inst-actions">
        ${isRunning
          ? '<button onclick="viewInstanceLive(\'' + esc(inst.id) + '\', \'' + esc(inst.port) + '\')">📺 Live</button>'
            + '<button onclick="viewInstanceLogs(\'' + esc(inst.id) + '\')">📄 Logs</button>'
            + '<button onclick="resetProfile(\'' + esc(name) + '\')">🔄 Reset</button>'
            + '<button class="danger" onclick="stopInstance(\'' + esc(inst.id) + '\')">⏹ Stop</button>'
          : (getProfileHeadless(name)
              ? '<button onclick="launchProfile(\'' + esc(name) + '\')">🚀 Launch</button>'
                + '<button onclick="launchHeaded(\'' + esc(name) + '\')">🖥️ Headed</button>'
              : '<button onclick="launchHeaded(\'' + esc(name) + '\')">🖥️ Launch</button>'
                + '<button onclick="launchProfile(\'' + esc(name) + '\')">🚀 Headless</button>'
            )
            + '<button onclick="resetProfile(\'' + esc(name) + '\')">🔄 Reset</button>'
            + '<button onclick="viewAnalytics(\'' + esc(name) + '\')">📊 Analytics</button>'
            + '<button class="danger" onclick="deleteProfile(\'' + esc(name) + '\')">🗑️ Delete</button>'
        }
      </div>
    </div>
  `;
}

function showCreateProfileModal() {
  document.getElementById('create-profile-modal').classList.add('open');
  document.getElementById('create-name').focus();
}
function closeCreateProfileModal() {
  document.getElementById('create-profile-modal').classList.remove('open');
}

async function doCreateProfile() {
  const name = document.getElementById('create-name').value.trim();
  const source = document.getElementById('create-source').value.trim();

  if (!name) { await appAlert('Name required'); return; }
  closeCreateProfileModal();

  try {
    if (source) {
      await fetch('/profiles/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, source })
      });
    } else {
      await fetch('/profiles/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
      });
    }
    loadProfiles();
  } catch (e) { await appAlert('Failed: ' + e.message, 'Error'); }
}

function getProfilePort(name) {
  const saved = localStorage.getItem('pinchtab-port-' + name);
  if (saved) return saved;
  // Auto-assign based on profile index: 9868, 9869, ...
  const cards = document.querySelectorAll('#profiles-grid .inst-card');
  let idx = 0;
  cards.forEach((c, i) => { if (c.querySelector('.inst-name')?.textContent === name) idx = i; });
  return String(9867 + Math.max(idx, 1));
}
function saveProfilePort(name, port, headless) {
  localStorage.setItem('pinchtab-port-' + name, port);
  localStorage.setItem('pinchtab-headless-' + name, headless ? '1' : '0');
}
function getProfileHeadless(name) {
  const saved = localStorage.getItem('pinchtab-headless-' + name);
  if (saved !== null) return saved === '1';
  return true; // default headless
}
function openLaunchModal(name, headless) {
  document.getElementById('launch-name').value = name;
  document.getElementById('launch-port').value = getProfilePort(name);
  document.getElementById('launch-headless').checked = headless;
  document.getElementById('launch-modal').classList.add('open');
  document.getElementById('launch-port').focus();
}
function launchProfile(name) { openLaunchModal(name, true); }
function launchHeaded(name) { openLaunchModal(name, false); }
function closeLaunchModal() {
  document.getElementById('launch-modal').classList.remove('open');
}

async function doLaunch() {
  const name = document.getElementById('launch-name').value.trim();
  const port = document.getElementById('launch-port').value.trim();
  const headless = document.getElementById('launch-headless').checked;

  if (!name || !port) { await appAlert('Port required'); return; }

  saveProfilePort(name, port, headless);
  closeLaunchModal();

  try {
    const res = await fetch('/instances/launch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, port, headless })
    });
    const data = await res.json();
    if (!res.ok) {
      await appAlert('Launch failed: ' + (data.error || 'unknown'), 'Error');
      return;
    }
    // Poll until running
    pollInstanceStatus(data.id);
  } catch (e) {
    await appAlert('Launch error: ' + e.message, 'Error');
  }
}

async function deleteProfile(name) {
  if (!await appConfirm('Delete profile "' + name + '"? This removes all data.', '🗑️ Delete Profile')) return;
  await fetch('/profiles/' + name, { method: 'DELETE' });
  loadProfiles();
}

function pollInstanceStatus(id) {
  let attempts = 0;
  const poll = setInterval(async () => {
    attempts++;
    await loadProfiles();
    if (attempts > 30) clearInterval(poll);
    try {
      const res = await fetch('/instances');
      const instances = await res.json();
      const inst = instances.find(i => i.id === id);
      if (inst && (inst.status === 'running' || inst.status === 'error' || inst.status === 'stopped')) {
        clearInterval(poll);
        loadProfiles();
      }
    } catch(e) { clearInterval(poll); }
  }, 1000);
}

async function stopInstance(id) {
  if (!await appConfirm('Stop instance ' + id + '?', '⏹ Stop Instance')) return;
  await fetch('/instances/' + id + '/stop', { method: 'POST' });
  setTimeout(loadProfiles, 1000);
}

async function viewInstanceLogs(id) {
  const res = await fetch('/instances/' + id + '/logs');
  const text = await res.text();
  const modal = document.getElementById('modal');
  const title = document.getElementById('modal-title');
  const body = document.getElementById('modal-body');
  title.textContent = 'Logs: ' + id;
  body.innerHTML = '<pre style="background:#0a0a0a;padding:12px;border-radius:6px;font-size:11px;max-height:400px;overflow:auto;color:#aaa;white-space:pre-wrap">' + esc(text) + '</pre><div class="btn-row" style="margin-top:12px"><button class="secondary" onclick="closeModal()">Close</button></div>';
  modal.classList.add('open');
}

async function viewInstanceLive(id, port) {
  // Switch to live view and load tabs from that instance
  switchView('live');
  try {
    const res = await fetch('/instances/tabs');
    const tabs = await res.json();
    const instTabs = tabs.filter(t => t.instancePort === port);
    const grid = document.getElementById('screencast-grid');
    document.getElementById('live-tab-count').textContent = instTabs.length + ' tab(s) on ' + id;

    if (instTabs.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No tabs in this instance.</div>';
      return;
    }

    grid.innerHTML = instTabs.map(t => `
      <div class="screen-tile" id="tile-${t.tabId}">
        <div class="tile-header">
          <span>
            <span class="tile-id">${esc(t.instanceName)}:${t.tabId.substring(0, 6)}</span>
            <span class="tile-status connecting" id="status-${t.tabId}"></span>
          </span>
          <span class="tile-url" id="url-${t.tabId}">${esc(t.url || 'about:blank')}</span>
        </div>
        <canvas id="canvas-${t.tabId}" width="800" height="600"></canvas>
        <div class="tile-footer">
          <span id="fps-${t.tabId}">—</span>
          <span id="size-${t.tabId}">—</span>
        </div>
      </div>
    `).join('');

    // Connect screencast directly to child instance
    instTabs.forEach(t => {
      connectScreencast(t.tabId, 'http://localhost:' + t.instancePort);
    });
  } catch (e) {
    console.error('Failed to load instance tabs', e);
  }
}

// Unified screencast connector — works for both main and child instances
function connectScreencast(tabId, baseUrl) {
  const wsProto = baseUrl ? 'ws:' : (location.protocol === 'https:' ? 'wss:' : 'ws:');
  const wsHost = baseUrl ? baseUrl.replace(/^https?:\/\//, '') : location.host;
  const wsUrl = wsProto + '//' + wsHost + '/screencast?tabId=' + tabId + getScreencastParams();

  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  screencastSockets[tabId] = socket;

  const canvas = document.getElementById('canvas-' + tabId);
  if (!canvas) return;
  const ctx2d = canvas.getContext('2d');

  let frameCount = 0;
  let lastFpsTime = Date.now();
  const statusEl = document.getElementById('status-' + tabId);
  const fpsEl = document.getElementById('fps-' + tabId);
  const sizeEl = document.getElementById('size-' + tabId);

  socket.onopen = () => { if (statusEl) statusEl.className = 'tile-status streaming'; };
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
  socket.onerror = () => { if (statusEl) statusEl.className = 'tile-status error'; };
  socket.onclose = () => { if (statusEl) statusEl.className = 'tile-status error'; };
}

function openInstanceDirect(port) {
  window.open('http://localhost:' + port + '/dashboard', '_blank');
}

// ---------------------------------------------------------------------------
// Screencast
// ---------------------------------------------------------------------------
const screencastSockets = {};

async function refreshTabs() {
  // Clean up existing connections
  Object.values(screencastSockets).forEach(s => s.close());
  Object.keys(screencastSockets).forEach(k => delete screencastSockets[k]);

  try {
    const res = await fetch('/screencast/tabs');
    const tabs = await res.json();
    const grid = document.getElementById('screencast-grid');
    document.getElementById('live-tab-count').textContent = tabs.length + ' tab(s)';

    if (tabs.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No tabs open.</div>';
      return;
    }

    grid.innerHTML = tabs.map(t => `
      <div class="screen-tile" id="tile-${t.id}">
        <div class="tile-header">
          <span>
            <span class="tile-id">${t.id.substring(0, 8)}</span>
            <span class="tile-status connecting" id="status-${t.id}"></span>
          </span>
          <span class="tile-url" id="url-${t.id}">${esc(t.url || 'about:blank')}</span>
        </div>
        <canvas id="canvas-${t.id}" width="800" height="600"></canvas>
        <div class="tile-footer">
          <span id="fps-${t.id}">—</span>
          <span id="size-${t.id}">—</span>
        </div>
      </div>
    `).join('');

    // Start screencast for each tab
    tabs.forEach(t => startScreencast(t.id));
  } catch (e) {
    console.error('Failed to load tabs', e);
  }
}

// startScreencast for main instance tabs (uses current host)
function startScreencast(tabId) { connectScreencast(tabId, null); }

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
  Object.values(screencastSockets).forEach(s => s.close());
});

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------
const screencastSettings = { fps: 1, quality: 30, maxWidth: 800 };

async function loadSettings() {
  // Load stealth status
  try {
    const res = await fetch('/stealth/status');
    const data = await res.json();
    document.getElementById('set-stealth').value = data.level || 'light';
    updateStealthInfo(data);
  } catch(e) {}

  // Load server health info
  try {
    const res = await fetch('/health');
    const data = await res.json();
    const tabs = await fetch('/tabs').then(r => r.json());
    document.getElementById('server-info').innerHTML = `
      <div class="info-row"><span class="info-label">Status</span><span class="info-val">${data.status}</span></div>
      <div class="info-row"><span class="info-label">Tabs</span><span class="info-val">${tabs.tabs ? tabs.tabs.length : 0}</span></div>
      <div class="info-row"><span class="info-label">CDP</span><span class="info-val">${data.cdp || 'embedded'}</span></div>
      <div class="info-row"><span class="info-label">Port</span><span class="info-val">${location.port || '80'}</span></div>
    `;
  } catch(e) {
    document.getElementById('server-info').textContent = 'Failed to load server info';
  }

  // Restore saved settings from localStorage
  const saved = JSON.parse(localStorage.getItem('pinchtab-settings') || '{}');
  if (saved.fps) { document.getElementById('set-fps').value = saved.fps; document.getElementById('fps-val').textContent = saved.fps + ' fps'; screencastSettings.fps = saved.fps; }
  if (saved.quality) { document.getElementById('set-quality').value = saved.quality; document.getElementById('quality-val').textContent = saved.quality + '%'; screencastSettings.quality = saved.quality; }
  if (saved.maxWidth) { document.getElementById('set-maxwidth').value = saved.maxWidth; screencastSettings.maxWidth = saved.maxWidth; }

  // Toggle labels
  document.querySelectorAll('.toggle input').forEach(cb => {
    cb.addEventListener('change', () => {
      cb.parentElement.querySelector('.toggle-label').textContent = cb.checked ? 'On' : 'Off';
    });
  });
}

function updateStealthInfo(data) {
  const el = document.getElementById('stealth-info');
  if (!data || !data.level) { el.textContent = ''; return; }
  const tips = {
    light: 'Patches webdriver, CDP markers, plugins, languages, permissions. Works with X.com and Gmail.',
    full: 'Adds canvas noise, WebGL vendor spoofing, font metrics randomization. May break some sites (e.g. X.com crypto).'
  };
  el.textContent = tips[data.level] || '';
}

async function applySettings() {
  screencastSettings.fps = parseInt(document.getElementById('set-fps').value);
  screencastSettings.quality = parseInt(document.getElementById('set-quality').value);
  screencastSettings.maxWidth = parseInt(document.getElementById('set-maxwidth').value);

  // Save to localStorage
  localStorage.setItem('pinchtab-settings', JSON.stringify(screencastSettings));

  // Reconnect all screencasts with new settings
  Object.values(screencastSockets).forEach(s => s.close());
  Object.keys(screencastSockets).forEach(k => delete screencastSockets[k]);

  await appAlert('Settings saved. Switch to Live view to see changes.', '⚙️ Settings');
}

function getScreencastParams() {
  return '&quality=' + screencastSettings.quality + '&maxWidth=' + screencastSettings.maxWidth + '&fps=' + screencastSettings.fps;
}

async function applyStealth() {
  // Stealth level change would need a restart — just inform the user
  const level = document.getElementById('set-stealth').value;
  updateStealthInfo({ level });
  await appAlert('Stealth level change requires restarting Pinchtab with BRIDGE_STEALTH=' + level, '🛡️ Stealth');
}
