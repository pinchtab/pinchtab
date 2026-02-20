'use strict';

let profilesInterval = null;

function switchView(view) {
  document.querySelectorAll('.view-btn').forEach(b => b.classList.remove('active'));
  const btn = document.querySelector('[data-view="' + view + '"]');
  if (btn) btn.classList.add('active');
  document.getElementById('feed-view').style.display = view === 'feed' ? 'flex' : 'none';
  document.getElementById('profiles-view').style.display = view === 'profiles' ? 'flex' : 'none';
  document.getElementById('settings-view').style.display = view === 'settings' ? 'block' : 'none';

  if (view === 'profiles') loadProfiles();
  if (view === 'settings') loadSettings();

  if (profilesInterval) { clearInterval(profilesInterval); profilesInterval = null; }
  if (view === 'profiles') {
    profilesInterval = setInterval(loadProfiles, 10000);
  }
}

function openInstanceDirect(port) {
  window.open('http://localhost:' + port + '/dashboard', '_blank');
}

connect();
switchView('profiles');
