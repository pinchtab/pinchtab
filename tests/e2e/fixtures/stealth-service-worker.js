self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('message', async (event) => {
  const port = event.ports && event.ports[0];
  if (!port) return;

  const nav = self.navigator || {};
  const uaData = nav.userAgentData || null;

  port.postMessage({
    userAgent: nav.userAgent || '',
    platform: nav.platform || '',
    webdriverNotTrue: !('webdriver' in nav) || nav.webdriver !== true,
    webdriverValue: ('webdriver' in nav) ? nav.webdriver : 'absent',
    hardwareConcurrency: typeof nav.hardwareConcurrency === 'number' ? nav.hardwareConcurrency : null,
    deviceMemory: typeof nav.deviceMemory === 'number' ? nav.deviceMemory : null,
    userAgentDataPlatform: uaData ? uaData.platform : ''
  });
});
