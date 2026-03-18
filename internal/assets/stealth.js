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

// Hide webdriver: try deleting first, then make non-enumerable so
// 'webdriver' in navigator returns false (matching non-headless Chrome).
(function() {
  const proto = Object.getPrototypeOf(navigator);
  try { delete navigator.webdriver; } catch(e) {}
  try { delete proto.webdriver; } catch(e) {}
  const desc = { get: () => undefined, configurable: false, enumerable: false };
  if ('webdriver' in navigator) {
    try { Object.defineProperty(navigator, 'webdriver', desc); } catch(e) {}
  }
  if ('webdriver' in proto) {
    try { Object.defineProperty(proto, 'webdriver', desc); } catch(e) {}
  }
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

const stealthLevel = (typeof __pinchtab_stealth_level !== 'undefined') ? __pinchtab_stealth_level : 'light';

if (stealthLevel === 'full') {

// Patch both WebGL1 and WebGL2 — modern Chrome defaults to WebGL2.
// Without patching WebGL2, the SwiftShader renderer leaks via WEBGL_debug_renderer_info.
function patchWebGLGetParameter(ctx) {
  if (!ctx) return;
  const orig = ctx.prototype.getParameter;
  ctx.prototype.getParameter = function(parameter) {
    if (parameter === 37445) return 'Intel Inc.';          // UNMASKED_VENDOR_WEBGL
    if (parameter === 37446) return 'Intel Iris OpenGL Engine'; // UNMASKED_RENDERER_WEBGL
    return orig.apply(this, arguments);
  };
}
patchWebGLGetParameter(WebGLRenderingContext);
if (typeof WebGL2RenderingContext !== 'undefined') {
  patchWebGLGetParameter(WebGL2RenderingContext);
}

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
