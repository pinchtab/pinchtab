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
const headlessMode = (typeof __pinchtab_headless !== 'undefined') ? !!__pinchtab_headless : true;
const stealthProfile = (typeof __pinchtab_profile !== 'undefined' && __pinchtab_profile && typeof __pinchtab_profile === 'object') ? __pinchtab_profile : {};
const navigatorProto = Object.getPrototypeOf(navigator) || Navigator.prototype;

function deriveNavigatorPlatformFromUA(ua) {
  if (ua.includes('Macintosh') || ua.includes('Mac OS X')) return 'MacIntel';
  if (ua.includes('Windows')) return 'Win32';
  if (ua.includes('Linux')) return ua.includes('x86_64') ? 'Linux x86_64' : 'Linux';
  return navigator.platform || 'Win32';
}

const profileUserAgent = (typeof stealthProfile.userAgent === 'string' && stealthProfile.userAgent) ? stealthProfile.userAgent : (navigator.userAgent || '');
const profileLanguage = (typeof stealthProfile.language === 'string' && stealthProfile.language) ? stealthProfile.language : (navigator.language || 'en-US');
const profileLanguages = (Array.isArray(stealthProfile.languages) && stealthProfile.languages.length) ? stealthProfile.languages.slice() : [profileLanguage, 'en'].filter((value, index, list) => !!value && list.indexOf(value) === index);
const frozenProfileLanguages = Object.freeze(profileLanguages.slice());
const profileNavigatorPlatform = (typeof stealthProfile.navigatorPlatform === 'string' && stealthProfile.navigatorPlatform) ? stealthProfile.navigatorPlatform : deriveNavigatorPlatformFromUA(profileUserAgent);
const profileUserAgentData = (stealthProfile.userAgentData && typeof stealthProfile.userAgentData === 'object') ? stealthProfile.userAgentData : null;

function definePrototypeGetter(target, name, getter) {
  if (!target || typeof getter !== 'function') return;
  const existing = Object.getOwnPropertyDescriptor(target, name);
  try {
    Object.defineProperty(target, name, {
      get: getter,
      configurable: true,
      enumerable: existing ? existing.enumerable : true
    });
  } catch (e) {}
}

function arraysEqual(a, b) {
  if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
  return a.every((value, index) => value === b[index]);
}

function brandListsEqual(a, b) {
  if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
  return a.every((value, index) => {
    const other = b[index];
    return !!other && value.brand === other.brand && value.version === other.version;
  });
}

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

function maskFunctionAsNative(fn, name) {
  return fn;
}

function wrapCallableAsNative(fn, applyHandler) {
  if (typeof fn !== 'function' || typeof applyHandler !== 'function' || typeof Proxy !== 'function') {
    return fn;
  }
  try {
    return new Proxy(fn, {
      apply(target, thisArg, args) {
        return applyHandler(target, thisArg, args || []);
      }
    });
  } catch (e) {
    return fn;
  }
}

function wrapConstructableAsNative(fn, constructHandler) {
  if (typeof fn !== 'function' || typeof constructHandler !== 'function' || typeof Proxy !== 'function') {
    return fn;
  }
  try {
    return new Proxy(fn, {
      apply(target, thisArg, args) {
        return constructHandler(target, args || [], target);
      },
      construct(target, args, newTarget) {
        return constructHandler(target, args || [], newTarget || target);
      }
    });
  } catch (e) {
    return fn;
  }
}

function sanitizeStackString(stack) {
  if (typeof stack !== 'string') return stack;
  const lines = stack.split('\n');
  if (lines.length <= 1) return stack;
  const firstLine = lines[0];
  const filtered = lines.slice(1).filter(line => !stackSanitizerPatterns.some(pattern => pattern.test(line)));
  return [firstLine].concat(filtered).join('\n');
}

function sanitizeErrorStack(error) {
  if (!error || (typeof error !== 'object' && typeof error !== 'function')) return error;
  try {
    if (typeof error.stack === 'string') {
      Object.defineProperty(error, 'stack', {
        value: sanitizeStackString(error.stack),
        configurable: true,
        writable: true,
        enumerable: false
      });
    }
  } catch (e) {}
  return error;
}

// ═══════════════════════════════════════════════════════════════════════════
// LIGHT LEVEL - Safe, no functional impact
// ═══════════════════════════════════════════════════════════════════════════
// - Relies on launch-layer webdriver behavior instead of JS navigator proxying
// - Removes CDP markers (cdc_*, __webdriver, etc.)
// - Spoofs plugins array
// - Sets navigator.languages, platform
// - Basic chrome.runtime object (light/medium only)
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

// BASIC CHROME OBJECT
if (!window.chrome) { window.chrome = {}; }
if (!window.chrome.runtime) {
  window.chrome.runtime = { onConnect: undefined, onMessage: undefined };
}

// PLUGINS / MIMETYPES - prefer native values when available; only synthesize
// a modern PDF surface if Chrome exposes an empty array.
(function() {
  try {
    if (navigator.plugins && navigator.plugins.length > 0 && navigator.mimeTypes && navigator.mimeTypes.length > 0) {
      return;
    }
  } catch (e) {}

  const fakePlugins = [
    {
      name: 'PDF Viewer',
      filename: 'internal-pdf-viewer',
      description: 'Portable Document Format',
      mimeTypes: [{ type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format' }]
    },
    {
      name: 'Chrome PDF Viewer',
      filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai',
      description: 'Portable Document Format',
      mimeTypes: [{ type: 'application/x-google-chrome-pdf', suffixes: 'pdf', description: 'Portable Document Format' }]
    },
    {
      name: 'Chromium PDF Viewer',
      filename: 'internal-pdf-viewer',
      description: 'Portable Document Format',
      mimeTypes: [{ type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format' }]
    }
  ];

  const realPluginsProto = Object.getPrototypeOf(navigator.plugins) || PluginArray.prototype;
  const realMimeTypesProto = Object.getPrototypeOf(navigator.mimeTypes) || MimeTypeArray.prototype;
  const pluginArray = Object.create(realPluginsProto);
  const mimeTypeArray = Object.create(realMimeTypesProto);

  Object.defineProperties(pluginArray, {
    item: { value: function(i) { return this[i] || null; }, writable: false },
    namedItem: { value: function(name) { return this[name] || null; }, writable: false },
    refresh: { value: function() {}, writable: false }
  });

  Object.defineProperties(mimeTypeArray, {
    item: { value: function(i) { return this[i] || null; }, writable: false },
    namedItem: { value: function(name) { return this[name] || null; }, writable: false }
  });

  const mimeTypeEntries = [];
  fakePlugins.forEach((pluginDef, pluginIndex) => {
    const plugin = Object.create(Plugin.prototype);
    const pluginMimeTypes = [];

    pluginDef.mimeTypes.forEach((mimeDef) => {
      let mimeType = mimeTypeEntries.find((entry) => entry.type === mimeDef.type);
      if (!mimeType) {
        mimeType = Object.create(MimeType.prototype, {
          type: { value: mimeDef.type, writable: false, enumerable: true },
          suffixes: { value: mimeDef.suffixes, writable: false, enumerable: true },
          description: { value: mimeDef.description, writable: false, enumerable: true }
        });
        mimeTypeEntries.push(mimeType);
      }
      pluginMimeTypes.push(mimeType);
    });

    Object.defineProperties(plugin, {
      name: { value: pluginDef.name, writable: false, enumerable: true },
      filename: { value: pluginDef.filename, writable: false, enumerable: true },
      description: { value: pluginDef.description, writable: false, enumerable: true },
      length: { value: pluginMimeTypes.length, writable: false, enumerable: true },
      item: { value: function(i) { return this[i] || null; }, writable: false },
      namedItem: { value: function(name) { return this[name] || null; }, writable: false }
    });

    pluginMimeTypes.forEach((mimeType, mimeIndex) => {
      try {
        Object.defineProperty(mimeType, 'enabledPlugin', { get: () => plugin, configurable: true });
      } catch (e) {}
      Object.defineProperty(plugin, mimeIndex, { value: mimeType, writable: false, enumerable: true });
      Object.defineProperty(plugin, mimeType.type, { value: mimeType, writable: false, enumerable: false });
    });

    Object.defineProperty(pluginArray, pluginIndex, { value: plugin, writable: false, enumerable: true });
    Object.defineProperty(pluginArray, pluginDef.name, { value: plugin, writable: false, enumerable: false });
  });

  mimeTypeEntries.forEach((mimeType, mimeIndex) => {
    Object.defineProperty(mimeTypeArray, mimeIndex, { value: mimeType, writable: false, enumerable: true });
    Object.defineProperty(mimeTypeArray, mimeType.type, { value: mimeType, writable: false, enumerable: false });
  });

  Object.defineProperty(pluginArray, 'length', { value: fakePlugins.length, writable: false, enumerable: true });
  Object.defineProperty(mimeTypeArray, 'length', { value: mimeTypeEntries.length, writable: false, enumerable: true });

  definePrototypeGetter(navigatorProto, 'plugins', () => pluginArray);
  definePrototypeGetter(navigatorProto, 'mimeTypes', () => mimeTypeArray);
})();

// NAVIGATOR PROPERTIES
(function() {
  if (navigator.language !== profileLanguage) {
    definePrototypeGetter(navigatorProto, 'language', () => profileLanguage);
  }
  if (!arraysEqual(navigator.languages || [], frozenProfileLanguages)) {
    definePrototypeGetter(navigatorProto, 'languages', () => frozenProfileLanguages);
  }
  if (navigator.platform !== profileNavigatorPlatform) {
    definePrototypeGetter(navigatorProto, 'platform', () => profileNavigatorPlatform);
  }
})();

Object.defineProperty(navigator.connection || {}, 'rtt', {
  get: () => 50 + Math.floor(seededRandom(sessionSeed * 3) * 100),
  configurable: true
});

// NETWORK INFORMATION - downlinkMax is often missing in headless.
// Define it on the prototype so page checks see a normal API surface without
// creating an own-property mismatch on navigator.connection instances.
if (navigator.connection) {
  try {
    const connectionProto = Object.getPrototypeOf(navigator.connection);
    if (connectionProto && !Object.prototype.hasOwnProperty.call(connectionProto, 'downlinkMax')) {
      Object.defineProperty(connectionProto, 'downlinkMax', {
        get: () => Infinity,
        configurable: true,
        enumerable: true
      });
    }
  } catch (e) {}
}

// SCREEN DIMENSIONS - Headless often has weird/small values
(function() {
  if (!headlessMode) return;
  const outerWidth = Math.max(window.outerWidth || 0, window.innerWidth || 0, 1280);
  const outerHeight = Math.max(window.outerHeight || 0, window.innerHeight || 0, 720);
  const screenWidth = Math.max(outerWidth + 120, 1366);
  const screenHeight = Math.max(outerHeight + 80, 768);
  const chromeHeight = Math.max(screenHeight - outerHeight, 32);
  const availHeight = Math.max(outerHeight, screenHeight - chromeHeight);
  
  Object.defineProperty(screen, 'width', { get: () => screenWidth, configurable: true });
  Object.defineProperty(screen, 'height', { get: () => screenHeight, configurable: true });
  Object.defineProperty(screen, 'availWidth', { get: () => screenWidth, configurable: true });
  Object.defineProperty(screen, 'availHeight', { get: () => availHeight, configurable: true });
  Object.defineProperty(screen, 'colorDepth', { get: () => 24, configurable: true });
  Object.defineProperty(screen, 'pixelDepth', { get: () => 24, configurable: true });
})();

if (!(window.devicePixelRatio > 0)) {
  try {
    Object.defineProperty(window, 'devicePixelRatio', { get: () => 1, configurable: true });
  } catch (e) {}
}

// BATTERY API - Headless often lacks this
if (!navigator.getBattery) {
  navigator.getBattery = wrapCallableAsNative(function getBattery() {
    return Promise.resolve({
      charging: true,
      chargingTime: 0,
      dischargingTime: Infinity,
      level: 1.0,
      addEventListener: function() {},
      removeEventListener: function() {}
    });
  }, (target, thisArg, args) => Reflect.apply(target, thisArg, args));
}

// ═══════════════════════════════════════════════════════════════════════════
// MEDIUM LEVEL - Standard stealth (may affect error monitoring)
// ═══════════════════════════════════════════════════════════════════════════
// Adds:
// - navigator.userAgentData (Client Hints API)
// - chrome.runtime.connect/sendMessage (required by Cloudflare Turnstile)
// - maxTouchPoints
//
// ⚠️ May interfere with: extension messaging
// ═══════════════════════════════════════════════════════════════════════════

if (stealthLevel === 'medium' || stealthLevel === 'full') {

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
  if (stealthLevel !== 'medium') return;
  // chrome.runtime.connect - required by Cloudflare Turnstile
  // ⚠️ Returns dummy Port object - real extension messaging won't work
  if (!window.chrome.runtime.connect) {
    const connect = function connect(extensionId, connectInfo) {
      return {
        name: connectInfo?.name || '',
        sender: undefined,
        onDisconnect: { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } },
        onMessage: { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } },
        postMessage: function() {},
        disconnect: function() {}
      };
    };
    window.chrome.runtime.connect = wrapCallableAsNative(connect, (target, thisArg, args) => Reflect.apply(target, thisArg, args));
  }
  
  if (!window.chrome.runtime.sendMessage) {
    const sendMessage = function sendMessage(extensionId, message, options, callback) {
      if (typeof callback === 'function') setTimeout(callback, 0);
    };
    window.chrome.runtime.sendMessage = wrapCallableAsNative(sendMessage, (target, thisArg, args) => Reflect.apply(target, thisArg, args));
  }
  
  window.chrome.runtime.onConnect = window.chrome.runtime.onConnect || { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } };
  window.chrome.runtime.onMessage = window.chrome.runtime.onMessage || { addListener: function() {}, removeListener: function() {}, hasListener: function() { return false; } };
  
})();

// NAVIGATOR.USERAGENTDATA - Client Hints API (required by Turnstile/CF)
(function() {
  if (!profileUserAgentData) return;

  const nativeUAData = navigator.userAgentData;
  if (nativeUAData) {
    return;
  }

  const brands = Array.isArray(profileUserAgentData.brands) ? profileUserAgentData.brands.map((brand) => ({ brand: brand.brand, version: brand.version })) : [];
  const fullVersionList = Array.isArray(profileUserAgentData.fullVersionList) ? profileUserAgentData.fullVersionList.map((brand) => ({ brand: brand.brand, version: brand.version })) : [];
  const userAgentData = {
    brands: brands,
    mobile: !!profileUserAgentData.mobile,
    platform: profileUserAgentData.platform,
    getHighEntropyValues: async function getHighEntropyValues(hints) {
      const values = { brands: brands, mobile: !!profileUserAgentData.mobile, platform: profileUserAgentData.platform };
      for (const hint of hints) {
        if (hint === 'platformVersion') values.platformVersion = profileUserAgentData.platformVersion;
        else if (hint === 'architecture') values.architecture = profileUserAgentData.architecture;
        else if (hint === 'model') values.model = profileUserAgentData.model || '';
        else if (hint === 'bitness') values.bitness = profileUserAgentData.bitness || '64';
        else if (hint === 'uaFullVersion') values.uaFullVersion = (fullVersionList[1] && fullVersionList[1].version) || '';
        else if (hint === 'fullVersionList') values.fullVersionList = fullVersionList;
        else if (hint === 'wow64') values.wow64 = !!profileUserAgentData.wow64;
      }
      return values;
    },
    toJSON: function toJSON() { return { brands: this.brands, mobile: this.mobile, platform: this.platform }; }
  };

  userAgentData.getHighEntropyValues = wrapCallableAsNative(userAgentData.getHighEntropyValues, (target, thisArg, args) => Reflect.apply(target, thisArg, args));
  userAgentData.toJSON = wrapCallableAsNative(userAgentData.toJSON, (target, thisArg, args) => Reflect.apply(target, thisArg, args));
  definePrototypeGetter(navigatorProto, 'userAgentData', () => userAgentData);
})();

// maxTouchPoints
if (navigator.maxTouchPoints !== 0) {
  definePrototypeGetter(navigatorProto, 'maxTouchPoints', () => 0);
}

// IFRAME ISOLATION - ensure same-origin iframe contexts receive the same
// medium-tier compatibility surface even for dynamic about:blank/srcdoc frames.
(function() {
  const patchIframeWindow = (frameWindow) => {
    try {
      if (!frameWindow || frameWindow === window || frameWindow.__pinchtabIframePatched) return;
      Object.defineProperty(frameWindow, '__pinchtabIframePatched', { value: true, configurable: true });

      if (window.chrome && !frameWindow.chrome) {
        frameWindow.chrome = {};
      }
      if (window.chrome && frameWindow.chrome && !frameWindow.chrome.runtime) {
        frameWindow.chrome.runtime = window.chrome.runtime;
      }
      if (window.chrome && frameWindow.chrome && !frameWindow.chrome.app && window.chrome.app) {
        frameWindow.chrome.app = window.chrome.app;
      }

      const childNavigator = frameWindow.navigator;
      if (!childNavigator) return;
      const childNavigatorProto = Object.getPrototypeOf(childNavigator);

      if (navigator.userAgentData && childNavigatorProto && !('userAgentData' in childNavigator)) {
        try {
          Object.defineProperty(childNavigatorProto, 'userAgentData', {
            get: () => navigator.userAgentData,
            configurable: true,
            enumerable: true
          });
        } catch (e) {}
      }

      if (childNavigator.permissions && typeof navigator.permissions.query === 'function') {
        try {
          childNavigator.permissions.query = wrapCallableAsNative(navigator.permissions.query, (target, thisArg, args) => {
            return Reflect.apply(target, navigator.permissions, args);
          });
          maskFunctionAsNative(childNavigator.permissions.query, 'query');
        } catch (e) {}
      }

      if (typeof navigator.maxTouchPoints !== 'undefined') {
        try {
          Object.defineProperty(childNavigator, 'maxTouchPoints', {
            get: () => navigator.maxTouchPoints,
            configurable: true
          });
        } catch (e) {}
      }
    } catch (e) {}
  };

  const patchIframeElement = (iframe) => {
    if (!iframe || iframe.__pinchtabIframeObserved) return;
    try {
      Object.defineProperty(iframe, '__pinchtabIframeObserved', { value: true, configurable: true });
    } catch (e) {}

    const apply = () => {
      try {
        const frameWindow = iframe.contentWindow;
        if (!frameWindow) return;
        const href = frameWindow.location && frameWindow.location.href ? frameWindow.location.href : '';
        if (href && href !== 'about:blank' && href !== 'about:srcdoc' && frameWindow.location.origin !== window.location.origin) {
          return;
        }
        patchIframeWindow(frameWindow);
      } catch (e) {}
    };

    iframe.addEventListener('load', apply, true);
    setTimeout(apply, 0);
  };

  const registerNode = (node) => {
    if (!node || node.nodeType !== 1) return;
    if (node.tagName === 'IFRAME') {
      patchIframeElement(node);
    }
    if (typeof node.querySelectorAll === 'function') {
      node.querySelectorAll('iframe').forEach(patchIframeElement);
    }
  };

  if (document.documentElement) {
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        mutation.addedNodes.forEach(registerNode);
      });
    });
    observer.observe(document.documentElement, { childList: true, subtree: true });
  }

  document.querySelectorAll('iframe').forEach(patchIframeElement);
})();

} // end medium level

// ═══════════════════════════════════════════════════════════════════════════
// FULL LEVEL - Maximum stealth (may break functionality)
// ═══════════════════════════════════════════════════════════════════════════
// Adds:
// - screen/window realism adjustments
// - headless-only WebGL vendor/renderer spoofing
//
// ⚠️ May break: graphics-dependent sites in headless mode
// ═══════════════════════════════════════════════════════════════════════════

if (stealthLevel === 'full') {

(function() {
  if (!headlessMode) return;
  const numericWindowValue = (value) => {
    return Number.isFinite(value) ? value : 0;
  };
  const defineWindowMetric = (name, value) => {
    try {
      Object.defineProperty(window, name, { get: () => value, configurable: true });
    } catch (e) {}
  };

  const screenX = Math.max(numericWindowValue(window.screenX), numericWindowValue(window.screenLeft));
  const screenY = Math.max(numericWindowValue(window.screenY), numericWindowValue(window.screenTop));
  const innerWidth = Math.max(numericWindowValue(window.innerWidth), 1280);
  const innerHeight = Math.max(numericWindowValue(window.innerHeight), 720);
  const widthDelta = Math.max(numericWindowValue(window.outerWidth) - innerWidth, 0);
  const heightDelta = Math.max(numericWindowValue(window.outerHeight) - innerHeight, 0);
  const outerWidth = widthDelta > 120 ? innerWidth : Math.max(numericWindowValue(window.outerWidth), innerWidth);
  const outerHeight = heightDelta > 96 ? innerHeight + 80 : Math.max(numericWindowValue(window.outerHeight), innerHeight + Math.min(heightDelta, 96));

  defineWindowMetric('screenX', screenX);
  defineWindowMetric('screenLeft', screenX);
  defineWindowMetric('screenY', screenY);
  defineWindowMetric('screenTop', screenY);
  defineWindowMetric('outerWidth', outerWidth);
  defineWindowMetric('outerHeight', outerHeight);
})();

// WebGL SPOOFING
// ⚠️ Changes reported GPU - may affect WebGL-dependent applications
(function() {
  if (!headlessMode) return;
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
    proto.getParameter = wrapCallableAsNative(getParameter, (target, thisArg, args) => {
      const parameter = args[0];
      if (parameter === 37445) return vendor; // UNMASKED_VENDOR
      if (parameter === 37446) return renderer; // UNMASKED_RENDERER
      return Reflect.apply(target, thisArg, args);
    });
  };
  spoofWebGL(WebGLRenderingContext.prototype);
  if (typeof WebGL2RenderingContext !== 'undefined') spoofWebGL(WebGL2RenderingContext.prototype);
})();

// Keep canvas/audio/webrtc/font primitives native in full mode. Public anti-bot
// sites classify these wrappers more aggressively than they reward the extra
// fingerprint noise, so the current contract favors native behavior here.

} // end full level
