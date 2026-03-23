(function() {
  const key = "__pinchtabScreencastRepaint";
  const state = globalThis[key];
  if (!state) {
    return 0;
  }

  state.refs = Math.max(0, (state.refs || 1) - 1);
  if (state.refs > 0) {
    return state.refs;
  }

  try {
    if (state.timer) {
      clearInterval(state.timer);
    }
  } catch (_) {}

  try {
    if (state.host) {
      state.host.remove();
    } else if (state.element) {
      state.element.remove();
    }
  } catch (_) {}

  delete globalThis[key];
  return 0;
})()
