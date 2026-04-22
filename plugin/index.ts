/**
 * Pinchtab OpenClaw Plugin
 *
 * Two tools:
 * - `pinchtab`: Full-featured browser control with all actions
 * - `browser`: OpenClaw-compatible simplified interface
 */

import type { PluginApi, PluginConfig } from "./types.js";
import { pinchtabToolSchema, pinchtabToolDescription, executePinchtabAction } from "./tools/pinchtab.js";
import { browserToolSchema, browserToolDescription, executeBrowserAction } from "./tools/browser.js";

function getConfig(api: PluginApi): PluginConfig {
  return api.config?.plugins?.entries?.pinchtab?.config ?? {};
}

export default function register(api: PluginApi) {
  const cfg = getConfig(api);

  // Register the full-featured pinchtab tool
  api.registerTool(
    {
      name: "pinchtab",
      description: pinchtabToolDescription,
      parameters: pinchtabToolSchema,
      async execute(_id: string, params: any) {
        return executePinchtabAction(getConfig(api), params);
      },
    },
    { optional: true },
  );

  // Register OpenClaw-compatible browser tool
  if (cfg.registerBrowserTool !== false) {
    api.registerTool(
      {
        name: "browser",
        description: browserToolDescription,
        parameters: browserToolSchema,
        async execute(_id: string, params: any) {
          return executeBrowserAction(getConfig(api), params);
        },
      },
      { optional: true },
    );
  }
}
