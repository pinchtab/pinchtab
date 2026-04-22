import type { PluginConfig } from "../types.js";
import { pinchtabFetch, textResult, imageResult, resourceResult, normalizeActionParams, looksLikeStaleRef } from "../client.js";
import { checkNavigationPolicy, checkEvaluatePolicy, checkDownloadPolicy, checkUploadPolicy, enforcePolicyOrReturn } from "../policy.js";
import { ensureServerRunning, getEnhancedHealth, getLastTabId, setLastTabId } from "../session.js";

export const pinchtabToolSchema = {
  type: "object",
  properties: {
    action: {
      type: "string",
      enum: [
        "navigate", "snapshot", "click", "type", "press", "fill", "hover",
        "mouse-move", "mouse-down", "mouse-up", "mouse-wheel", "scroll",
        "select", "focus", "text", "wait", "handoff", "tabs", "screenshot",
        "evaluate", "pdf", "download", "upload", "network", "health",
      ],
      description: "Action to perform",
    },
    url: { type: "string", description: "URL for navigate or new tab" },
    ref: { type: "string", description: "Element ref from snapshot (e.g. e5)" },
    text: { type: "string", description: "Text to type or fill" },
    key: { type: "string", description: "Key to press (e.g. Enter, Tab, Escape)" },
    expression: { type: "string", description: "JavaScript expression for evaluate" },
    selector: { type: "string", description: "CSS selector for snapshot scope or action target" },
    filter: { type: "string", enum: ["interactive", "all"], description: "Snapshot filter" },
    format: { type: "string", enum: ["json", "compact", "text", "yaml"], description: "Snapshot format" },
    maxTokens: { type: "number", description: "Truncate snapshot to ~N tokens" },
    depth: { type: "number", description: "Max snapshot tree depth" },
    diff: { type: "boolean", description: "Snapshot diff: only changes since last" },
    value: { type: "string", description: "Value for select dropdown" },
    scrollY: { type: "number", description: "Pixels to scroll vertically" },
    x: { type: "number", description: "Mouse X coordinate" },
    y: { type: "number", description: "Mouse Y coordinate" },
    button: { type: "string", enum: ["left", "right", "middle"], description: "Mouse button" },
    deltaX: { type: "number", description: "Mouse wheel horizontal delta" },
    deltaY: { type: "number", description: "Mouse wheel vertical delta" },
    waitNav: { type: "boolean", description: "Wait for navigation after action" },
    tabId: { type: "string", description: "Target tab ID" },
    tabAction: { type: "string", enum: ["list", "new", "close"], description: "Tab sub-action" },
    newTab: { type: "boolean", description: "Open URL in new tab" },
    blockImages: { type: "boolean", description: "Block image loading" },
    timeout: { type: "number", description: "Navigation timeout in seconds" },
    quality: { type: "number", description: "JPEG quality 1-100" },
    screenshotFormat: { type: "string", enum: ["jpeg", "png"], description: "Screenshot format" },
    mode: { type: "string", enum: ["readability", "raw"], description: "Text extraction mode" },
    ms: { type: "number", description: "Wait milliseconds" },
    state: { type: "string", enum: ["visible", "hidden", "attached", "detached"], description: "Wait state" },
    load: { type: "string", enum: ["load", "domcontentloaded", "networkidle"], description: "Document load state" },
    fn: { type: "string", description: "JavaScript predicate for wait" },
    humanReason: { type: "string", description: "Reason for manual handoff" },
    humanPrompt: { type: "string", description: "Instruction for human handoff" },
    landscape: { type: "boolean", description: "PDF landscape orientation" },
    scale: { type: "number", description: "PDF print scale" },
    downloadUrl: { type: "string", description: "URL to download file from" },
    savePath: { type: "string", description: "Path to save downloaded file" },
    files: { type: "array", items: { type: "string" }, description: "Base64-encoded files for upload" },
    paths: { type: "array", items: { type: "string" }, description: "File paths for upload" },
    networkAction: { type: "string", enum: ["list", "get", "export", "clear"], description: "Network sub-action" },
    requestId: { type: "string", description: "Network request ID for get action" },
    method: { type: "string", description: "Filter network by HTTP method" },
    status: { type: "string", description: "Filter network by status (e.g. 4xx, 5xx, 200)" },
    resourceType: { type: "string", description: "Filter network by resource type (xhr, fetch, document)" },
    limit: { type: "number", description: "Limit network results" },
    exportFormat: { type: "string", enum: ["har", "json"], description: "Network export format" },
  },
  required: ["action"],
};

export const pinchtabToolDescription = `Browser control via Pinchtab. Actions:
- navigate: go to URL (url, tabId?, newTab?, blockImages?, timeout?)
- snapshot: accessibility tree (filter?, format?, selector?, maxTokens?, depth?, diff?, tabId?)
- click/type/press/fill/hover/scroll/select/focus: act on element (ref, text?, key?, value?, scrollY?, waitNav?, tabId?)
- mouse-move/mouse-down/mouse-up/mouse-wheel: low-level mouse controls (ref|selector|x+y, button?, deltaX?, deltaY?, tabId?)
- text: extract readable text (mode?, tabId?)
- wait: pause until condition (selector|text|url|load|fn|ms, tabId?, timeout?, state?)
- handoff: request human intervention mid-flow (captcha/login/2FA/credentials)
- tabs: list/new/close tabs (tabAction?, url?, tabId?)
- screenshot: JPEG screenshot (quality?, tabId?)
- evaluate: run JS (expression, tabId?)
- pdf: export page as PDF (landscape?, scale?, tabId?)
- download: download file from URL (downloadUrl, savePath?, tabId?)
- upload: upload files to file input (selector, files[]|paths[], tabId?)
- network: capture/inspect network requests (networkAction: list|get|export|clear, requestId?, method?, status?, limit?)
- health: check connectivity

Token strategy: use "text" for reading (~800 tokens), "snapshot" with filter=interactive&format=compact for interactions (~3,600), diff=true on subsequent snapshots.`;

export async function executePinchtabAction(cfg: PluginConfig, params: any): Promise<any> {
  const normalized = normalizeActionParams(params);
  const { action } = normalized;

  // Auto-start server if needed
  if (action !== "health") {
    const serverCheck = await ensureServerRunning(cfg);
    if (!serverCheck.ok) {
      return textResult({ error: serverCheck.error });
    }
  }

  // Session tab persistence
  if (cfg.persistSessionTabs !== false && !normalized.tabId && getLastTabId()) {
    normalized.tabId = getLastTabId();
  }

  // Policy: block evaluate if not allowed
  if (action === "evaluate") {
    const policyResult = enforcePolicyOrReturn(checkEvaluatePolicy(cfg));
    if (policyResult) return policyResult;
  }

  // --- navigate ---
  if (action === "navigate") {
    const navPolicy = enforcePolicyOrReturn(checkNavigationPolicy(cfg, normalized.url));
    if (navPolicy) return navPolicy;

    const body: any = { url: normalized.url };
    if (normalized.tabId) body.tabId = normalized.tabId;
    if (normalized.newTab) body.newTab = true;
    if (normalized.blockImages) body.blockImages = true;
    if (normalized.timeout) body.timeout = normalized.timeout;
    const result = await pinchtabFetch(cfg, "/navigate", { body });
    if (result?.tabId) setLastTabId(result.tabId);
    return textResult(result);
  }

  // --- snapshot ---
  if (action === "snapshot") {
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    const filter = normalized.filter ?? cfg.defaultSnapshotFilter ?? "interactive";
    const format = normalized.format ?? cfg.defaultSnapshotFormat ?? "compact";
    query.set("filter", filter);
    query.set("format", format);
    if (normalized.selector) query.set("selector", normalized.selector);
    if (normalized.maxTokens) query.set("maxTokens", String(normalized.maxTokens));
    if (normalized.depth) query.set("depth", String(normalized.depth));
    if (normalized.diff) query.set("diff", "true");
    return textResult(await pinchtabFetch(cfg, `/snapshot?${query.toString()}`));
  }

  // --- element actions ---
  const elementActions = [
    "click", "type", "press", "fill", "hover", "mouse-move", "mouse-down",
    "mouse-up", "mouse-wheel", "scroll", "select", "focus",
  ];
  if (elementActions.includes(action)) {
    const body: any = { kind: action };
    for (const k of ["ref", "text", "key", "selector", "value", "scrollY", "x", "y", "button", "deltaX", "deltaY", "tabId", "waitNav"]) {
      if (normalized[k] !== undefined) body[k] = normalized[k];
    }

    let result = await pinchtabFetch(cfg, "/action", { body });

    // Stale ref recovery
    if (body.ref && looksLikeStaleRef(result)) {
      const q = new URLSearchParams();
      q.set("filter", "interactive");
      q.set("format", "compact");
      if (body.tabId) q.set("tabId", body.tabId);
      await pinchtabFetch(cfg, `/snapshot?${q.toString()}`);
      const retried = await pinchtabFetch(cfg, "/action", { body });
      result = retried?.error
        ? { ...retried, warning: "Action retried once after snapshot refresh but still failed." }
        : { ...retried, warning: "Action succeeded after automatic snapshot refresh (stale ref recovery)." };
    }

    // Fill fallback to type
    if (action === "fill" && result?.error && body.ref && (typeof body.text === "string" || typeof body.value === "string")) {
      const typeBody: any = { ...body, kind: "type" };
      if (typeof typeBody.text !== "string" && typeof typeBody.value === "string") {
        typeBody.text = typeBody.value;
      }
      delete typeBody.value;
      const typed = await pinchtabFetch(cfg, "/action", { body: typeBody });
      if (!typed?.error) {
        result = { ...typed, warning: "Fill failed; type fallback succeeded." };
      }
    }

    return textResult(result);
  }

  // --- text ---
  if (action === "text") {
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    if (normalized.mode) query.set("mode", normalized.mode);
    return textResult(await pinchtabFetch(cfg, `/text?${query.toString()}`));
  }

  // --- wait ---
  if (action === "wait") {
    const body: any = {};
    for (const k of ["selector", "text", "url", "load", "fn", "ms", "tabId", "timeout", "state"]) {
      if (normalized[k] !== undefined) body[k] = normalized[k];
    }
    return textResult(await pinchtabFetch(cfg, "/wait", { body }));
  }

  // --- handoff ---
  if (action === "handoff") {
    const hasWaitCondition = ["selector", "text", "url", "load", "fn", "ms"].some((k) => normalized[k] !== undefined);
    const handoffMeta = {
      status: "human_handoff_required",
      reason: normalized.humanReason || "Manual intervention required (captcha/login/2FA/credential entry).",
      instructions: normalized.humanPrompt || "Please complete the step in the headed browser, then resume automation.",
    };

    if (!hasWaitCondition) {
      return textResult({ ...handoffMeta, resumed: false, next: "Call with a wait condition or use action='wait' to resume." });
    }

    const waitBody: any = {};
    for (const k of ["selector", "text", "url", "load", "fn", "ms", "tabId", "timeout", "state"]) {
      if (normalized[k] !== undefined) waitBody[k] = normalized[k];
    }
    const waitResult = await pinchtabFetch(cfg, "/wait", { body: waitBody });
    return textResult({ ...handoffMeta, resumed: !waitResult?.error, waitResult });
  }

  // --- tabs ---
  if (action === "tabs") {
    const tabAction = normalized.tabAction || "list";
    if (tabAction === "list") {
      const listed = await pinchtabFetch(cfg, "/tabs");
      const tabs = Array.isArray(listed?.tabs) ? listed.tabs : Array.isArray(listed) ? listed : [];
      if (tabs.length > 0) return textResult(listed);

      const instances = await pinchtabFetch(cfg, "/instances");
      const list = Array.isArray(instances?.value) ? instances.value : Array.isArray(instances) ? instances : [];
      const running = list.find((i: any) => i?.status === "running" && i?.id);
      if (!running) {
        return textResult({ ...listed, warning: "No tabs returned and no running instance found." });
      }
      const instanceTabs = await pinchtabFetch(cfg, `/instances/${running.id}/tabs`);
      return textResult({ source: "instance-fallback", instanceId: running.id, tabs: instanceTabs?.tabs ?? instanceTabs });
    }
    const body: any = { action: tabAction };
    if (normalized.url) body.url = normalized.url;
    if (normalized.tabId) body.tabId = normalized.tabId;
    const result = await pinchtabFetch(cfg, "/tab", { body });
    if (tabAction === "new" && result?.tabId) setLastTabId(result.tabId);
    return textResult(result);
  }

  // --- screenshot ---
  if (action === "screenshot") {
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    const format = normalized.screenshotFormat ?? cfg.screenshotFormat ?? "jpeg";
    const quality = normalized.quality ?? cfg.screenshotQuality ?? 80;
    if (format === "png") query.set("format", "png");
    query.set("quality", String(quality));
    try {
      const res = await pinchtabFetch(cfg, `/screenshot?${query.toString()}`, { rawResponse: true });
      if (res instanceof Response) {
        if (!res.ok) return textResult({ error: `Screenshot failed: ${res.status} ${await res.text()}` });
        const buf = await res.arrayBuffer();
        const b64 = Buffer.from(buf).toString("base64");
        return imageResult(b64, format === "png" ? "image/png" : "image/jpeg");
      }
      return textResult(res);
    } catch (err: any) {
      return textResult({ error: `Screenshot failed: ${err?.message}` });
    }
  }

  // --- evaluate ---
  if (action === "evaluate") {
    const body: any = { expression: normalized.expression };
    if (normalized.tabId) body.tabId = normalized.tabId;
    return textResult(await pinchtabFetch(cfg, "/evaluate", { body }));
  }

  // --- pdf ---
  if (action === "pdf") {
    const query = new URLSearchParams();
    query.set("raw", "true");
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    if (normalized.landscape) query.set("landscape", "true");
    if (normalized.scale) query.set("scale", String(normalized.scale));
    try {
      const res = await pinchtabFetch(cfg, `/pdf?${query.toString()}`, { rawResponse: true });
      if (res instanceof Response) {
        if (!res.ok) return textResult({ error: `PDF failed: ${res.status} ${await res.text()}` });
        const buf = await res.arrayBuffer();
        const b64 = Buffer.from(buf).toString("base64");
        return resourceResult("pdf://export", "application/pdf", b64);
      }
      return textResult(res);
    } catch (err: any) {
      return textResult({ error: `PDF failed: ${err?.message}` });
    }
  }

  // --- network ---
  if (action === "network") {
    const networkAction = normalized.networkAction || "list";

    if (networkAction === "clear") {
      const query = new URLSearchParams();
      if (normalized.tabId) query.set("tabId", normalized.tabId);
      return textResult(await pinchtabFetch(cfg, `/network/clear?${query.toString()}`, { method: "POST" }));
    }

    if (networkAction === "get" && normalized.requestId) {
      return textResult(await pinchtabFetch(cfg, `/network/${normalized.requestId}`));
    }

    if (networkAction === "export") {
      const query = new URLSearchParams();
      if (normalized.tabId) query.set("tabId", normalized.tabId);
      if (normalized.exportFormat === "json") query.set("format", "json");
      const result = await pinchtabFetch(cfg, `/network/export?${query.toString()}`);
      if (normalized.exportFormat === "har" || !normalized.exportFormat) {
        return resourceResult("har://export", "application/json", Buffer.from(JSON.stringify(result)).toString("base64"));
      }
      return textResult(result);
    }

    // list
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    if (normalized.method) query.set("method", normalized.method);
    if (normalized.status) query.set("status", normalized.status);
    if (normalized.resourceType) query.set("type", normalized.resourceType);
    if (normalized.limit) query.set("limit", String(normalized.limit));
    return textResult(await pinchtabFetch(cfg, `/network?${query.toString()}`));
  }

  // --- download ---
  if (action === "download") {
    const policyResult = enforcePolicyOrReturn(checkDownloadPolicy(cfg));
    if (policyResult) return policyResult;

    const query = new URLSearchParams();
    if (normalized.downloadUrl) query.set("url", normalized.downloadUrl);
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    if (normalized.savePath) {
      query.set("output", "file");
      query.set("path", normalized.savePath);
    }
    return textResult(await pinchtabFetch(cfg, `/download?${query.toString()}`));
  }

  // --- upload ---
  if (action === "upload") {
    const policyResult = enforcePolicyOrReturn(checkUploadPolicy(cfg));
    if (policyResult) return policyResult;

    const body: any = {};
    if (normalized.tabId) body.tabId = normalized.tabId;
    if (normalized.selector) body.selector = normalized.selector;
    if (normalized.files) body.files = normalized.files;
    if (normalized.paths) body.paths = normalized.paths;
    return textResult(await pinchtabFetch(cfg, "/upload", { body }));
  }

  // --- health ---
  if (action === "health") {
    return textResult(await getEnhancedHealth(cfg));
  }

  return textResult({ error: `Unknown action: ${action}` });
}
