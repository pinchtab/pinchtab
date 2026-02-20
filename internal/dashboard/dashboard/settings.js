'use strict';

const screencastSettings = { fps: 1, quality: 30, maxWidth: 800 };

async function loadSettings() {
  try {
    const res = await fetch('/stealth/status');
    const data = await res.json();
    document.getElementById('set-stealth').value = data.level || 'light';
    updateStealthInfo(data);
  } catch (e) {}

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
  } catch (e) {
    document.getElementById('server-info').textContent = 'Failed to load server info';
  }

  const saved = JSON.parse(localStorage.getItem('pinchtab-settings') || '{}');
  if (saved.fps) { document.getElementById('set-fps').value = saved.fps; document.getElementById('fps-val').textContent = saved.fps + ' fps'; screencastSettings.fps = saved.fps; }
  if (saved.quality) { document.getElementById('set-quality').value = saved.quality; document.getElementById('quality-val').textContent = saved.quality + '%'; screencastSettings.quality = saved.quality; }
  if (saved.maxWidth) { document.getElementById('set-maxwidth').value = saved.maxWidth; screencastSettings.maxWidth = saved.maxWidth; }

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

  localStorage.setItem('pinchtab-settings', JSON.stringify(screencastSettings));

  Object.values(screencastSockets).forEach(s => s.close());
  Object.keys(screencastSockets).forEach(k => delete screencastSockets[k]);

  await appAlert('Settings saved.', '⚙️ Settings');
}

async function applyStealth() {
  const level = document.getElementById('set-stealth').value;
  updateStealthInfo({ level });
  await appAlert('Stealth level change requires restarting Pinchtab with BRIDGE_STEALTH=' + level, '🛡️ Stealth');
}
