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

// webdriver: DO NOT override navigator.webdriver here.
// With --enable-automation=false (set in init.go), Chrome natively has webdriver=false.
// Any JS override triggers CreepJS lie detector (lieProps['Navigator.webdriver']).
// "Less is more" — the native false value passes all 3 CreepJS conditions:
//   (1) webdriver === undefined → false (it's false, not undefined)
//   (2) !!webdriver → false
//   (3) lieProps → false (no tampering)
// See: d_20260318_049

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

// Create a proper PluginArray that passes all three sannysoft checks:
//   1. navigator.plugins instanceof PluginArray
//   2. navigator.plugins.length > 0
//   3. navigator.plugins[0].toString() === '[object Plugin]'
(function() {
  const fakePlugins = [
    { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer',             description: 'Portable Document Format' },
    { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '' },
    { name: 'Native Client',     filename: 'internal-nacl-plugin',             description: '' },
  ];

  function makePlugin(p) {
    // Use Plugin.prototype if available (gives native instanceof check).
    // Fall back to a plain object with explicit Symbol.toStringTag so that
    // plugin.toString() returns '[object Plugin]' either way.
    let base = {};
    try { if (typeof Plugin !== 'undefined') base = Object.create(Plugin.prototype); } catch(e) {}
    Object.defineProperty(base, Symbol.toStringTag, { value: 'Plugin', configurable: false });
    ['name','filename','description'].forEach(function(k) {
      Object.defineProperty(base, k, { value: p[k], writable: false, enumerable: true, configurable: false });
    });
    Object.defineProperty(base, 'length',    { value: 1,            writable: false, enumerable: true });
    Object.defineProperty(base, 'item',      { value: function() { return null; }, writable: false });
    Object.defineProperty(base, 'namedItem', { value: function() { return null; }, writable: false });
    return base;
  }

  // Borrow the real PluginArray prototype so instanceof PluginArray === true.
  const realProto = Object.getPrototypeOf(navigator.plugins);
  const arr = Object.create(realProto);
  Object.defineProperty(arr, 'length',    { value: fakePlugins.length, writable: false, enumerable: true });
  Object.defineProperty(arr, 'item',      { value: function(i) { return arr[i] || null; }, writable: false });
  Object.defineProperty(arr, 'namedItem', { value: function(n) {
    for (var i = 0; i < fakePlugins.length; i++) { if (arr[i] && arr[i].name === n) return arr[i]; }
    return null;
  }, writable: false });
  Object.defineProperty(arr, 'refresh',   { value: function() {}, writable: false });

  fakePlugins.forEach(function(p, i) {
    var plugin = makePlugin(p);
    Object.defineProperty(arr, i,     { value: plugin, writable: false, enumerable: true });
    Object.defineProperty(arr, p.name, { value: plugin, writable: false, enumerable: false });
  });

  Object.defineProperty(navigator, 'plugins', { get: function() { return arr; }, configurable: true });
})();

Object.defineProperty(navigator, 'languages', {
  get: () => ['en-US', 'en'],
});


Object.defineProperty(navigator, 'platform', {
  get: () => 'MacIntel',
});

Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 100,
});

// Fix noDownlinkMax: define on prototype so it exists as a proper API property.
// Headless Chrome's NetworkInformation prototype lacks downlinkMax entirely.
// Real Chrome on WiFi reports Infinity.
if (navigator.connection) {
  try {
    const connProto = Object.getPrototypeOf(navigator.connection);
    if (!connProto.hasOwnProperty('downlinkMax')) {
      Object.defineProperty(connProto, 'downlinkMax', {
        get: () => Infinity,
        configurable: true,
        enumerable: true
      });
    }
  } catch(e) {}
}

const stealthLevel = (typeof __pinchtab_stealth_level !== 'undefined') ? __pinchtab_stealth_level : 'light';

if (stealthLevel === 'full') {

// Fix screen dimensions: headless Chrome reports screen as 800×600 even in new mode.
// Use window.outerWidth/Height as the screen dimensions (they come from --window-size).
// Also set devicePixelRatio to 2 on macOS (Retina) for consistency.
(function() {
  // Common screen resolutions — sorted ascending by width
  const screens = [
    { w: 1440, h: 900 },  { w: 1536, h: 864 },  { w: 1680, h: 1050 },
    { w: 1920, h: 1080 }, { w: 2560, h: 1440 }, { w: 3840, h: 2160 },
  ];
  const ow = window.outerWidth || 1280;
  const oh = window.outerHeight || 800;
  // Screen must be STRICTLY larger than window (real monitors > browser windows).
  // Filter pool to entries larger than current window, pick one by seed.
  const valid = screens.filter(s => s.w > ow && s.h > oh);
  const pool = valid.length > 0 ? valid : [screens[screens.length - 1]];
  const picked = pool[Math.floor(seededRandom(sessionSeed + 9999) * pool.length)];
  const sw = picked.w;
  const sh = picked.h;
  const dpr = (navigator.platform === 'MacIntel') ? 2 : 1;

  const overrides = {
    width: sw, height: sh, availWidth: sw, availHeight: sh - 25,
    colorDepth: 24, pixelDepth: 24
  };
  // Override directly on window.screen (own properties, not prototype-inherited)
  for (const [key, value] of Object.entries(overrides)) {
    try {
      Object.defineProperty(window.screen, key, { get: () => value, configurable: true });
    } catch(e) {}
  }
  // Fix devicePixelRatio
  try {
    Object.defineProperty(window, 'devicePixelRatio', { get: () => dpr, configurable: true });
  } catch(e) {}
  // Fix screenX/screenY for taskbar simulation (macOS has 25px menu bar)
  try {
    Object.defineProperty(window, 'screenY', { get: () => 25, configurable: true });
  } catch(e) {}
})();

// Fix hasKnownBgColor: headless Chrome resolves CSS system colors like 'ActiveText'
// to hardcoded defaults (e.g. rgb(255,0,0)) instead of querying the OS theme.
// CSS properties aren't JS descriptors — they're handled by C++ code, so we can't
// intercept the setter. Instead, override getComputedStyle to patch the returned
// value when the element's inline style contains a system color keyword.
(function() {
  const systemColorMap = {
    'activetext': 'rgb(0, 102, 204)',
    'accentcolor': 'rgb(0, 122, 255)',
    'accentcolortext': 'rgb(255, 255, 255)',
  };
  const colorProps = ['color', 'backgroundColor', 'borderColor'];
  const origGCS = window.getComputedStyle;
  window.getComputedStyle = function(element, pseudoElt) {
    const style = origGCS.call(this, element, pseudoElt);
    if (element && element.style) {
      for (const prop of colorProps) {
        const inlineVal = element.style.getPropertyValue(prop === 'backgroundColor' ? 'background-color' : prop === 'borderColor' ? 'border-color' : prop);
        const key = inlineVal ? inlineVal.toLowerCase().trim() : '';
        if (systemColorMap[key]) {
          try {
            Object.defineProperty(style, prop, {
              get: () => systemColorMap[key],
              configurable: true,
            });
          } catch(e) {}
        }
      }
    }
    return style;
  };
  // Preserve native toString to avoid lie detection
  window.getComputedStyle.toString = origGCS.toString.bind(origGCS);
  Object.defineProperty(window.getComputedStyle, 'name', { value: 'getComputedStyle' });
  Object.defineProperty(window.getComputedStyle, 'length', { value: origGCS.length });
})();

// WebGL: DO NOT spoof UNMASKED_RENDERER/VENDOR here.
// The subprocess architecture (BRIDGE_ONLY=1) gives Chrome real Metal GPU access.
// Real GPU info (Apple M2 Metal) is fine — it's what a real Mac reports.
// Spoofing to "Intel Iris" caused hasBadWebGL detection in CreepJS because
// the page reported "Intel Iris" but the Service Worker reported real "Apple M2"
// (workers access WebGL via OffscreenCanvas, bypassing our page-level spoof).
// The original spoof was needed to hide SwiftShader (CPU renderer) in the old
// monolithic mode. With subprocess architecture, it's counterproductive.
//
// If SwiftShader IS detected (fallback/CI), the old patchWebGLGetParameter can
// be restored here. Check: ANGLE (Google, ..., SwiftShader) in the renderer string.

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
      // Skip fully transparent pixels — modifying them creates detectable artifacts
      if (imageData.data[idx] === 0 && imageData.data[idx+1] === 0 && 
          imageData.data[idx+2] === 0 && imageData.data[idx+3] === 0) continue;
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
    // Skip fully transparent pixels
    if (imageData.data[pixelIndex] === 0 && imageData.data[pixelIndex+1] === 0 && 
        imageData.data[pixelIndex+2] === 0 && imageData.data[pixelIndex+3] === 0) continue;
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


