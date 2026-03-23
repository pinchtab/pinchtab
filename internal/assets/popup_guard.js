(function() {
  const _split = String.prototype.split;
  const _map = Array.prototype.map;
  const _filter = Array.prototype.filter;
  const _push = Array.prototype.push;
  const _join = Array.prototype.join;
  const _trim = String.prototype.trim;
  const _toLowerCase = String.prototype.toLowerCase;
  const _call = Function.prototype.call;
  const _defineProperty = Object.defineProperty;

  const mergeWindowFeatures = function(features) {
    const parts = typeof features === 'string'
      ? _filter.call(_map.call(_split.call(features, ','), function(part) { return _trim.call(String(part)); }), function(x) { return !!x; })
      : [];
    
    let hasNoOpener = false;
    let hasNoReferrer = false;
    for (let i = 0; i < parts.length; i++) {
      const lower = _toLowerCase.call(String(parts[i]));
      if (lower === 'noopener') hasNoOpener = true;
      if (lower === 'noreferrer') hasNoReferrer = true;
    }

    if (!hasNoOpener) _push.call(parts, 'noopener');
    if (!hasNoReferrer) _push.call(parts, 'noreferrer');
    return _join.call(parts, ',');
  };

  try {
    const originalOpen = window.open;
    if (typeof originalOpen === 'function') {
      _defineProperty(window, 'open', {
        configurable: false,
        writable: false,
        value: function(url, target, features) {
          return _call.call(originalOpen, window, url, target, mergeWindowFeatures(features));
        }
      });
    }
  } catch (_) {}

  try {
    _defineProperty(window, 'opener', {
      configurable: false,
      get: function() { return null; },
      set: function() { return true; }
    });
  } catch (_) {
    try {
      window.opener = null;
    } catch (_) {}
  }
})();
