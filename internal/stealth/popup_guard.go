package stealth

const PopupGuardInitScript = `(function() {
  const mergeWindowFeatures = function(features) {
    const parts = typeof features === 'string'
      ? features.split(',').map(function(part) { return String(part).trim(); }).filter(Boolean)
      : [];
    const seen = new Set(parts.map(function(part) { return part.toLowerCase(); }));
    if (!seen.has('noopener')) {
      parts.push('noopener');
    }
    if (!seen.has('noreferrer')) {
      parts.push('noreferrer');
    }
    return parts.join(',');
  };

  try {
    const originalOpen = window.open;
    if (typeof originalOpen === 'function') {
      Object.defineProperty(window, 'open', {
        configurable: true,
        writable: true,
        value: function(url, target, features) {
          return originalOpen.call(window, url, target, mergeWindowFeatures(features));
        }
      });
    }
  } catch (_) {}

  try {
    Object.defineProperty(window, 'opener', {
      configurable: true,
      get: function() { return null; },
      set: function() { return true; }
    });
  } catch (_) {
    try {
      window.opener = null;
    } catch (_) {}
  }
})();`
