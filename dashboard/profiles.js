'use strict';

let profilesLoading = false;
const profileByName = {};

async function fetchJSONOr(url, fallback) {
  try {
    const res = await fetch(url);
    if (!res.ok) return fallback;
    const contentType = res.headers.get('content-type') || '';
    if (!contentType.includes('application/json')) return fallback;
    return await res.json();
  } catch (e) {
    return fallback;
  }
}

async function loadProfiles() {
  if (profilesLoading) return;
  profilesLoading = true;

  const grid = document.getElementById('profiles-grid');
  if (!grid.querySelector('.inst-card')) {
    grid.innerHTML = '<div class="loading-overlay"><span class="spinner"></span>Loading profiles...</div>';
  }

  try {
    const [profiles, instances, health] = await Promise.all([
      fetchJSONOr('/profiles', []),
      fetchJSONOr('/instances', []),
      fetchJSONOr('/health', {})
    ]);
    let tabsData = { tabs: [] };
    if (!health.mode) {
      tabsData = await fetchJSONOr('/tabs', { tabs: [] });
    }

    const instanceByName = {};
    instances.forEach(inst => { instanceByName[inst.name] = inst; });

    Object.keys(profileByName).forEach(k => { delete profileByName[k]; });
    profiles.forEach(p => { profileByName[p.name] = p; });

    const profileNames = new Set(profiles.map(p => p.name));
    const extraInstances = instances.filter(i => !profileNames.has(i.name));

    const grid = document.getElementById('profiles-grid');
    const cards = [];

    if (!health.mode) {
      cards.push(renderMainCard(tabsData.tabs ? tabsData.tabs.length : 0));
    }

    profiles.forEach(p => {
      cards.push(renderProfileCard(p, instanceByName[p.name] || null));
    });

    extraInstances.forEach(inst => {
      cards.push(renderProfileCard({
        name: inst.name,
        sizeMB: 0,
        source: 'instance'
      }, inst));
    });

    if (cards.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No profiles yet.<br>Click <b>+ New Profile</b> to create one.</div>';
      return;
    }
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
    <div class="inst-card" style="border-color:var(--border-active)">
      <div class="inst-header">
        <span class="inst-name">🦀 Main</span>
        <span class="inst-badge running">running :${location.port || '9867'}</span>
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">Tabs</span><span class="value">${tabCount}</span></div>
        <div class="inst-row"><span class="label">Port</span><span class="value">${location.port || '9867'}</span></div>
      </div>
      <div class="inst-actions">
        <button onclick="viewMainLive()">Live</button>
        <button class="btn-feed" onclick="viewProfileInfo('main', '')">Info</button>
      </div>
    </div>
  `;
}

function renderProfileCard(profile, inst) {
  const name = profile.name;
  const sizeMB = profile.sizeMB;
  const source = profile.source;
  const accountText = profile.accountEmail || profile.accountName || '';
  const accountValue = accountText || '--';
  const chromeProfileName = profile.chromeProfileName || '';
  const status = inst ? inst.status : 'stopped';
  const isRunning = status === 'running';
  const isError = status === 'error';
  const tabsValue = (isRunning && inst) ? String(inst.tabCount ?? 0) : '-';
  const pidValue = (isRunning && inst && inst.pid) ? String(inst.pid) : '-';
  let statusBadge = '<span class="inst-badge stopped">stopped</span>';
  if (status === 'running') statusBadge = '<span class="inst-badge running">running :' + inst.port + '</span>';
  else if (status === 'starting') statusBadge = '<span class="inst-badge starting">starting</span>';
  else if (status === 'stopping') statusBadge = '<span class="inst-badge stopping">stopping</span>';
  else if (status === 'error') statusBadge = '<span class="inst-badge error">error</span>';

  let actions = '';
  if (isRunning) {
    actions =
      '<button onclick="viewInstanceLive(\'' + esc(inst.id) + '\', \'' + esc(inst.port) + '\')">Live</button>'
      + '<button class="btn-feed" onclick="viewProfileInfo(\'' + esc(name) + '\', \'' + esc(inst.id) + '\')">Info</button>'
      + '<button class="danger" onclick="stopProfile(\'' + esc(name) + '\')">Stop</button>';
  } else if (inst && (status === 'starting' || status === 'stopping')) {
    actions =
      '<button class="btn-feed" onclick="viewProfileInfo(\'' + esc(name) + '\', \'' + esc(inst.id) + '\')">Info</button>'
      + '<button class="danger" onclick="stopProfile(\'' + esc(name) + '\')">Stop</button>';
  } else {
    actions =
      '<button class="btn-launch" onclick="openLaunchModal(\'' + esc(name) + '\')">Launch</button>'
      + '<button class="btn-feed" onclick="viewProfileInfo(\'' + esc(name) + '\', \'' + esc(inst ? inst.id : '') + '\')">Info</button>'
      + '<button class="danger" onclick="deleteProfile(\'' + esc(name) + '\')">Delete</button>';
  }

  return `
    <div class="inst-card">
      <div class="inst-header">
        <span class="inst-name">${esc(name)}</span>
        ${statusBadge}
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">Size</span><span class="value">${sizeMB ? sizeMB.toFixed(0) + ' MB' : '—'}</span></div>
        <div class="inst-row"><span class="label">Source</span><span class="value">${esc(source)}</span></div>
        ${chromeProfileName ? '<div class="inst-row"><span class="label">Chrome</span><span class="value">' + esc(chromeProfileName) + '</span></div>' : ''}
        <div class="inst-row"><span class="label">Account</span><span class="value">${esc(accountValue)}</span></div>
        <div class="inst-row"><span class="label">Tabs</span><span class="value">${tabsValue}</span></div>
        <div class="inst-row"><span class="label">PID</span><span class="value">${pidValue}</span></div>
        ${isError && inst && inst.error ? '<div class="inst-row"><span class="label">Error</span><span class="value">' + esc(inst.error) + '</span></div>' : ''}
      </div>
      <div class="inst-actions">
        ${actions}
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
  if (saved) {
    if (saved === String(location.port || '9867')) {
      return String(Number(saved) + 1);
    }
    return saved;
  }
  const cards = document.querySelectorAll('#profiles-grid .inst-card');
  let idx = 0;
  cards.forEach((c, i) => { if (c.querySelector('.inst-name')?.textContent === name) idx = i; });
  return String(9867 + Math.max(idx, 1));
}

function saveProfilePort(name, port) {
  localStorage.setItem('pinchtab-port-' + name, port);
}

function openLaunchModal(name) {
  document.getElementById('launch-name').value = name;
  document.getElementById('launch-port').value = getProfilePort(name);
  document.getElementById('launch-profile-path').value = (profileByName[name] && profileByName[name].path) || '';
  updateLaunchCommand();
  document.getElementById('launch-modal').classList.add('open');
  document.getElementById('launch-port').focus();
}

function closeLaunchModal() {
  document.getElementById('launch-modal').classList.remove('open');
}

function shellQuote(value) {
  return '\'' + String(value || '').replace(/'/g, "'\\''") + '\'';
}

function updateLaunchCommand() {
  const name = document.getElementById('launch-name').value.trim();
  const port = document.getElementById('launch-port').value.trim();
  const profilePathRaw = document.getElementById('launch-profile-path').value.trim();
  const profilePath = profilePathRaw || '<PROFILE_PATH>';
  const statePath = profilePath + '/.pinchtab-state';
  const prefix = profilePathRaw ? '' : '# replace <PROFILE_PATH> first\n';
  const cmd = '(test -x ./bin/pinchtab || go build -o ./bin/pinchtab .) && '
    + 'BRIDGE_PORT=' + shellQuote(port || '9868') + ' '
    + 'BRIDGE_PROFILE=' + shellQuote(profilePath) + ' '
    + 'BRIDGE_STATE_DIR=' + shellQuote(statePath) + ' '
    + 'BRIDGE_HEADLESS=false BRIDGE_NO_RESTORE=true BRIDGE_NO_DASHBOARD=true '
    + './bin/pinchtab';
  document.getElementById('launch-command').value = prefix + cmd;
}

async function copyLaunchCommand() {
  updateLaunchCommand();
  const el = document.getElementById('launch-command');
  const text = el.value;
  try {
    await navigator.clipboard.writeText(text);
    await appAlert('Command copied to clipboard.', 'Launch');
  } catch (e) {
    el.focus();
    el.select();
    document.execCommand('copy');
    await appAlert('Command copied (fallback).', 'Launch');
  }
}

async function doLaunch() {
  const name = document.getElementById('launch-name').value.trim();
  const port = document.getElementById('launch-port').value.trim();
  const headless = false;

  if (!name || !port) { await appAlert('Port required'); return; }
  if (port === String(location.port || '9867')) {
    await appAlert('Choose a different port than the dashboard port (' + (location.port || '9867') + ').', 'Port in Use');
    return;
  }

  saveProfilePort(name, port);
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


function pollInstanceStatus(id) {
  let attempts = 0;
  const poll = setInterval(async () => {
    attempts++;
    if (attempts > 60) {
      clearInterval(poll);
      return;
    }
    try {
      const res = await fetch('/instances');
      const instances = await res.json();
      const inst = instances.find(i => i.id === id);
      if (inst && (inst.status === 'running' || inst.status === 'error' || inst.status === 'stopped')) {
        clearInterval(poll);
        if (inst.status === 'error') {
          await appAlert('Launch failed: ' + (inst.error || 'unknown error'), 'Error');
        } else if (inst.status === 'stopped') {
          await appAlert('Launch stopped before ready. Check Logs for details.', 'Launch');
        }
        loadProfiles();
      }
    } catch (e) {}
  }, 1000);
}

async function stopInstance(id) {
  if (!await appConfirm('Stop instance ' + id + '?', '⏹ Stop Instance')) return;
  await fetch('/instances/' + id + '/stop', { method: 'POST' });
  setTimeout(loadProfiles, 1000);
}

async function stopProfile(name) {
  if (!await appConfirm('Stop profile "' + name + '" gracefully?', '⏹ Stop Profile')) return;
  const res = await fetch('/profiles/' + encodeURIComponent(name) + '/stop', { method: 'POST' });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    await appAlert('Stop failed: ' + (data.error || res.statusText), 'Error');
    return;
  }
  setTimeout(loadProfiles, 700);
}

async function viewProfileInfo(name, instanceID) {
  const encName = encodeURIComponent(name);
  const [analytics, agents, liveTabs, state] = await Promise.all([
    fetchJSONOr('/profiles/' + encName + '/analytics', null),
    fetchJSONOr('/dashboard/agents', []),
    fetchJSONOr('/instances/tabs', []),
    fetchJSONOr('/profiles/' + encName + '/instance', null),
  ]);

  const finalInstanceID = instanceID || (state && state.id ? state.id : '');
  let logsText = '';
  if (finalInstanceID) {
    try {
      const logsRes = await fetch('/instances/' + encodeURIComponent(finalInstanceID) + '/logs');
      logsText = logsRes.ok ? await logsRes.text() : '';
    } catch (e) {
      logsText = '';
    }
  }

  const tabsCount = Array.isArray(liveTabs)
    ? (name === 'main' ? liveTabs.length : liveTabs.filter(t => t.instanceName === name).length)
    : 0;
  const agentsForProfile = Array.isArray(agents)
    ? agents.filter(a => a.profile === name)
    : [];
  const status = state && state.status ? state.status : 'stopped';
  const port = state && state.port ? state.port : '-';
  const pid = state && state.pid ? state.pid : '-';

  let html = '';
  html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">STATUS</h4>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">State: <span style="color:var(--text)">' + esc(status) + '</span></div>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Port: <span style="color:var(--text)">' + esc(String(port)) + '</span></div>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:12px">PID: <span style="color:var(--text)">' + esc(String(pid)) + '</span></div>';

  html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">LIVE</h4>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Tabs open: <span style="color:var(--text)">' + tabsCount + '</span></div>';
  html += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:12px">Agents: <span style="color:var(--text)">' + agentsForProfile.length + '</span></div>';

  if (analytics && analytics.totalActions > 0) {
    html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">FEED (' + analytics.totalActions + ' actions)</h4>';
    if (analytics.topEndpoints) {
      analytics.topEndpoints.forEach(e => {
        html += '<div style="font-size:12px;color:var(--text-subtle);padding:2px 0">' + esc(e.endpoint) + ' - ' + e.count + 'x, avg ' + e.avgMs + 'ms</div>';
      });
    }
    if (analytics.suggestions) {
      html += '<div style="margin-top:8px">';
      analytics.suggestions.forEach(s => {
        html += '<p style="color:var(--text-bright);font-size:12px;margin-bottom:4px">' + esc(s) + '</p>';
      });
      html += '</div>';
    }
  } else {
    html += '<p style="color:var(--text-faint);font-size:12px;margin-bottom:12px">No tracked actions yet.</p>';
  }

  html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">LOGS</h4>';
  if (!logsText) {
    html += '<p style="color:var(--text-faint);font-size:12px">No instance logs available for this profile.</p>';
  } else {
    html += '<pre style="background:var(--bg);padding:12px;border-radius:6px;font-size:11px;max-height:45vh;overflow:auto;color:var(--text-subtle);white-space:pre-wrap">' + esc(logsText) + '</pre>';
  }

  showModal('INFO: ' + name, html, null, { wide: true });
}

const launchPortInput = document.getElementById('launch-port');
if (launchPortInput) {
  launchPortInput.addEventListener('input', updateLaunchCommand);
}

document.getElementById('modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) closeModal();
});
