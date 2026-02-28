'use strict';

let instancesLoading = false;

async function loadInstances() {
  if (instancesLoading) return;
  instancesLoading = true;

  const grid = document.getElementById('instances-grid');
  if (!grid.querySelector('.inst-card')) {
    grid.innerHTML = '<div class="loading-overlay"><span class="spinner"></span>Loading instances...</div>';
  }

  try {
    const instancesRaw = await fetchJSONOr('/instances', { instances: [] });
    const instances = Array.isArray(instancesRaw.instances) ? instancesRaw.instances : [];

    const cards = [];

    instances.forEach(inst => {
      cards.push(renderInstanceCard(inst));
    });

    const grid = document.getElementById('instances-grid');
    if (cards.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No running instances.<br>Launch a profile to start one.</div>';
      return;
    }
    grid.innerHTML = cards.join('');
  } catch (e) {
    console.error('Failed to load instances', e);
    grid.innerHTML = '<div class="loading-overlay">Failed to load instances. <button onclick="loadInstances()" class="refresh-btn" style="margin-left:8px">↻ Retry</button></div>';
  } finally {
    instancesLoading = false;
  }
}

function renderInstanceCard(inst) {
  const id = inst.id || '';
  const name = inst.name || 'Unknown';
  const port = inst.port || '-';
  const status = inst.status || 'stopped';
  const headless = inst.headless !== false;
  const mode = headless ? 'headless' : 'headed';
  const startTime = inst.startTime ? new Date(inst.startTime).toLocaleString() : '-';

  let statusBadge = '<span class="inst-badge stopped">stopped</span>';
  if (status === 'running') statusBadge = '<span class="inst-badge running">running</span>';
  else if (status === 'starting') statusBadge = '<span class="inst-badge starting">starting</span>';
  else if (status === 'stopping') statusBadge = '<span class="inst-badge stopping">stopping</span>';
  else if (status === 'error') statusBadge = '<span class="inst-badge error">error</span>';

  let actions = '';
  if (status === 'running') {
    actions = '<button onclick="viewInstanceDetails(\'' + esc(id) + '\', \'' + esc(name) + '\')">Details</button>'
      + '<button class="danger" onclick="stopInstance(\'' + esc(id) + '\', \'' + esc(name) + '\')">Stop</button>';
  } else {
    actions = '<button onclick="viewInstanceDetails(\'' + esc(id) + '\', \'' + esc(name) + '\')">Details</button>';
  }

  return `
    <div class="inst-card">
      <div class="inst-header">
        <span class="inst-name">${esc(name)}</span>
        ${statusBadge}
      </div>
      <div class="inst-body">
        <div class="inst-row"><span class="label">ID</span><span class="value" style="font-family:var(--font-mono);font-size:11px">${esc(id.substring(0, 12))}...</span></div>
        <div class="inst-row"><span class="label">Port</span><span class="value">${esc(String(port))}</span></div>
        <div class="inst-row"><span class="label">Mode</span><span class="value">${mode}</span></div>
        <div class="inst-row"><span class="label">Status</span><span class="value" style="text-transform:capitalize">${esc(status)}</span></div>
        <div class="inst-row"><span class="label">Started</span><span class="value">${esc(startTime)}</span></div>
      </div>
      <div class="inst-actions">
        ${actions}
      </div>
    </div>
  `;
}

async function viewInstanceDetails(id, name) {
  const encId = encodeURIComponent(id);

  try {
    const [logsRes, tabsRaw] = await Promise.all([
      fetch('/instances/' + encId + '/logs'),
      fetchJSONOr('/instances/tabs', [])
    ]);

    const logsText = logsRes.ok ? await logsRes.text() : 'No logs available';
    const tabs = Array.isArray(tabsRaw.tabs) ? tabsRaw.tabs : [];
    const instanceTabs = tabs.filter(t => t.instanceId === id || t.instanceName === name);

    let html = '';
    html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">TABS (' + instanceTabs.length + ')</h4>';
    if (instanceTabs.length > 0) {
      instanceTabs.forEach(tab => {
        const tabTitle = tab.title || 'Untitled';
        const tabUrl = tab.url || '';
        html += '<div style="font-size:12px;padding:4px 0;border-bottom:1px solid var(--border)">';
        html += '<div style="color:var(--text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(tabTitle) + '</div>';
        html += '<div style="color:var(--text-faint);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(tabUrl) + '</div>';
        html += '</div>';
      });
    } else {
      html += '<p style="color:var(--text-faint);font-size:12px">No tabs open in this instance.</p>';
    }

    html += '<div style="margin-top:16px"></div>';
    html += '<h4 style="color:var(--text-muted);font-size:12px;margin-bottom:8px">LOGS</h4>';
    if (!logsText) {
      html += '<p style="color:var(--text-faint);font-size:12px">No logs available.</p>';
    } else {
      html += '<pre style="background:var(--bg);padding:12px;border-radius:6px;font-size:11px;max-height:30vh;overflow:auto;color:var(--text-subtle);white-space:pre-wrap">' + esc(logsText) + '</pre>';
    }

    html += '<div style="display:flex;justify-content:flex-end;gap:8px;margin-top:16px;padding-top:16px;border-top:1px solid var(--border)">';
    html += '<button class="secondary" onclick="closeModal()">Close</button>';
    html += '</div>';

    showModal('Instance: ' + name, html, ' ', { wide: true });
  } catch (e) {
    await appAlert('Failed to load details: ' + e.message, 'Error');
  }
}

async function stopInstance(id, name) {
  if (!await appConfirm('Stop instance "' + name + '"?', '⏹ Stop Instance')) return;

  try {
    const res = await fetch('/instances/' + encodeURIComponent(id) + '/stop', { method: 'POST' });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      await appAlert('Stop failed: ' + (data.error || res.statusText), 'Error');
      return;
    }
    setTimeout(loadInstances, 700);
  } catch (e) {
    await appAlert('Stop error: ' + e.message, 'Error');
  }
}
