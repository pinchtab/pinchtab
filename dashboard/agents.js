'use strict';

const MAX_EVENTS = 500;
const events = [];
const agents = {};
let selectedAgent = null;
let currentFilter = 'all';

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
    el.innerHTML = '<div class="empty-state"><img src="/dashboard/pinchtab-headed-192.png" alt="" class="empty-icon">No matching events.</div>';
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

document.querySelectorAll('.filter-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    currentFilter = btn.dataset.filter;
    renderFeed();
  });
});
