'use strict';

// ---------------------------------------------------------------------------
// Profile management — cards, launch, create, delete
// ---------------------------------------------------------------------------

let profilesLoading = false;

async function loadProfiles() {
  if (profilesLoading) return; // debounce concurrent calls
  profilesLoading = true;

  const grid = document.getElementById('profiles-grid');
  // Show spinner only on first load (when grid is empty or has placeholder)
  if (!grid.querySelector('.inst-card')) {
    grid.innerHTML = '<div class="loading-overlay"><span class="spinner"></span>Loading profiles...</div>';
  }

  try {
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

    const running = {};
    instances.forEach(inst => { if (inst.status === 'running') running[inst.name] = inst; });

    const profileNames = new Set(profiles.map(p => p.name));
    const extraInstances = instances.filter(i => !profileNames.has(i.name));

    const grid = document.getElementById('profiles-grid');
    const cards = [];

    // Only show Main card if we're running in embedded mode (bridge serves /health with no "mode")
    if (!health.mode) {
      cards.push(renderMainCard(tabsData.tabs ? tabsData.tabs.length : 0));
    }

    profiles.forEach(p => {
      cards.push(renderProfileCard(p.name, p.sizeMB, p.source, running[p.name] || null));
    });

    extraInstances.forEach(inst => {
      cards.push(renderProfileCard(inst.name, 0, 'instance', inst.status === 'running' ? inst : null));
    });

    grid.innerHTML = cards.join('');
  } catch (e) {
    console.error('Failed to load profiles', e);
    grid.innerHTML = '<div class="loading-overlay">Failed to load profiles. <button onclick="loadProfiles()" class="refresh-btn" style="margin-left:8px">↻ Retry</button></div>';
  } finally {
    profilesLoading = false;
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

// ---------------------------------------------------------------------------
// Profile CRUD + launch
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Port/mode memory (localStorage)
// ---------------------------------------------------------------------------

function getProfilePort(name) {
  const saved = localStorage.getItem('pinchtab-port-' + name);
  if (saved) return saved;
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
  return true;
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

async function resetProfile(name) {
  if (!await appConfirm('Reset profile "' + name + '"? This clears session data, cookies, and cache.', '🔄 Reset Profile')) return;
  await fetch('/profiles/' + name + '/reset', { method: 'POST' });
  closeModal();
  loadProfiles();
}

// ---------------------------------------------------------------------------
// Instance lifecycle
// ---------------------------------------------------------------------------

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
    } catch (e) { clearInterval(poll); }
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
  showModal('📄 Logs: ' + id,
    '<pre style="background:var(--bg);padding:12px;border-radius:6px;font-size:11px;max-height:400px;overflow:auto;color:var(--text-subtle);white-space:pre-wrap">' + esc(text) + '</pre>'
  );
}

async function viewAnalytics(name) {
  let analytics = null;
  try {
    const res = await fetch('/profiles/' + name + '/analytics');
    if (res.ok) analytics = await res.json();
  } catch (e) {}

  let tabs = [], agentList = [];
  try {
    const tabsRes = await fetch('/tabs');
    const tabsData = await tabsRes.json();
    tabs = tabsData.tabs || [];
  } catch (e) {}
  try {
    const agentsRes = await fetch('/dashboard/agents');
    agentList = await agentsRes.json() || [];
  } catch (e) {}

  let html = '';
  html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">LIVE STATUS</h4>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Tabs open: <span style="color:var(--text)">' + tabs.length + '</span></div>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:12px">Agents seen: <span style="color:var(--text)">' + agentList.length + '</span></div>';

  if (agentList.length > 0) {
    html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">AGENTS</h4>';
    agentList.forEach(a => {
      html += '<div style="font-size:12px;color:var(--text-subtle);padding:3px 0;display:flex;justify-content:space-between">';
      html += '<span style="color:var(--primary);font-weight:600">' + esc(a.agentId) + '</span>';
      html += '<span>' + a.actionCount + ' actions — ' + esc(a.lastAction || '') + '</span>';
      html += '</div>';
    });
    html += '<div style="margin-bottom:12px"></div>';
  }

  if (analytics && analytics.totalActions > 0) {
    html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">TRACKED (' + analytics.totalActions + ' actions)</h4>';
    if (analytics.topEndpoints) {
      analytics.topEndpoints.forEach(e => {
        html += '<div style="font-size:12px;color:var(--text-subtle);padding:2px 0">' + esc(e.endpoint) + ' — ' + e.count + 'x, avg ' + e.avgMs + 'ms</div>';
      });
    }
    if (analytics.suggestions) {
      html += '<div style="margin-top:8px">';
      analytics.suggestions.forEach(s => {
        html += '<p style="color:var(--primary);font-size:12px;margin-bottom:4px">💡 ' + esc(s) + '</p>';
      });
      html += '</div>';
    }
  } else {
    html += '<p style="color:var(--text-faint);font-size:12px">No tracked actions yet. Agents need to send <code style="color:var(--text-muted)">X-Profile</code> header to track per-profile analytics.</p>';
  }

  showModal('📊 Analytics: ' + name, html);
}

document.getElementById('modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) closeModal();
});
