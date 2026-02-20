'use strict';

let profilesLoading = false;
let detailsLiveContext = null;

async function refreshDetailsLive() {
  if (!detailsLiveContext) return;
  closeScreencastSockets();

  const grid = document.getElementById('details-screencast-grid');
  const countEl = document.getElementById('details-live-count');
  if (!grid || !countEl) return;

  try {
    const result = await loadInstanceStreams(
      detailsLiveContext.instanceId,
      detailsLiveContext.port
    );
    if (!result.ok) {
      countEl.textContent = 'Unavailable';
      renderLiveEmpty(grid, 'Live view unavailable.');
      return;
    }
    countEl.textContent = result.countLabel;
    if (!Array.isArray(result.streams) || result.streams.length === 0) {
      renderLiveEmpty(grid, result.emptyMessage || 'No tabs open.');
      return;
    }
    renderStreams(grid, result.streams);
  } catch (e) {
    countEl.textContent = 'Error';
    renderLiveEmpty(grid, 'Failed to load live view.');
    console.error('Details live refresh failed', e);
  }
}
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
  const useWhen = profile.useWhen || '';
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
    actions = '<button onclick="viewProfileDetails(\'' + esc(name) + '\', \'' + esc(inst.id) + '\')">Details</button>'
      + '<button class="danger" onclick="stopProfile(\'' + esc(name) + '\')">Stop</button>';
  } else {
    actions = '<button onclick="viewProfileDetails(\'' + esc(name) + '\', \'' + esc(inst ? inst.id : '') + '\')">Details</button>'
      + '<button class="btn-launch" onclick="openLaunchModal(\'' + esc(name) + '\')">Launch</button>';
  }

  return `
    <div class="inst-card">
      <div class="inst-header">
        <span class="inst-name">${esc(name)}</span>
        ${statusBadge}
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">Size</span><span class="value">${sizeMB ? sizeMB.toFixed(0) + ' MB' : '—'}</span></div>
        <div class="inst-row"><span class="label">Account</span><span class="value">${esc(accountValue)}</span></div>
        <div class="inst-row" style="align-items:flex-start"><span class="label">Use when</span><span class="value" style="display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden;text-overflow:ellipsis;height:3.9em;line-height:1.3em;text-align:left">${useWhen ? esc(useWhen) : '<span style="color:var(--text-faint)">—</span>'}</span></div>
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
  const useWhen = document.getElementById('create-usewhen').value.trim();
  const source = document.getElementById('create-source').value.trim();

  if (!name) { await appAlert('Name required'); return; }
  closeCreateProfileModal();

  try {
    const body = { name, useWhen };
    if (source) {
      body.source = source;
      await fetch('/profiles/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
    } else {
      await fetch('/profiles/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
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
  const cmd = '(test -x ./bin/pinchtab || go build -o ./bin/pinchtab ./cmd/pinchtab) && '
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

async function viewProfileDetails(name, instanceID) {
  const encName = encodeURIComponent(name);
  const [profileInfo, analytics, agents, liveTabs, state] = await Promise.all([
    fetchJSONOr('/profiles', []).then(profiles => profiles.find(p => p.name === name) || {}),
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
  const isRunning = status === 'running';

  // Build tab content: Profile & Live/Logs
  let tabProfile = '';
  
  // Metadata
  tabProfile += '<label style="color:var(--text-muted);font-size:12px;display:block;margin-bottom:4px">Name</label>';
  tabProfile += '<input id="profile-name" value="' + esc(name) + '" style="width:100%;margin-bottom:12px" />';
  tabProfile += '<label style="color:var(--text-muted);font-size:12px;display:block;margin-bottom:4px">Use this profile when</label>';
  tabProfile += '<textarea id="profile-usewhen" style="width:100%;min-height:80px;margin-bottom:16px;resize:vertical">' + esc(profileInfo.useWhen || '') + '</textarea>';

  // Status
  tabProfile += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">STATUS</h4>';
  tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">State: <span style="color:var(--text)">' + esc(status) + '</span></div>';
  tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Port: <span style="color:var(--text)">' + esc(String(port)) + '</span></div>';
  tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">PID: <span style="color:var(--text)">' + esc(String(pid)) + '</span></div>';
  tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Size: <span style="color:var(--text)">' + (profileInfo.sizeMB ? profileInfo.sizeMB.toFixed(0) + ' MB' : '—') + '</span></div>';
  tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Source: <span style="color:var(--text)">' + esc(profileInfo.source || '—') + '</span></div>';
  if (profileInfo.path) {
    tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Path: <span style="color:var(--text);font-size:11px;font-family:var(--font-mono)">' + esc(profileInfo.path) + '</span></div>';
  }
  if (profileInfo.chromeProfileName) {
    tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Chrome: <span style="color:var(--text)">' + esc(profileInfo.chromeProfileName) + '</span></div>';
  }
  if (profileInfo.accountEmail || profileInfo.accountName) {
    tabProfile += '<div style="font-size:13px;color:var(--text-subtle);margin-bottom:4px">Account: <span style="color:var(--text)">' + esc(profileInfo.accountEmail || profileInfo.accountName) + '</span></div>';
  }

  // === LIVE TAB ===
  let tabLive = '';
  if (isRunning && state && state.port) {
    tabLive += '<div class="live-popup">';
    tabLive += '<div class="live-toolbar">';
    tabLive += '<button class="refresh-btn" id="details-live-refresh">Refresh</button>';
    tabLive += '<span id="details-live-count" class="live-count">' + tabsCount + ' tab(s)</span>';
    tabLive += '</div>';
    tabLive += '<div id="details-screencast-grid" class="screencast-grid empty">';
    tabLive += '<div class="empty-state"><span class="spinner"></span>Loading live view...</div>';
    tabLive += '</div>';
    tabLive += '</div>';
  } else {
    tabLive += '<div style="display:flex;align-items:center;justify-content:center;height:300px;color:var(--text-faint);font-size:14px">';
    tabLive += 'Instance not running. Launch the profile to see live view.';
    tabLive += '</div>';
  }

  // === LOGS TAB ===
  let tabLogs = '';

  // Tabs & agents
  tabLogs += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">TABS (' + tabsCount + ')</h4>';
  if (isRunning && Array.isArray(liveTabs)) {
    const profileTabs = name === 'main' ? liveTabs : liveTabs.filter(t => t.instanceName === name);
    if (profileTabs.length > 0) {
      profileTabs.forEach(tab => {
        const tabTitle = tab.title || 'Untitled';
        const tabUrl = tab.url || '';
        tabLogs += '<div style="font-size:12px;padding:4px 0;border-bottom:1px solid var(--border)">';
        tabLogs += '<div style="color:var(--text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(tabTitle) + '</div>';
        tabLogs += '<div style="color:var(--text-faint);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(tabUrl) + '</div>';
        tabLogs += '</div>';
      });
    } else {
      tabLogs += '<p style="color:var(--text-faint);font-size:12px">No tabs open.</p>';
    }
  } else {
    tabLogs += '<p style="color:var(--text-faint);font-size:12px">Instance not running.</p>';
  }

  tabLogs += '<div style="margin-top:16px"></div>';
  tabLogs += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">AGENTS (' + agentsForProfile.length + ')</h4>';
  if (agentsForProfile.length > 0) {
    agentsForProfile.forEach(a => {
      tabLogs += '<div style="font-size:12px;color:var(--text-subtle);padding:2px 0">' + esc(a.agentId) + ' — ' + esc(a.status) + '</div>';
    });
  } else {
    tabLogs += '<p style="color:var(--text-faint);font-size:12px">No agents connected.</p>';
  }

  // Activity
  tabLogs += '<div style="margin-top:16px"></div>';
  if (analytics && analytics.totalActions > 0) {
    tabLogs += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">ACTIVITY (' + analytics.totalActions + ' actions)</h4>';
    if (analytics.topEndpoints) {
      analytics.topEndpoints.forEach(e => {
        tabLogs += '<div style="font-size:12px;color:var(--text-subtle);padding:2px 0">' + esc(e.endpoint) + ' — ' + e.count + 'x, avg ' + e.avgMs + 'ms</div>';
      });
    }
  } else {
    tabLogs += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">ACTIVITY</h4>';
    tabLogs += '<p style="color:var(--text-faint);font-size:12px">No tracked actions yet.</p>';
  }

  // Logs
  tabLogs += '<div style="margin-top:16px"></div>';
  tabLogs += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">LOGS</h4>';
  if (!logsText) {
    tabLogs += '<p style="color:var(--text-faint);font-size:12px">No instance logs available.</p>';
  } else {
    tabLogs += '<pre style="background:var(--bg);padding:12px;border-radius:6px;font-size:11px;max-height:30vh;overflow:auto;color:var(--text-subtle);white-space:pre-wrap">' + esc(logsText) + '</pre>';
  }

  // Assemble with tabs
  let html = '';
  html += '<div class="details-tabs" style="display:flex;gap:0;border-bottom:1px solid var(--border);margin-bottom:16px">';
  html += '<button class="details-tab active" onclick="switchDetailsTab(this, \'details-pane-profile\')" style="flex:1;padding:10px;background:none;border:none;border-radius:0;border-bottom:2px solid var(--primary);color:var(--text-bright);font-size:13px;font-weight:600;cursor:pointer;font-family:inherit">Profile</button>';
  html += '<button class="details-tab" onclick="switchDetailsTab(this, \'details-pane-live\')" style="flex:1;padding:10px;background:none;border:none;border-radius:0;border-bottom:2px solid transparent;color:var(--text-muted);font-size:13px;font-weight:600;cursor:pointer;font-family:inherit">Live</button>';
  html += '<button class="details-tab" onclick="switchDetailsTab(this, \'details-pane-logs\')" style="flex:1;padding:10px;background:none;border:none;border-radius:0;border-bottom:2px solid transparent;color:var(--text-muted);font-size:13px;font-weight:600;cursor:pointer;font-family:inherit">Logs</button>';
  html += '</div>';

  html += '<div style="height:400px;overflow-y:auto">';
  html += '<div id="details-pane-profile">' + tabProfile + '</div>';
  html += '<div id="details-pane-live" style="display:none">' + tabLive + '</div>';
  html += '<div id="details-pane-logs" style="display:none">' + tabLogs + '</div>';
  html += '</div>';

  // Footer buttons
  html += '<div style="display:flex;justify-content:space-between;align-items:center;margin-top:16px;padding-top:16px;border-top:1px solid var(--border)">';
  html += '<button class="danger" onclick="deleteProfileFromDetails(\'' + esc(name) + '\')">Delete</button>';
  html += '<div style="display:flex;gap:8px">';
  html += '<button onclick="saveProfileMetadata(\'' + esc(name) + '\')">Save</button>';
  html += '<button class="secondary" onclick="closeModal()">Close</button>';
  html += '</div>';
  html += '</div>';

  showModal('DETAILS', html, ' ', {
    wide: true,
    onClose: () => {
      closeScreencastSockets();
      detailsLiveContext = null;
    }
  });

  // Set up live screencast context for this profile
  if (isRunning && state && state.port) {
    detailsLiveContext = {
      instanceId: finalInstanceID,
      port: state.port,
      name: name
    };
    const refreshBtn = document.getElementById('details-live-refresh');
    if (refreshBtn) refreshBtn.onclick = () => refreshDetailsLive();
  }
}

function switchDetailsTab(btn, paneId) {
  btn.closest('.details-tabs').querySelectorAll('.details-tab').forEach(t => {
    t.style.borderBottomColor = 'transparent';
    t.style.color = 'var(--text-muted)';
    t.classList.remove('active');
  });
  btn.style.borderBottomColor = 'var(--primary)';
  btn.style.color = 'var(--text-bright)';
  btn.classList.add('active');
  document.getElementById('details-pane-profile').style.display = 'none';
  document.getElementById('details-pane-live').style.display = 'none';
  document.getElementById('details-pane-logs').style.display = 'none';
  document.getElementById(paneId).style.display = '';

  // Start/stop screencast when switching to/from Live tab
  if (paneId === 'details-pane-live' && detailsLiveContext) {
    refreshDetailsLive();
  } else {
    closeScreencastSockets();
  }
}

async function saveProfileMetadata(name) {
  const useWhen = document.getElementById('profile-usewhen').value.trim();
  const newName = document.getElementById('profile-name').value.trim();
  
  try {
    const res = await fetch('/profiles/' + encodeURIComponent(name), {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ useWhen, name: newName })
    });
    
    if (res.ok) {
      closeModal();
      loadProfiles();
    } else {
      const data = await res.json().catch(() => ({}));
      await appAlert('Save failed: ' + (data.error || res.statusText), 'Error');
    }
  } catch (e) {
    await appAlert('Save error: ' + e.message, 'Error');
  }
}

async function deleteProfileFromDetails(name) {
  closeModal();
  if (!await appConfirm('Delete profile "' + name + '"? This removes all data.', '🗑️ Delete Profile')) return;
  
  try {
    await fetch('/profiles/' + encodeURIComponent(name), { method: 'DELETE' });
    profilesLoading = false;
    loadProfiles();
  } catch (e) {
    await appAlert('Delete failed: ' + e.message, 'Error');
  }
}

const launchPortInput = document.getElementById('launch-port');
if (launchPortInput) {
  launchPortInput.addEventListener('input', updateLaunchCommand);
}

document.getElementById('modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) closeModal();
});
