(function() {
  const key = "__pinchtabScreencastRepaint";
  const state = globalThis[key] || (globalThis[key] = { refs: 0, tick: 0 });
  state.refs += 1;
  if (state.refs > 1) {
    return state.refs;
  }

  const host = document.createElement("div");
  host.setAttribute("aria-hidden", "true");
  host.style.cssText = "position:fixed;left:0;top:0;width:0;height:0;overflow:hidden;pointer-events:none;z-index:2147483647;";

  const shadow = host.attachShadow({ mode: "closed" });
  const el = document.createElement("div");
  el.style.cssText = "position:fixed;left:0;top:0;width:1px;height:1px;pointer-events:none;background:rgba(0,0,0,0.001);opacity:0.999;transform:translateZ(0) scale(1);will-change:opacity,transform;";
  shadow.appendChild(el);

  const mount = () => {
    const parent = document.body || document.documentElement;
    if (parent && !host.isConnected) {
      parent.appendChild(host);
    }
  };

  const tick = () => {
    mount();
    state.tick = (state.tick + 1) % 2;
    el.style.opacity = state.tick === 0 ? "0.999" : "1";
    el.style.transform = state.tick === 0 ? "translateZ(0) scale(1)" : "translateZ(0) scale(1.0001)";
  };

  mount();
  tick();

  state.host = host;
  state.element = el;
  state.mount = mount;
  state.timer = setInterval(tick, 250);
  return state.refs;
})()
