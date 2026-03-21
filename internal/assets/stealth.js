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

Object.defineProperty(navigator, 'webdriver', { 
  get: () => undefined,
  configurable: true 
});

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

Object.defineProperty(navigator, 'plugins', {
  get: () => [{
    name: 'Chrome PDF Plugin',
    filename: 'internal-pdf-viewer',
    description: 'Portable Document Format'
  }, {
    name: 'Chrome PDF Viewer',
    filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai',
    description: ''
  }, {
    name: 'Native Client',
    filename: 'internal-nacl-plugin',
    description: ''
  }],
});

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


// ========== Layer 2: Deep Anti-Detection Patches ==========

// --- 2a: Iframe Isolation ---
// Anti-bot scripts create about:blank iframes and check navigator.webdriver
// inside them BEFORE our stealth script can inject. This MutationObserver
// intercepts iframe creation and patches them instantly.
(function() {
  function patchFrame(frame) {
    try {
      const win = frame.contentWindow;
      if (win && win.navigator) {
        Object.defineProperty(win.navigator, 'webdriver', {
          get: () => undefined, configurable: true
        });
      }
    } catch(e) {} // cross-origin iframes will throw, that's fine
  }

  // Patch any existing iframes
  document.querySelectorAll('iframe').forEach(patchFrame);

  // Watch for new iframes
  const obs = new MutationObserver(mutations => {
    for (const m of mutations) {
      for (const node of m.addedNodes) {
        if (node.tagName === 'IFRAME') patchFrame(node);
        if (node.querySelectorAll) {
          node.querySelectorAll('iframe').forEach(patchFrame);
        }
      }
    }
  });
  if (document.documentElement) {
    obs.observe(document.documentElement, { childList: true, subtree: true });
  }
})();

// --- 2b: Error.stack Sanitization ---
// When CDP-injected scripts throw, their stack traces contain revealing
// references like __puppeteer_evaluation_script__, __chromedp_eval,
// pptr:evaluate, or devtools:// URLs. Sanitize them.
(function() {
  const OrigError = Error;
  const origDesc = Object.getOwnPropertyDescriptor(OrigError.prototype, 'stack');
  if (origDesc && origDesc.get) {
    Object.defineProperty(OrigError.prototype, 'stack', {
      get() {
        const stack = origDesc.get.call(this);
        if (typeof stack !== 'string') return stack;
        return stack.split('\n').filter(line =>
          !line.includes('__puppeteer') &&
          !line.includes('__chromedp') &&
          !line.includes('pptr:') &&
          !line.includes('://devtools') &&
          !line.includes('__pinchtab')
        ).join('\n');
      },
      set(v) { if (origDesc.set) origDesc.set.call(this, v); },
      configurable: true
    });
  }
})();

// --- 2c: Native Function.toString Masking ---
// When we override navigator properties with Object.defineProperty,
// the getter's toString() returns JS source instead of "[native code]".
// Advanced anti-bot checks detect this. Patch toString for our overrides.
(function() {
  const nativeToString = Function.prototype.toString;
  const overrides = new Map();

  // Register a function as "native-looking"
  function maskAsNative(fn, name) {
    overrides.set(fn, `function ${name || ''}() { [native code] }`);
  }

  Function.prototype.toString = function() {
    if (overrides.has(this)) return overrides.get(this);
    return nativeToString.call(this);
  };
  // Make the toString override itself look native
  maskAsNative(Function.prototype.toString, 'toString');

  // Re-apply navigator.webdriver with a maskable getter
  const wdGetter = function() { return undefined; };
  maskAsNative(wdGetter, 'get webdriver');
  Object.defineProperty(navigator, 'webdriver', {
    get: wdGetter,
    configurable: true
  });

  // Expose utility for other scripts (e.g., fingerprint rotation)
  window.__pinchtab_maskAsNative = maskAsNative;
})();
