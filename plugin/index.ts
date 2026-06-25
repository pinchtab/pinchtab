/**
 * PinchTab OpenClaw Plugin
 *
 * Two tools:
 * - `pinchtab`: Full-featured browser control with all actions
 * - `browser`: OpenClaw-compatible simplified interface
 */

import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";
import type { PluginApi, PluginConfig, PluginRuntimeContext, PluginTool, PluginToolContext } from "./types.js";
import { pinchtabToolSchema, pinchtabToolDescription, executePinchtabAction } from "./tools/pinchtab.js";
import { browserToolSchema, browserToolDescription, executeBrowserAction } from "./tools/browser.js";

function getConfig(api: PluginApi): PluginConfig {
  return (api.pluginConfig ?? api.config?.plugins?.entries?.pinchtab?.config ?? {}) as PluginConfig;
}

function toRuntimeContext(ctx: PluginToolContext): PluginRuntimeContext {
  return {
    agentId: ctx.agentId,
    sessionId: ctx.sessionId,
    sessionKey: ctx.sessionKey,
  };
}

const pinchtabPlugin: ReturnType<typeof definePluginEntry> = definePluginEntry({
  id: "pinchtab",
  name: "PinchTab",
  description: "Browser control for AI agents via PinchTab.",
  register(api) {
    const cfg = getConfig(api);

    api.registerTool((ctx) => {
      const runtimeContext = toRuntimeContext(ctx);
      const pinchtabTool = {
        name: "pinchtab",
        label: "PinchTab",
        description: pinchtabToolDescription,
        parameters: pinchtabToolSchema,
        async execute(_id: string, params: any) {
          return executePinchtabAction(getConfig(api), params, runtimeContext);
        },
      } satisfies PluginTool;
      return pinchtabTool;
    }, { optional: true });

    if (cfg.registerBrowserTool !== false) {
      api.registerTool((ctx) => {
        const runtimeContext = toRuntimeContext(ctx);
        const browserTool = {
          name: "browser",
          label: "Browser",
          description: browserToolDescription,
          parameters: browserToolSchema,
          async execute(_id: string, params: any) {
            return executeBrowserAction(getConfig(api), params, runtimeContext);
          },
        } satisfies PluginTool;
        return browserTool;
      }, { optional: true });
    }
  },
});

export default pinchtabPlugin;
