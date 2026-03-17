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

// ═══════════════════════════════════════════════════════════════════════════
// CDP MARKER CLEANUP - Remove automation traces from window object
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  // Known CDP/automation markers
  const markerPatterns = [
    /^cdc_/,
    /^\$cdc_/,
    /^__webdriver/,
    /^__selenium/,
    /^__driver/,
    /^\$chrome_/,
    /^__puppeteer/,
    /^__playwright/
  ];
  
  for (const prop of Object.getOwnPropertyNames(window)) {
    if (markerPatterns.some(p => p.test(prop))) {
      try { delete window[prop]; } catch(e) {}
    }
  }
  
  // Legacy specific markers
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
  delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;
})();

// ═══════════════════════════════════════════════════════════════════════════
// ERROR.PREPARESTACKTRACE PROTECTION - Prevent CDP detection via stack traces
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  const originalPrepareStackTrace = Error.prepareStackTrace;
  Object.defineProperty(Error, 'prepareStackTrace', {
    get() { return originalPrepareStackTrace; },
    set(fn) { /* block modifications that could detect CDP */ },
    configurable: true,
    enumerable: false
  });
})();

// ═══════════════════════════════════════════════════════════════════════════
// WEBDRIVER EVASION - Hide the property completely
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  const proto = Object.getPrototypeOf(navigator);
  
  try { delete navigator.webdriver; } catch(e) {}
  try { delete proto.webdriver; } catch(e) {}
  
  const desc = { 
    get: () => undefined, 
    configurable: false, 
    enumerable: false
  };
  
  if ('webdriver' in navigator) {
    try { Object.defineProperty(navigator, 'webdriver', desc); } catch(e) {}
  }
  if ('webdriver' in proto) {
    try { Object.defineProperty(proto, 'webdriver', desc); } catch(e) {}
  }
})();

// ═══════════════════════════════════════════════════════════════════════════
// CHROME OBJECT - Full API spoofing (required by Cloudflare Turnstile)
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  if (!window.chrome) { window.chrome = {}; }
  
  // chrome.runtime with connect() - required by Turnstile
  if (!window.chrome.runtime) {
    window.chrome.runtime = {};
  }
  
  if (!window.chrome.runtime.connect) {
    window.chrome.runtime.connect = function(extensionId, connectInfo) {
      return {
        name: connectInfo?.name || '',
        sender: undefined,
        onDisconnect: { 
          addListener: function() {}, 
          removeListener: function() {},
          hasListener: function() { return false; },
          hasListeners: function() { return false; }
        },
        onMessage: { 
          addListener: function() {}, 
          removeListener: function() {},
          hasListener: function() { return false; },
          hasListeners: function() { return false; }
        },
        postMessage: function() {},
        disconnect: function() {}
      };
    };
  }
  
  if (!window.chrome.runtime.sendMessage) {
    window.chrome.runtime.sendMessage = function(extensionId, message, options, callback) {
      if (typeof callback === 'function') {
        setTimeout(callback, 0);
      }
    };
  }
  
  if (!window.chrome.runtime.onConnect) {
    window.chrome.runtime.onConnect = {
      addListener: function() {},
      removeListener: function() {},
      hasListener: function() { return false; }
    };
  }
  
  if (!window.chrome.runtime.onMessage) {
    window.chrome.runtime.onMessage = {
      addListener: function() {},
      removeListener: function() {},
      hasListener: function() { return false; }
    };
  }
  
  // chrome.csi() - Chrome Speed Index (some sites check this)
  if (!window.chrome.csi) {
    window.chrome.csi = function() {
      const now = Date.now();
      return { 
        startE: now - 500, 
        onloadT: now - 100, 
        pageT: now, 
        tran: 15 
      };
    };
  }
  
  // chrome.loadTimes() - deprecated but still checked
  if (!window.chrome.loadTimes) {
    window.chrome.loadTimes = function() {
      const now = Date.now() / 1000;
      return {
        requestTime: now - 0.5,
        startLoadTime: now - 0.4,
        commitLoadTime: now - 0.3,
        finishDocumentLoadTime: now - 0.1,
        finishLoadTime: now,
        firstPaintTime: now - 0.2,
        firstPaintAfterLoadTime: 0,
        navigationType: "Other",
        wasFetchedViaSpdy: false,
        wasNpnNegotiated: true,
        npnNegotiatedProtocol: "h2",
        wasAlternateProtocolAvailable: false,
        connectionInfo: "h2"
      };
    };
  }
  
  // chrome.app object
  if (!window.chrome.app) {
    window.chrome.app = {
      isInstalled: false,
      InstallState: { 
        DISABLED: 'disabled', 
        INSTALLED: 'installed', 
        NOT_INSTALLED: 'not_installed' 
      },
      RunningState: { 
        CANNOT_RUN: 'cannot_run', 
        READY_TO_RUN: 'ready_to_run', 
        RUNNING: 'running' 
      },
      getDetails: function() { return null; },
      getIsInstalled: function() { return false; }
    };
  }
})();

// ═══════════════════════════════════════════════════════════════════════════
// PERMISSIONS API - Enhanced to handle multiple permission types
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  const originalQuery = navigator.permissions.query.bind(navigator.permissions);
  
  navigator.permissions.query = async function(desc) {
    // Handle known permissions that might be inconsistent in automation
    const permissionHandlers = {
      'notifications': () => Notification.permission === 'default' ? 'prompt' : Notification.permission,
      'geolocation': () => 'prompt',
      'camera': () => 'prompt',
      'microphone': () => 'prompt',
      'background-sync': () => 'granted',
      'accelerometer': () => 'granted',
      'gyroscope': () => 'granted',
      'magnetometer': () => 'granted'
    };
    
    if (desc.name in permissionHandlers) {
      const state = permissionHandlers[desc.name]();
      return { state: state, onchange: null };
    }
    
    return originalQuery(desc);
  };
})();

// ═══════════════════════════════════════════════════════════════════════════
// NAVIGATOR.USERAGENTDATA - Client Hints API (required by Turnstile/CF)
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  const ua = navigator.userAgent || '';
  
  // Extract Chrome version from UA
  const chromeMatch = ua.match(/Chrome\/(\d+)/);
  const chromeVersion = chromeMatch ? chromeMatch[1] : '129';
  
  // Determine platform from UA
  let platform = 'Windows';
  let platformVersion = '15.0.0';
  let architecture = 'x86';
  let bitness = '64';
  
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) {
    platform = 'macOS';
    platformVersion = '14.0.0';
  } else if (ua.includes('Linux')) {
    platform = 'Linux';
    platformVersion = '6.5.0';
  }
  
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
      const values = {
        brands: brands,
        mobile: false,
        platform: platform
      };
      
      for (const hint of hints) {
        switch(hint) {
          case 'platformVersion': values.platformVersion = platformVersion; break;
          case 'architecture': values.architecture = architecture; break;
          case 'model': values.model = ''; break;
          case 'bitness': values.bitness = bitness; break;
          case 'uaFullVersion': values.uaFullVersion = chromeVersion + '.0.0.0'; break;
          case 'fullVersionList': values.fullVersionList = brands.map(b => ({ ...b, version: b.version + '.0.0.0' })); break;
          case 'wow64': values.wow64 = false; break;
        }
      }
      
      return values;
    },
    
    toJSON: function() {
      return {
        brands: this.brands,
        mobile: this.mobile,
        platform: this.platform
      };
    }
  };
  
  Object.defineProperty(Navigator.prototype, 'userAgentData', {
    get: () => userAgentData,
    configurable: true,
    enumerable: true
  });
})();

// ═══════════════════════════════════════════════════════════════════════════
// PLUGINS ARRAY - Proper PluginArray that passes instanceof checks
// ═══════════════════════════════════════════════════════════════════════════
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

// ═══════════════════════════════════════════════════════════════════════════
// NAVIGATOR PROPERTIES - Languages, Platform, Hardware
// ═══════════════════════════════════════════════════════════════════════════
Object.defineProperty(navigator, 'languages', {
  get: () => ['en-US', 'en'],
  configurable: true
});

// Derive platform from user agent
(function() {
  const ua = navigator.userAgent || '';
  let platform = 'Win32';
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) {
    platform = 'MacIntel';
  } else if (ua.includes('Linux')) {
    platform = ua.includes('x86_64') || ua.includes('amd64') ? 'Linux x86_64' : 'Linux';
  }
  Object.defineProperty(navigator, 'platform', {
    get: () => platform,
    configurable: true
  });
})();

// Hardware fingerprint (seeded for consistency)
const hardwareCore = 2 + Math.floor(seededRandom(sessionSeed) * 6) * 2;
const deviceMem = [2, 4, 8, 16][Math.floor(seededRandom(sessionSeed * 2) * 4)];

Object.defineProperty(navigator, 'hardwareConcurrency', {
  get: () => hardwareCore,
  configurable: true
});

Object.defineProperty(navigator, 'deviceMemory', {
  get: () => deviceMem,
  configurable: true
});

// maxTouchPoints - 0 for desktop (consistent with non-touch device)
Object.defineProperty(navigator, 'maxTouchPoints', {
  get: () => 0,
  configurable: true
});

// Connection RTT
Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 50 + Math.floor(seededRandom(sessionSeed * 3) * 100),
  configurable: true
});

// ═══════════════════════════════════════════════════════════════════════════
// VIDEO CODEC SPOOFING - Return consistent codec support
// ═══════════════════════════════════════════════════════════════════════════
(function() {
  const originalCanPlayType = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function(type) {
    // Common codecs that should be supported
    if (type.includes('avc1') || type.includes('h264')) return 'probably';
    if (type.includes('mp4a.40') || type.includes('aac')) return 'probably';
    if (type === 'video/mp4') return 'probably';
    if (type === 'audio/mp4') return 'probably';
    if (type.includes('vp8') || type.includes('vp9')) return 'probably';
    if (type.includes('opus')) return 'probably';
    if (type === 'video/webm') return 'probably';
    if (type === 'audio/webm') return 'probably';
    return originalCanPlayType.apply(this, arguments);
  };
})();

// ═══════════════════════════════════════════════════════════════════════════
// TIMEZONE
// ═══════════════════════════════════════════════════════════════════════════
const __pinchtab_origGetTimezoneOffset = Date.prototype.getTimezoneOffset;
Object.defineProperty(Date.prototype, 'getTimezoneOffset', {
  value: function() {
    return window.__pinchtab_timezone || __pinchtab_origGetTimezoneOffset.call(this);
  },
  configurable: true
});

// ═══════════════════════════════════════════════════════════════════════════
// FULL STEALTH MODE - Additional fingerprint protections
// ═══════════════════════════════════════════════════════════════════════════
const stealthLevel = (typeof __pinchtab_stealth_level !== 'undefined') ? __pinchtab_stealth_level : 'light';

if (stealthLevel === 'full') {

// WebGL spoofing (both contexts)
(function() {
  const spoofWebGL = (proto) => {
    const getParameter = proto.getParameter;
    proto.getParameter = function(parameter) {
      // UNMASKED_VENDOR_WEBGL
      if (parameter === 37445) return 'Google Inc. (Intel)';
      // UNMASKED_RENDERER_WEBGL
      if (parameter === 37446) return 'ANGLE (Intel, Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0)';
      return getParameter.apply(this, arguments);
    };
  };
  spoofWebGL(WebGLRenderingContext.prototype);
  if (typeof WebGL2RenderingContext !== 'undefined') {
    spoofWebGL(WebGL2RenderingContext.prototype);
  }
})();

// Canvas fingerprint noise
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
    const pixelIndex = Math.floor(seededRandom(sessionSeed + 2000 + i) * pixelCount) * 4;
    imageData.data[pixelIndex] = Math.min(255, Math.max(0, imageData.data[pixelIndex] + (seededRandom(sessionSeed + 3000 + i) > 0.5 ? 1 : -1)));
  }
  return imageData;
};

// Font measurement noise
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

// WebRTC IP leak prevention
if (window.RTCPeerConnection) {
  const originalRTCPeerConnection = window.RTCPeerConnection;
  window.RTCPeerConnection = function(config, constraints) {
    if (config && config.iceServers) config.iceTransportPolicy = 'relay';
    return new originalRTCPeerConnection(config, constraints);
  };
  window.RTCPeerConnection.prototype = originalRTCPeerConnection.prototype;
}

// AudioContext fingerprint protection
if (window.AudioContext || window.webkitAudioContext) {
  const AudioContextClass = window.AudioContext || window.webkitAudioContext;
  const originalCreateOscillator = AudioContextClass.prototype.createOscillator;
  const originalCreateDynamicsCompressor = AudioContextClass.prototype.createDynamicsCompressor;
  
  AudioContextClass.prototype.createOscillator = function() {
    const oscillator = originalCreateOscillator.apply(this, arguments);
    const originalConnect = oscillator.connect.bind(oscillator);
    oscillator.connect = function(dest) {
      // Add subtle noise to audio fingerprinting attempts
      if (dest instanceof AnalyserNode) {
        const noise = seededRandom(sessionSeed + 4000) * 0.0001;
        oscillator.frequency.value += noise;
      }
      return originalConnect(dest);
    };
    return oscillator;
  };
}

} // end stealthLevel === 'full'
