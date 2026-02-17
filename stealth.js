// Enhanced stealth script injected into every new document via page.AddScriptToEvaluateOnNewDocument.
// Hides automation indicators from sophisticated bot detection systems.

// Session-stable seed injected by Go at launch time via __pinchtab_seed.
// Falls back to a fixed value if not set (shouldn't happen in normal operation).
// This seed is constant across all page loads within the same Pinchtab session,
// so hardware fingerprint values stay consistent (as they would in a real browser).
const sessionSeed = (typeof __pinchtab_seed !== 'undefined') ? __pinchtab_seed : 42;

// Mulberry32 PRNG — simple, fast, uniform distribution, deterministic.
// Much better than Math.sin() which produces visible patterns.
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

// Hide webdriver flag completely
Object.defineProperty(navigator, 'webdriver', { 
  get: () => undefined,
  configurable: true 
});

// Remove automation-related window properties
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Array;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Promise;
delete window.cdc_adoQpoasnfa76pfcZLmcfl_Symbol;

// Fix chrome runtime to look more realistic
if (!window.chrome) { window.chrome = {}; }
if (!window.chrome.runtime) {
  window.chrome.runtime = {
    onConnect: undefined,
    onMessage: undefined
  };
}

// Enhanced permissions API
const originalQuery = window.navigator.permissions.query;
window.navigator.permissions.query = (parameters) => (
  parameters.name === 'notifications' ?
    Promise.resolve({ state: Notification.permission }) :
    originalQuery(parameters)
);

// Realistic plugins array (common plugins)
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

// Realistic language preferences
Object.defineProperty(navigator, 'languages', {
  get: () => ['en-US', 'en'],
});

// Hardware values defined later with proper seeding

// Platform information (consistent with User-Agent)
Object.defineProperty(navigator, 'platform', {
  get: () => 'MacIntel',
});

// Fix missing properties that indicate automation
Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 100,
});

// Spoof WebGL for fingerprint resistance
const getParameter = WebGLRenderingContext.prototype.getParameter;
WebGLRenderingContext.prototype.getParameter = function(parameter) {
  // Spoof common WebGL parameters that are used for fingerprinting
  if (parameter === 37445) { // UNMASKED_VENDOR_WEBGL
    return 'Intel Inc.';
  }
  if (parameter === 37446) { // UNMASKED_RENDERER_WEBGL
    return 'Intel Iris OpenGL Engine';
  }
  return getParameter.apply(this, arguments);
};

// Mouse event realism (add slight randomness)
// Mouse event handling - removed wrapper as it can break legitimate functionality

// Canvas fingerprinting noise
const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
const originalToBlob = HTMLCanvasElement.prototype.toBlob;
const originalGetImageData = CanvasRenderingContext2D.prototype.getImageData;

HTMLCanvasElement.prototype.toDataURL = function(...args) {
  const context = this.getContext('2d');
  if (context && this.width > 0 && this.height > 0) {
    // Create a temporary canvas for noise injection
    const tempCanvas = document.createElement('canvas');
    tempCanvas.width = this.width;
    tempCanvas.height = this.height;
    const tempCtx = tempCanvas.getContext('2d');
    
    // Copy current canvas
    tempCtx.drawImage(this, 0, 0);
    
    // Add imperceptible noise to a few pixels
    const imageData = tempCtx.getImageData(0, 0, this.width, this.height);
    const pixelCount = Math.min(10, Math.floor(imageData.data.length / 400));
    
    for (let i = 0; i < pixelCount; i++) {
      const idx = Math.floor(seededRandom(sessionSeed + i) * (imageData.data.length / 4)) * 4;
      // Tiny 1-bit changes
      if (imageData.data[idx] < 255) imageData.data[idx] += 1;
      if (imageData.data[idx + 1] < 255) imageData.data[idx + 1] += 1;
    }
    
    tempCtx.putImageData(imageData, 0, 0);
    return originalToDataURL.apply(tempCanvas, args);
  }
  
  return originalToDataURL.apply(this, args);
};

HTMLCanvasElement.prototype.toBlob = function(callback, type, quality) {
  // Use toDataURL which has noise, then convert to blob
  const dataURL = this.toDataURL(type, quality);
  const arr = dataURL.split(',');
  const mime = arr[0].match(/:(.*?);/)[1];
  const bstr = atob(arr[1]);
  let n = bstr.length;
  const u8arr = new Uint8Array(n);
  while(n--){
    u8arr[n] = bstr.charCodeAt(n);
  }
  const blob = new Blob([u8arr], {type: mime});
  
  // Add human-like async delay
  setTimeout(() => callback(blob), 5 + seededRandom(sessionSeed + 1000) * 10);
};

CanvasRenderingContext2D.prototype.getImageData = function(...args) {
  const imageData = originalGetImageData.apply(this, args);
  
  // Only add noise to a few pixels to maintain visual integrity
  // This is enough to change fingerprint without breaking functionality
  const pixelCount = imageData.data.length / 4;
  const noisyPixels = Math.min(10, pixelCount * 0.0001); // Very few pixels
  
  for (let i = 0; i < noisyPixels; i++) {
    const pixelIndex = Math.floor(Math.random() * pixelCount) * 4;
    // Tiny changes that won't be visible
    imageData.data[pixelIndex] = Math.min(255, Math.max(0, imageData.data[pixelIndex] + (Math.random() > 0.5 ? 1 : -1)));
  }
  
  return imageData;
};

// Font fingerprinting protection
const originalMeasureText = CanvasRenderingContext2D.prototype.measureText;
CanvasRenderingContext2D.prototype.measureText = function(text) {
  const metrics = originalMeasureText.apply(this, arguments);
  const noise = 0.0001 + (seededRandom(sessionSeed + text.length) * 0.0002);
  
  // Return a proper TextMetrics object
  return new Proxy(metrics, {
    get(target, prop) {
      if (prop === 'width') {
        return target.width * (1 + noise);
      }
      return target[prop];
    }
  });
};

// WebRTC IP leak prevention - hide local IPs
if (window.RTCPeerConnection) {
  const originalRTCPeerConnection = window.RTCPeerConnection;
  window.RTCPeerConnection = function(config, constraints) {
    // Force TURN relay to hide local IPs
    if (config && config.iceServers) {
      config.iceTransportPolicy = 'relay';
    }
    return new originalRTCPeerConnection(config, constraints);
  };
  window.RTCPeerConnection.prototype = originalRTCPeerConnection.prototype;
}

// Timezone spoofing - configurable via window.__pinchtab_timezone
Object.defineProperty(Date.prototype, 'getTimezoneOffset', {
  value: function() { 
    return window.__pinchtab_timezone || new Date().getTimezoneOffset();
  }
});

// Use the already-defined sessionSeed and seededRandom from top of file
const hardwareCore = 2 + Math.floor(seededRandom(sessionSeed) * 6) * 2; // 2,4,6,8,10,12
const deviceMem = [2, 4, 8, 16][Math.floor(seededRandom(sessionSeed * 2) * 4)];

Object.defineProperty(navigator, 'hardwareConcurrency', {
  get: () => hardwareCore
});

Object.defineProperty(navigator, 'deviceMemory', {
  get: () => deviceMem
});
