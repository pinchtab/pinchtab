const sessionSeed = (typeof __pinchtab_seed !== 'undefined') ? __pinchtab_seed : 42;

const seededRandom = (function() {
  const cache = {};
  return function(seed) {
    if (cache[seed] !== undefined) return cache[seed];
    let t = (seed + 0x6D2B79F5) | 0;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    const result = ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    cache[seed] = result;
    return result;
  };
})();

// Aggressive webdriver removal - Chrome sets this after page context creation
// We need to intercept at multiple levels
(function() {
  // Method 1: Delete from prototype
  const proto = Object.getPrototypeOf(navigator);
  try { delete proto.webdriver; } catch(e) {}
  
  // Method 2: Non-configurable property on instance (prevents Chrome from overwriting)
  try {
    Object.defineProperty(navigator, 'webdriver', {
      get: () => undefined,
      set: () => {},  // Ignore any attempts to set it
      configurable: false  // Prevent Chrome from reconfiguring
    });
  } catch(e) {}
  
  // Method 3: Also lock down the prototype
  try {
    Object.defineProperty(proto, 'webdriver', {
      get: () => undefined,
      set: () => {},
      configurable: false
    });
  } catch(e) {}
  
  // Method 4: Continuously clean up (for late injections)
  const cleanup = () => {
    if (navigator.webdriver === true) {
      try { delete navigator.webdriver; } catch(e) {}
    }
  };
  cleanup();
  setTimeout(cleanup, 0);
  setTimeout(cleanup, 10);
  setTimeout(cleanup, 100);
})();

delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;

if (!window.chrome) { window.chrome = {}; }
if (!window.chrome.runtime) {
  window.chrome.runtime = {
    onConnect: undefined,
    onMessage: undefined
  };
}

const originalQuery = window.navigator.permissions.query;
window.navigator.permissions.query = (parameters) => (
  parameters.name === 'notifications' ?
    Promise.resolve({ state: Notification.permission }) :
    originalQuery(parameters)
);

// Create a proper PluginArray-like object that passes instanceof checks
(function() {
  const fakePlugins = [
    { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer', description: 'Portable Document Format', length: 1 },
    { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '', length: 1 },
    { name: 'Native Client', filename: 'internal-nacl-plugin', description: '', length: 1 }
  ];
  
  // Get the real PluginArray prototype from navigator.plugins
  const realPluginsProto = Object.getPrototypeOf(navigator.plugins);
  
  // Create array-like object with PluginArray prototype
  const pluginArray = Object.create(realPluginsProto, {
    length: { value: fakePlugins.length, writable: false, enumerable: true },
    item: { value: function(i) { return this[i] || null; }, writable: false },
    namedItem: { value: function(name) { 
      for (let i = 0; i < this.length; i++) {
        if (this[i] && this[i].name === name) return this[i];
      }
      return null;
    }, writable: false },
    refresh: { value: function() {}, writable: false }
  });
  
  // Add indexed access
  fakePlugins.forEach((p, i) => {
    // Create Plugin-like object
    const plugin = Object.create(Plugin.prototype, {
      name: { value: p.name, writable: false, enumerable: true },
      filename: { value: p.filename, writable: false, enumerable: true },
      description: { value: p.description, writable: false, enumerable: true },
      length: { value: p.length, writable: false, enumerable: true },
      item: { value: function(i) { return null; }, writable: false },
      namedItem: { value: function(n) { return null; }, writable: false }
    });
    Object.defineProperty(pluginArray, i, { value: plugin, writable: false, enumerable: true });
    Object.defineProperty(pluginArray, p.name, { value: plugin, writable: false, enumerable: false });
  });
  
  Object.defineProperty(navigator, 'plugins', {
    get: () => pluginArray,
    configurable: true
  });
})();

Object.defineProperty(navigator, 'languages', {
  get: () => ['en-US', 'en'],
});


// Derive platform from user agent to avoid mismatch detection
(function() {
  const ua = navigator.userAgent || '';
  let platform = 'Win32'; // default
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) {
    platform = 'MacIntel';
  } else if (ua.includes('Linux')) {
    platform = ua.includes('x86_64') || ua.includes('amd64') ? 'Linux x86_64' : 'Linux';
  } else if (ua.includes('Windows')) {
    platform = ua.includes('Win64') || ua.includes('WOW64') ? 'Win32' : 'Win32';
  }
  Object.defineProperty(navigator, 'platform', {
    get: () => platform,
    configurable: true
  });
})();

Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 100,
});

const stealthLevel = (typeof __pinchtab_stealth_level !== 'undefined') ? __pinchtab_stealth_level : 'light';

if (stealthLevel === 'full') {

const getParameter = WebGLRenderingContext.prototype.getParameter;
WebGLRenderingContext.prototype.getParameter = function(parameter) {
  if (parameter === 37445) return 'Intel Inc.';
  if (parameter === 37446) return 'Intel Iris OpenGL Engine';
  return getParameter.apply(this, arguments);
};

const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
const originalToBlob = HTMLCanvasElement.prototype.toBlob;
const originalGetImageData = CanvasRenderingContext2D.prototype.getImageData;

HTMLCanvasElement.prototype.toDataURL = function(...args) {
  const context = this.getContext('2d');
  if (context && this.width > 0 && this.height > 0) {
    const tempCanvas = document.createElement('canvas');
    tempCanvas.width = this.width;
    tempCanvas.height = this.height;
    const tempCtx = tempCanvas.getContext('2d');
    tempCtx.drawImage(this, 0, 0);
    const imageData = tempCtx.getImageData(0, 0, this.width, this.height);
    const pixelCount = Math.min(10, Math.floor(imageData.data.length / 400));
    for (let i = 0; i < pixelCount; i++) {
      const idx = Math.floor(seededRandom(sessionSeed + i) * (imageData.data.length / 4)) * 4;
      if (imageData.data[idx] < 255) imageData.data[idx] += 1;
      if (imageData.data[idx + 1] < 255) imageData.data[idx + 1] += 1;
    }
    tempCtx.putImageData(imageData, 0, 0);
    return originalToDataURL.apply(tempCanvas, args);
  }
  return originalToDataURL.apply(this, args);
};

HTMLCanvasElement.prototype.toBlob = function(callback, type, quality) {
  const dataURL = this.toDataURL(type, quality);
  const arr = dataURL.split(',');
  const mime = arr[0].match(/:(.*?);/)[1];
  const bstr = atob(arr[1]);
  let n = bstr.length;
  const u8arr = new Uint8Array(n);
  while(n--){ u8arr[n] = bstr.charCodeAt(n); }
  const blob = new Blob([u8arr], {type: mime});
  setTimeout(() => callback(blob), 5 + seededRandom(sessionSeed + 1000) * 10);
};

CanvasRenderingContext2D.prototype.getImageData = function(...args) {
  const imageData = originalGetImageData.apply(this, args);
  const pixelCount = imageData.data.length / 4;
  const noisyPixels = Math.min(10, pixelCount * 0.0001);
  for (let i = 0; i < noisyPixels; i++) {
    const pixelIndex = Math.floor(Math.random() * pixelCount) * 4;
    imageData.data[pixelIndex] = Math.min(255, Math.max(0, imageData.data[pixelIndex] + (Math.random() > 0.5 ? 1 : -1)));
  }
  return imageData;
};

const originalMeasureText = CanvasRenderingContext2D.prototype.measureText;
CanvasRenderingContext2D.prototype.measureText = function(text) {
  const metrics = originalMeasureText.apply(this, arguments);
  const noise = 0.0001 + (seededRandom(sessionSeed + text.length) * 0.0002);
  return new Proxy(metrics, {
    get(target, prop) {
      if (prop === 'width') return target.width * (1 + noise);
      return target[prop];
    }
  });
};

if (window.RTCPeerConnection) {
  const originalRTCPeerConnection = window.RTCPeerConnection;
  window.RTCPeerConnection = function(config, constraints) {
    if (config && config.iceServers) config.iceTransportPolicy = 'relay';
    return new originalRTCPeerConnection(config, constraints);
  };
  window.RTCPeerConnection.prototype = originalRTCPeerConnection.prototype;
}

}

const __pinchtab_origGetTimezoneOffset = Date.prototype.getTimezoneOffset;
Object.defineProperty(Date.prototype, 'getTimezoneOffset', {
  value: function() {
    return window.__pinchtab_timezone || __pinchtab_origGetTimezoneOffset.call(this);
  }
});

const hardwareCore = 2 + Math.floor(seededRandom(sessionSeed) * 6) * 2;
const deviceMem = [2, 4, 8, 16][Math.floor(seededRandom(sessionSeed * 2) * 4)];

Object.defineProperty(navigator, 'hardwareConcurrency', {
  get: () => hardwareCore,
  configurable: true
});

Object.defineProperty(navigator, 'deviceMemory', {
  get: () => deviceMem
});
