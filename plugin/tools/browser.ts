import type { PluginConfig } from "../types.js";
import { pinchtabFetch, textResult, imageResult, resourceResult } from "../client.js";
import { checkNavigationPolicy, enforcePolicyOrReturn } from "../policy.js";
import { ensureServerRunning, getEnhancedHealth, resolveProfile, getLastTabId, setLastTabId } from "../session.js";

export const browserToolSchema = {
  type: "object",
  properties: {
    action: {
      type: "string",
      enum: ["navigate", "snapshot", "screenshot", "click", "type", "fill", "press", "hover", "scroll", "select", "tabs", "pdf", "status"],
      description: "Browser action",
    },
    url: { type: "string", description: "URL to navigate to" },
    profile: { type: "string", description: "Profile: 'openclaw' (isolated) or 'user' (attached session)" },
    ref: { type: "string", description: "Element ref from snapshot (e.g. e5)" },
    text: { type: "string", description: "Text to type or fill" },
    key: { type: "string", description: "Key to press (Enter, Tab, Escape)" },
    value: { type: "string", description: "Value for select" },
    selector: { type: "string", description: "CSS selector for snapshot scope" },
    format: { type: "string", enum: ["compact", "json", "text"], description: "Output format" },
    maxTokens: { type: "number", description: "Truncate output to ~N tokens" },
    quality: { type: "number", description: "Screenshot JPEG quality 1-100" },
    tabId: { type: "string", description: "Target tab ID" },
    tabAction: { type: "string", enum: ["list", "new", "close"], description: "Tab operation (for action=tabs)" },
    newTab: { type: "boolean", description: "Open URL in new tab" },
    landscape: { type: "boolean", description: "PDF landscape orientation" },
    scale: { type: "number", description: "PDF print scale" },
  },
  required: ["action"],
};

export const browserToolDescription = `OpenClaw-compatible browser control (backed by Pinchtab).

Actions:
- navigate: go to URL (url, profile?, newTab?)
- snapshot: accessibility tree for interactions (selector?, format?, maxTokens?)
- screenshot: capture page image (quality?, format?)
- click/type/fill/press/hover/scroll/select: element actions (ref, text?, key?, value?)
- tabs: list/new/close tabs (tabAction?, url?, tabId?)
- pdf: export page as PDF (landscape?, scale?)
- status: check browser/server health

Profiles: "openclaw" (default isolated), "user" (attach to existing session)`;

export async function executeBrowserAction(cfg: PluginConfig, params: any): Promise<any> {
  const { action, profile } = params;

  // Resolve profile to instance
  const profileConfig = resolveProfile(cfg, profile);

  // Auto-start server if needed
  if (action !== "status") {
    const serverCheck = await ensureServerRunning(cfg);
    if (!serverCheck.ok) {
      return textResult({ error: serverCheck.error });
    }
  }

  // Session tab persistence
  if (cfg.persistSessionTabs !== false && !params.tabId && getLastTabId()) {
    params.tabId = getLastTabId();
  }

  // --- navigate ---
  if (action === "navigate") {
    const navPolicy = enforcePolicyOrReturn(checkNavigationPolicy(cfg, params.url));
    if (navPolicy) return navPolicy;

    const body: any = { url: params.url };
    if (params.tabId) body.tabId = params.tabId;
    if (params.newTab) body.newTab = true;
    if (profileConfig.instanceId) body.instanceId = profileConfig.instanceId;
    const result = await pinchtabFetch(cfg, "/navigate", { body });
    if (result?.tabId) setLastTabId(result.tabId);
    return textResult(result);
  }

  // --- snapshot ---
  if (action === "snapshot") {
    const query = new URLSearchParams();
    if (params.tabId) query.set("tabId", params.tabId);
    query.set("filter", "interactive");
    query.set("format", params.format || cfg.defaultSnapshotFormat || "compact");
    if (params.selector) query.set("selector", params.selector);
    if (params.maxTokens) query.set("maxTokens", String(params.maxTokens));
    return textResult(await pinchtabFetch(cfg, `/snapshot?${query.toString()}`));
  }

  // --- screenshot ---
  if (action === "screenshot") {
    const query = new URLSearchParams();
    if (params.tabId) query.set("tabId", params.tabId);
    const fmt = params.format === "png" ? "png" : (cfg.screenshotFormat || "jpeg");
    if (fmt === "png") query.set("format", "png");
    query.set("quality", String(params.quality || cfg.screenshotQuality || 80));
    try {
      const res = await pinchtabFetch(cfg, `/screenshot?${query.toString()}`, { rawResponse: true });
      if (res instanceof Response) {
        if (!res.ok) return textResult({ error: `Screenshot failed: ${res.status}` });
        const buf = await res.arrayBuffer();
        const b64 = Buffer.from(buf).toString("base64");
        return imageResult(b64, fmt === "png" ? "image/png" : "image/jpeg");
      }
      return textResult(res);
    } catch (err: any) {
      return textResult({ error: `Screenshot failed: ${err?.message}` });
    }
  }

  // --- element actions ---
  if (["click", "type", "fill", "press", "hover", "scroll", "select"].includes(action)) {
    const body: any = { kind: action };
    if (params.ref) body.ref = params.ref;
    if (params.text) body.text = params.text;
    if (params.key) body.key = params.key;
    if (params.value) body.value = params.value;
    if (params.tabId) body.tabId = params.tabId;
    return textResult(await pinchtabFetch(cfg, "/action", { body }));
  }

  // --- tabs ---
  if (action === "tabs") {
    const tabAction = params.tabAction || "list";
    if (tabAction === "list") {
      return textResult(await pinchtabFetch(cfg, "/tabs"));
    }
    const body: any = { action: tabAction };
    if (params.url) body.url = params.url;
    if (params.tabId) body.tabId = params.tabId;
    const result = await pinchtabFetch(cfg, "/tab", { body });
    if (tabAction === "new" && result?.tabId) setLastTabId(result.tabId);
    return textResult(result);
  }

  // --- pdf ---
  if (action === "pdf") {
    const query = new URLSearchParams();
    query.set("raw", "true");
    if (params.tabId) query.set("tabId", params.tabId);
    if (params.landscape) query.set("landscape", "true");
    if (params.scale) query.set("scale", String(params.scale));
    try {
      const res = await pinchtabFetch(cfg, `/pdf?${query.toString()}`, { rawResponse: true });
      if (res instanceof Response) {
        if (!res.ok) return textResult({ error: `PDF failed: ${res.status}` });
        const buf = await res.arrayBuffer();
        const b64 = Buffer.from(buf).toString("base64");
        return resourceResult("pdf://export", "application/pdf", b64);
      }
      return textResult(res);
    } catch (err: any) {
      return textResult({ error: `PDF failed: ${err?.message}` });
    }
  }

  // --- status ---
  if (action === "status") {
    return textResult(await getEnhancedHealth(cfg));
  }

  return textResult({ error: `Unknown browser action: ${action}` });
}
