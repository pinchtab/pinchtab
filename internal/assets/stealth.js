// ═══════════════════════════════════════════════════════════════════════════
// PINCHTAB STEALTH - Bot Detection Evasion
// ═══════════════════════════════════════════════════════════════════════════
//
// Stealth Levels:
//   light  - Safe, no functional impact. Basic automation hiding.
//   medium - Standard stealth. May affect error monitoring and some APIs.
//   full   - Maximum stealth. May break WebRTC, canvas-dependent features.
//
// ═══════════════════════════════════════════════════════════════════════════

const sessionSeed = (typeof __pinchtab_seed !== 'undefined') ? __pinchtab_seed : 42;
const stealthLevel = (typeof __pinchtab_stealth_level !== 'undefined') ? __pinchtab_stealth_level : 'light';

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

// ═══════════════════════════════════════════════════════════════════════════
// LIGHT LEVEL - Safe, no functional impact
// ═══════════════════════════════════════════════════════════════════════════
// - Hides navigator.webdriver
// - Removes CDP markers (cdc_*, __webdriver, etc.)
// - Spoofs plugins array
// - Sets navigator.languages, platform, hardwareConcurrency, deviceMemory
// - Basic chrome.runtime object
// ═══════════════════════════════════════════════════════════════════════════

// CDP MARKER CLEANUP - Remove automation traces
(function() {
  const markers = [
    'cdc_adoQpoasnfa76pfcZLmcfl_Array',
    'cdc_adoQpoasnfa76pfcZLmcfl_Promise',
    'cdc_adoQpoasnfa76pfcZLmcfl_Symbol'
  ];
  markers.forEach(m => { try { delete window[m]; } catch(e) {} });
  
  // Pattern-based cleanup
  const markerPatterns = [/^cdc_/, /^\$cdc_/, /^__webdriver/, /^__selenium/, /^__driver/];
  for (const prop of Object.getOwnPropertyNames(window)) {
    if (markerPatterns.some(p => p.test(prop))) {
      try { delete window[prop]; } catch(e) {}
    }
  }
})();

// WEBDRIVER EVASION - Hide the property
(function() {
  const realNavigator = window.navigator;
  const proxyNavigator = new Proxy(realNavigator, {
    get(target, prop) {
      if (prop === 'webdriver') return undefined;
      return Reflect.get(target, prop, target);
    },
    has(target, prop) {
      if (prop === 'webdriver') return false;
      return Reflect.has(target, prop);
    },
    getOwnPropertyDescriptor(target, prop) {
      if (prop === 'webdriver') return undefined;
      return Reflect.getOwnPropertyDescriptor(target, prop);
    }
  });

  try {
    Object.defineProperty(window, 'navigator', {
      get: () => proxyNavigator,
      configurable: true
    });
  } catch(e) {}

  const proto = Object.getPrototypeOf(realNavigator);
  try { delete realNavigator.webdriver; } catch(e) {}
  try { delete proto.webdriver; } catch(e) {}
})();

// BASIC CHROME OBJECT
if (!window.chrome) { window.chrome = {}; }
if (!window.chrome.runtime) {
  window.chrome.runtime = { onConnect: undefined, onMessage: undefined };
}

// PLUGINS ARRAY - Proper PluginArray that passes instanceof checks
(function() {
  const fakePlugins = [
    { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer', description: 'Portable Document Format', length: 1 },
    { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '', length: 1 },
    { name: 'Native Client', filename: 'internal-nacl-plugin', description: '', length: 1 }
  ];
  
  const realPluginsProto = Object.getPrototypeOf(navigator.plugins);
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
  
  fakePlugins.forEach((p, i) => {
    const plugin = Object.create(Plugin.prototype, {
      name: { value: p.name, writable: false, enumerable: true },
      filename: { value: p.filename, writable: false, enumerable: true },
      description: { value: p.description, writable: false, enumerable: true },
      length: { value: p.length, writable: false, enumerable: true },
      item: { value: function() { return null; }, writable: false },
      namedItem: { value: function() { return null; }, writable: false }
    });
    Object.defineProperty(pluginArray, i, { value: plugin, writable: false, enumerable: true });
    Object.defineProperty(pluginArray, p.name, { value: plugin, writable: false, enumerable: false });
  });
  
  Object.defineProperty(navigator, 'plugins', { get: () => pluginArray, configurable: true });
})();

// NAVIGATOR PROPERTIES
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'], configurable: true });

(function() {
  const ua = navigator.userAgent || '';
  let platform = 'Win32';
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) platform = 'MacIntel';
  else if (ua.includes('Linux')) platform = ua.includes('x86_64') ? 'Linux x86_64' : 'Linux';
  Object.defineProperty(navigator, 'platform', { get: () => platform, configurable: true });
})();

const hardwareCore = 2 + Math.floor(seededRandom(sessionSeed) * 6) * 2;
const deviceMem = [2, 4, 8, 16][Math.floor(seededRandom(sessionSeed * 2) * 4)];

Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => hardwareCore, configurable: true });
Object.defineProperty(navigator, 'deviceMemory', { get: () => deviceMem, configurable: true });

Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 50 + Math.floor(seededRandom(sessionSeed * 3) * 100),
  configurable: true
});

// TIMEZONE OVERRIDE
const __pinchtab_origGetTimezoneOffset = Date.prototype.getTimezoneOffset;
Object.defineProperty(Date.prototype, 'getTimezoneOffset', {
  value: function() { return window.__pinchtab_timezone || __pinchtab_origGetTimezoneOffset.call(this); },
  configurable: true
});

// BASIC PERMISSIONS (notifications only)
const originalQuery = navigator.permissions.query.bind(navigator.permissions);
navigator.permissions.query = (parameters) => (
  parameters.name === 'notifications' ?
    Promise.resolve({ state: Notification.permission === 'default' ? 'prompt' : Notification.permission, onchange: null }) :
    originalQuery(parameters)
);

// SCREEN DIMENSIONS - Headless often has weird/small values
(function() {
  const screenWidth = 1920;
  const screenHeight = 1080;
  const availHeight = 1055; // Account for taskbar
  
  Object.defineProperty(screen, 'width', { get: () => screenWidth, configurable: true });
  Object.defineProperty(screen, 'height', { get: () => screenHeight, configurable: true });
  Object.defineProperty(screen, 'availWidth', { get: () => screenWidth, configurable: true });
  Object.defineProperty(screen, 'availHeight', { get: () => availHeight, configurable: true });
  Object.defineProperty(screen, 'colorDepth', { get: () => 24, configurable: true });
  Object.defineProperty(screen, 'pixelDepth', { get: () => 24, configurable: true });
})();

// BATTERY API - Headless often lacks this
if (!navigator.getBattery) {
  navigator.getBattery = function() {
    return Promise.resolve({
      charging: true,
      chargingTime: 0,
      dischargingTime: Infinity,
      level: 1.0,
      addEventListener: function() {},
      removeEventListener: function() {}
    });
  };
}

// ═══════════════════════════════════════════════════════════════════════════
// MEDIUM LEVEL - Standard stealth (may affect error monitoring)
// ═══════════════════════════════════════════════════════════════════════════
// Adds:
// - navigator.userAgentData (Client Hints API)
// - chrome.runtime.connect/sendMessage (required by Cloudflare Turnstile)
// - chrome.csi, chrome.loadTimes, chrome.app
// - Error.prepareStackTrace protection
// - Enhanced permissions API
// - Video codec spoofing
// - maxTouchPoints
//
// ⚠️ May interfere with: Error monitoring (Sentry), extension messaging
// ═══════════════════════════════════════════════════════════════════════════

if (stealthLevel === 'medium' || stealthLevel === 'full') {

// ERROR.PREPARESTACKTRACE PROTECTION
// ⚠️ May interfere with error monitoring tools (Sentry, LogRocket, etc.)
(function() {
  const originalPrepareStackTrace = Error.prepareStackTrace;
  Object.defineProperty(Error, 'prepareStackTrace', {
    get() { return originalPrepareStackTrace; },
    set(fn) { /* block modifications to prevent CDP detection */ },
    configurable: true,
    enumerable: false
  });
})();

// ENHANCED CDP MARKER CLEANUP (includes puppeteer/playwright)
(function() {
  const patterns = [/^__puppeteer/, /^__playwright/, /^\$chrome_/];
  for (const prop of Object.getOwnPropertyNames(window)) {
    if (patterns.some(p => p.test(prop))) {
      try { delete window[prop]; } catch(e) {}
    }
  }
})();

// CHROME EXTENDED APIS
(function() {
  // chrome.runtime.connect - required by Cloudflare Turnstile
  // ⚠️ Returns dummy Port object - real extension messaging won't work
  if (!window.chrome.runtime.connect) {
    window.chrome.runtime.connect = function(extensionId, connectInfo) {
      return {
        name: connectInfo?.name || '',
        sender: undefined,
        onDisconnect: { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } },
        onMessage: { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } },
        postMessage: function() {},
        disconnect: function() {}
      };
    };
  }
  
  if (!window.chrome.runtime.sendMessage) {
    window.chrome.runtime.sendMessage = function(extensionId, message, options, callback) {
      if (typeof callback === 'function') setTimeout(callback, 0);
    };
  }
  
  window.chrome.runtime.onConnect = window.chrome.runtime.onConnect || { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } };
  window.chrome.runtime.onMessage = window.chrome.runtime.onMessage || { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } };
  
  // chrome.csi - Chrome Speed Index
  if (!window.chrome.csi) {
    window.chrome.csi = function() {
      const now = Date.now();
      return { startE: now - 500, onloadT: now - 100, pageT: now, tran: 15 };
    };
  }
  
  // chrome.loadTimes - deprecated but still checked
  if (!window.chrome.loadTimes) {
    window.chrome.loadTimes = function() {
      const now = Date.now() / 1000;
      return {
        requestTime: now - 0.5, startLoadTime: now - 0.4, commitLoadTime: now - 0.3,
        finishDocumentLoadTime: now - 0.1, finishLoadTime: now, firstPaintTime: now - 0.2,
        firstPaintAfterLoadTime: 0, navigationType: "Other", wasFetchedViaSpdy: false,
        wasNpnNegotiated: true, npnNegotiatedProtocol: "h2", wasAlternateProtocolAvailable: false, connectionInfo: "h2"
      };
    };
  }
  
  // chrome.app
  if (!window.chrome.app) {
    window.chrome.app = {
      isInstalled: false,
      InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
      RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' },
      getDetails: function() { return null; },
      getIsInstalled: function() { return false; }
    };
  }
})();

// NAVIGATOR.USERAGENTDATA - Client Hints API (required by Turnstile/CF)
(function() {
  const ua = navigator.userAgent || '';
  const chromeMatch = ua.match(/Chrome\/(\d+)/);
  const chromeVersion = chromeMatch ? chromeMatch[1] : '129';
  
  let platform = 'Windows', platformVersion = '15.0.0';
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) { platform = 'macOS'; platformVersion = '14.0.0'; }
  else if (ua.includes('Linux')) { platform = 'Linux'; platformVersion = '6.5.0'; }
  
  const brands = [
    { brand: 'Chromium', version: chromeVersion },
    { brand: 'Google Chrome', version: chromeVersion },
    { brand: 'Not=A?Brand', version: '24' }
  ];
  
  const userAgentData = {
    brands: brands,
    mobile: false,
    platform: platform,
    getHighEntropyValues: async function(hints) {
      const values = { brands: brands, mobile: false, platform: platform };
      for (const hint of hints) {
        if (hint === 'platformVersion') values.platformVersion = platformVersion;
        else if (hint === 'architecture') values.architecture = 'x86';
        else if (hint === 'model') values.model = '';
        else if (hint === 'bitness') values.bitness = '64';
        else if (hint === 'uaFullVersion') values.uaFullVersion = chromeVersion + '.0.0.0';
        else if (hint === 'fullVersionList') values.fullVersionList = brands.map(b => ({ ...b, version: b.version + '.0.0.0' }));
        else if (hint === 'wow64') values.wow64 = false;
      }
      return values;
    },
    toJSON: function() { return { brands: this.brands, mobile: this.mobile, platform: this.platform }; }
  };
  
  Object.defineProperty(Navigator.prototype, 'userAgentData', { get: () => userAgentData, configurable: true, enumerable: true });
})();

// ENHANCED PERMISSIONS API
// ⚠️ Returns fake permission states - may affect permission-dependent logic
(function() {
  const origQuery = navigator.permissions.query.bind(navigator.permissions);
  navigator.permissions.query = async function(desc) {
    const handlers = {
      'notifications': () => Notification.permission === 'default' ? 'prompt' : Notification.permission,
      'geolocation': () => 'prompt',
      'camera': () => 'prompt',
      'microphone': () => 'prompt',
      'background-sync': () => 'granted',
      'accelerometer': () => 'granted',
      'gyroscope': () => 'granted'
    };
    if (desc.name in handlers) return { state: handlers[desc.name](), onchange: null };
    return origQuery(desc);
  };
})();

// VIDEO CODEC SPOOFING
// ⚠️ Returns "probably" for common codecs - may affect codec selection logic
(function() {
  const originalCanPlayType = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function(type) {
    if (type.includes('avc1') || type.includes('h264')) return 'probably';
    if (type.includes('mp4a.40') || type.includes('aac')) return 'probably';
    if (type === 'video/mp4' || type === 'audio/mp4') return 'probably';
    if (type.includes('vp8') || type.includes('vp9') || type.includes('opus')) return 'probably';
    if (type === 'video/webm' || type === 'audio/webm') return 'probably';
    return originalCanPlayType.apply(this, arguments);
  };
})();

// maxTouchPoints
Object.defineProperty(navigator, 'maxTouchPoints', { get: () => 0, configurable: true });

} // end medium level

// ═══════════════════════════════════════════════════════════════════════════
// FULL LEVEL - Maximum stealth (may break functionality)
// ═══════════════════════════════════════════════════════════════════════════
// Adds:
// - WebGL vendor/renderer spoofing
// - Canvas fingerprint noise
// - WebRTC IP leak prevention
// - AudioContext fingerprint protection
// - Font measurement noise
//
// ⚠️ May break: WebRTC video calls, canvas-dependent apps, audio processing
// ═══════════════════════════════════════════════════════════════════════════

if (stealthLevel === 'full') {

// WebGL SPOOFING
// ⚠️ Changes reported GPU - may affect WebGL-dependent applications
(function() {
  const ua = navigator.userAgent || '';
  let vendor = 'Google Inc. (Intel)';
  let renderer = 'ANGLE (Intel, Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0)';

  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) {
    vendor = 'Google Inc. (Apple)';
    renderer = 'ANGLE (Apple, Apple M1, OpenGL 4.1)';
  } else if (ua.includes('Linux')) {
    vendor = 'Google Inc. (Intel)';
    renderer = 'ANGLE (Intel, Mesa Intel(R) UHD Graphics 630, OpenGL 4.6)';
  }

  const spoofWebGL = (proto) => {
    const getParameter = proto.getParameter;
    proto.getParameter = function(parameter) {
      if (parameter === 37445) return vendor; // UNMASKED_VENDOR
      if (parameter === 37446) return renderer; // UNMASKED_RENDERER
      return getParameter.apply(this, arguments);
    };
  };
  spoofWebGL(WebGLRenderingContext.prototype);
  if (typeof WebGL2RenderingContext !== 'undefined') spoofWebGL(WebGL2RenderingContext.prototype);
})();

// CANVAS FINGERPRINT NOISE
// ⚠️ Adds subtle pixel noise - may affect pixel-precise canvas operations
const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
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
  setTimeout(() => callback(new Blob([u8arr], {type: mime})), 5 + seededRandom(sessionSeed + 1000) * 10);
};

CanvasRenderingContext2D.prototype.getImageData = function(...args) {
  const imageData = originalGetImageData.apply(this, args);
  const pixelCount = imageData.data.length / 4;
  const noisyPixels = Math.min(10, pixelCount * 0.0001);
  for (let i = 0; i < noisyPixels; i++) {
    const pixelIndex = Math.floor(seededRandom(sessionSeed + 2000 + i) * pixelCount) * 4;
    imageData.data[pixelIndex] = Math.min(255, Math.max(0, imageData.data[pixelIndex] + (seededRandom(sessionSeed + 3000 + i) > 0.5 ? 1 : -1)));
  }
  return imageData;
};

// FONT MEASUREMENT NOISE
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

// WEBRTC IP LEAK PREVENTION
// ⚠️ Forces relay mode - direct P2P connections won't work
if (window.RTCPeerConnection) {
  const originalRTCPeerConnection = window.RTCPeerConnection;
  window.RTCPeerConnection = function(config, constraints) {
    if (config && config.iceServers) config.iceTransportPolicy = 'relay';
    return new originalRTCPeerConnection(config, constraints);
  };
  window.RTCPeerConnection.prototype = originalRTCPeerConnection.prototype;
}

// AUDIOCONTEXT FINGERPRINT PROTECTION
// ⚠️ Adds subtle frequency noise - may affect audio processing applications
if (window.AudioContext || window.webkitAudioContext) {
  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  const originalCreateOscillator = AudioContextClass.prototype.createOscillator;
  
  AudioContextClass.prototype.createOscillator = function() {
    const oscillator = originalCreateOscillator.apply(this, arguments);
    const originalConnect = oscillator.connect.bind(oscillator);
    oscillator.connect = function(dest) {
      if (dest instanceof AnalyserNode) {
        oscillator.frequency.value += seededRandom(sessionSeed + 4000) * 0.0001;
      }
      return originalConnect(dest);
    };
    return oscillator;
  };
}

} // end full level
