import type { PluginConfig, PluginRuntimeContext } from "../types.js";
import { pinchtabFetch, textResult, imageResult, resourceResult, normalizeActionParams, looksLikeStaleRef } from "../client.js";
import { checkNavigationPolicy, checkEvaluatePolicy, checkDownloadPolicy, checkUploadPolicy, checkNetworkInterceptPolicy, enforcePolicyOrReturn } from "../policy.js";
import { ensureReady, getEnhancedHealth, getLastTabId, rememberRuntimeContext, resolveEffectiveConfig, setLastTabId } from "../session.js";

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
    networkAction: { type: "string", enum: ["list", "get", "export", "clear", "route", "unroute"], description: "Network sub-action" },
    requestId: { type: "string", description: "Network request ID for get action" },
    method: { type: "string", description: "Filter network by HTTP method (also: limits a route rule to one method; fulfill rules without method skip OPTIONS preflights to avoid breaking CORS)" },
    status: { type: "string", description: "Filter network by status (e.g. 4xx, 5xx, 200)" },
    resourceType: { type: "string", description: "Filter network by resource type, or limit a route to one type (xhr, fetch, document, script, image, etc)" },
    limit: { type: "number", description: "Limit network results" },
    exportFormat: { type: "string", enum: ["har", "json"], description: "Network export format" },
    pattern: { type: "string", description: "URL pattern for network route/unroute (substring or glob)" },
    routeAction: { type: "string", enum: ["continue", "abort", "fulfill"], description: "Network route behavior. Default 'continue' (pass-through)." },
    responseBody: { type: "string", description: "Response body for routeAction=fulfill (sent verbatim; not auto-encoded)" },
    contentType: { type: "string", description: "Response Content-Type for fulfill (default application/json)" },
    responseStatus: { type: "number", description: "Response status code for fulfill (default 200)" },
  },
  required: ["action"],
};

export const pinchtabToolDescription = `Browser control via PinchTab. Actions:
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
- network: capture/inspect/intercept network requests (networkAction: list|get|export|clear|route|unroute). For route: pattern, routeAction (continue|abort|fulfill), responseBody?, contentType?, responseStatus?, resourceType?. For unroute: pattern? (omit to clear all). Fulfill is BLOCKED on hosts in security.allowedDomains (sensitive surfaces) and allowed elsewhere; rules on allowlisted hosts fall through to real fetch.
- health: check connectivity

Token strategy: use "text" for reading (~800 tokens), "snapshot" with filter=interactive&format=compact for interactions (~3,600), diff=true on subsequent snapshots.`;

type ActionHandler = (cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext) => Promise<any>;

const elementActions = [
  "click", "type", "press", "fill", "hover", "mouse-move", "mouse-down",
  "mouse-up", "mouse-wheel", "scroll", "select", "focus",
];

async function handleNavigate(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const navPolicy = enforcePolicyOrReturn(checkNavigationPolicy(cfg, normalized.url));
  if (navPolicy) return navPolicy;

  let result;
  if (normalized.newTab) {
    const body: any = { action: "new", url: normalized.url };
    if (normalized.blockImages) body.blockImages = true;
    if (normalized.timeout) body.timeout = normalized.timeout;
    result = await pinchtabFetch(cfg, "/tab", { body }, context);
  } else if (normalized.tabId) {
    const body: any = { url: normalized.url };
    if (normalized.blockImages) body.blockImages = true;
    if (normalized.timeout) body.timeout = normalized.timeout;
    result = await pinchtabFetch(cfg, `/tabs/${encodeURIComponent(normalized.tabId)}/navigate`, { body }, context);
  } else {
    const body: any = { url: normalized.url };
    if (normalized.blockImages) body.blockImages = true;
    if (normalized.timeout) body.timeout = normalized.timeout;
    result = await pinchtabFetch(cfg, "/navigate", { body }, context);
  }
  if (result?.tabId) setLastTabId(result.tabId, context);
  return textResult(result);
}

async function handleSnapshot(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
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
  return textResult(await pinchtabFetch(cfg, `/snapshot?${query.toString()}`, {}, context));
}

async function handleElementAction(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const body: any = { kind: normalized.action };
  for (const k of ["ref", "text", "key", "selector", "value", "scrollY", "x", "y", "button", "deltaX", "deltaY", "tabId", "waitNav"]) {
    if (normalized[k] !== undefined) body[k] = normalized[k];
  }

  let result = await pinchtabFetch(cfg, "/action", { body }, context);

  // Stale ref recovery
  if (body.ref && looksLikeStaleRef(result)) {
    const q = new URLSearchParams();
    q.set("filter", "interactive");
    q.set("format", "compact");
    if (body.tabId) q.set("tabId", body.tabId);
    await pinchtabFetch(cfg, `/snapshot?${q.toString()}`, {}, context);
    const retried = await pinchtabFetch(cfg, "/action", { body }, context);
    result = retried?.error
      ? { ...retried, warning: "Action retried once after snapshot refresh but still failed." }
      : { ...retried, warning: "Action succeeded after automatic snapshot refresh (stale ref recovery)." };
  }

  // Fill fallback to type
  if (normalized.action === "fill" && result?.error && body.ref && (typeof body.text === "string" || typeof body.value === "string")) {
    const typeBody: any = { ...body, kind: "type" };
    if (typeof typeBody.text !== "string" && typeof typeBody.value === "string") {
      typeBody.text = typeBody.value;
    }
    delete typeBody.value;
    const typed = await pinchtabFetch(cfg, "/action", { body: typeBody }, context);
    if (!typed?.error) {
      result = { ...typed, warning: "Fill failed; type fallback succeeded." };
    }
  }

  return textResult(result);
}

async function handleText(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const query = new URLSearchParams();
  if (normalized.tabId) query.set("tabId", normalized.tabId);
  if (normalized.mode) query.set("mode", normalized.mode);
  return textResult(await pinchtabFetch(cfg, `/text?${query.toString()}`, {}, context));
}

// Wait CONDITION fields decide whether a wait should run; the full body also
// carries tab/timeout/state config. Keeping the body list a superset of the
// condition list ensures a new wait condition propagates to both.
const WAIT_CONDITION_FIELDS = ["selector", "text", "url", "load", "fn", "ms"] as const;
const WAIT_BODY_FIELDS = [...WAIT_CONDITION_FIELDS, "tabId", "timeout", "state"] as const;

function buildWaitBody(normalized: any): any {
  const body: any = {};
  for (const k of WAIT_BODY_FIELDS) {
    if (normalized[k] !== undefined) body[k] = normalized[k];
  }
  return body;
}

function hasWaitCondition(normalized: any): boolean {
  return WAIT_CONDITION_FIELDS.some((k) => normalized[k] !== undefined);
}

async function handleWait(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  return textResult(await pinchtabFetch(cfg, "/wait", { body: buildWaitBody(normalized) }, context));
}

async function handleHandoff(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const handoffMeta = {
    status: "human_handoff_required",
    reason: normalized.humanReason || "Manual intervention required (captcha/login/2FA/credential entry).",
    instructions: normalized.humanPrompt || "Please complete the step in the headed browser, then resume automation.",
  };

  if (!hasWaitCondition(normalized)) {
    return textResult({ ...handoffMeta, resumed: false, next: "Call with a wait condition or use action='wait' to resume." });
  }

  const waitResult = await pinchtabFetch(cfg, "/wait", { body: buildWaitBody(normalized) }, context);
  return textResult({ ...handoffMeta, resumed: !waitResult?.error, waitResult });
}

async function handleTabs(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const tabAction = normalized.tabAction || "list";
  if (tabAction === "list") {
    const listed = await pinchtabFetch(cfg, "/tabs", {}, context);
    const tabs = Array.isArray(listed?.tabs) ? listed.tabs : Array.isArray(listed) ? listed : [];
    if (tabs.length > 0) return textResult(listed);

    const instances = await pinchtabFetch(cfg, "/instances", {}, context);
    const list = Array.isArray(instances?.value) ? instances.value : Array.isArray(instances) ? instances : [];
    const running = list.find((i: any) => i?.status === "running" && i?.id);
    if (!running) {
      return textResult({ ...listed, warning: "No tabs returned and no running instance found." });
    }
    const instanceTabs = await pinchtabFetch(cfg, `/instances/${encodeURIComponent(running.id)}/tabs`, {}, context);
    return textResult({ source: "instance-fallback", instanceId: running.id, tabs: instanceTabs?.tabs ?? instanceTabs });
  }
  if (tabAction === "close") {
    const body: any = {};
    if (normalized.tabId) body.tabId = normalized.tabId;
    const result = await pinchtabFetch(cfg, "/close", { body }, context);
    if (normalized.tabId && normalized.tabId === getLastTabId(context)) setLastTabId(undefined, context);
    return textResult(result);
  }
  const body: any = { action: tabAction };
  if (normalized.url) body.url = normalized.url;
  if (normalized.tabId) body.tabId = normalized.tabId;
  const result = await pinchtabFetch(cfg, "/tab", { body }, context);
  if (tabAction === "new" && result?.tabId) setLastTabId(result.tabId, context);
  return textResult(result);
}

// fetchBinaryResult performs a raw-response GET and adapts it to a tool result:
// non-2xx → an {error} textResult, a thrown error → an {error} textResult, a
// non-Response (e.g. already-JSON) body → textResult, and a 2xx binary body →
// toResult(base64, response). The response is passed so callers can derive a
// MIME type from its headers.
async function fetchBinaryResult(
  cfg: PluginConfig,
  path: string,
  label: string,
  toResult: (b64: string, res: Response) => any,
  context?: PluginRuntimeContext,
): Promise<any> {
  try {
    const res = await pinchtabFetch(cfg, path, { rawResponse: true }, context);
    if (res instanceof Response) {
      if (!res.ok) return textResult({ error: `${label} failed: ${res.status} ${await res.text()}` });
      const buf = await res.arrayBuffer();
      const b64 = Buffer.from(buf).toString("base64");
      return toResult(b64, res);
    }
    return textResult(res);
  } catch (err: any) {
    return textResult({ error: `${label} failed: ${err?.message}` });
  }
}

async function handleScreenshot(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const query = new URLSearchParams();
  if (normalized.tabId) query.set("tabId", normalized.tabId);
  const format = normalized.screenshotFormat ?? cfg.screenshotFormat ?? "jpeg";
  const quality = normalized.quality ?? cfg.screenshotQuality ?? 80;
  if (format === "png") query.set("format", "png");
  query.set("quality", String(quality));
  return fetchBinaryResult(
    cfg,
    `/screenshot?${query.toString()}`,
    "Screenshot",
    (b64) => imageResult(b64, format === "png" ? "image/png" : "image/jpeg"),
    context,
  );
}

async function handleEvaluate(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const policyResult = enforcePolicyOrReturn(checkEvaluatePolicy(cfg));
  if (policyResult) return policyResult;

  const body: any = { expression: normalized.expression };
  if (normalized.tabId) body.tabId = normalized.tabId;
  return textResult(await pinchtabFetch(cfg, "/evaluate", { body }, context));
}

async function handlePdf(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const query = new URLSearchParams();
  query.set("raw", "true");
  if (normalized.tabId) query.set("tabId", normalized.tabId);
  if (normalized.landscape) query.set("landscape", "true");
  if (normalized.scale) query.set("scale", String(normalized.scale));
  return fetchBinaryResult(
    cfg,
    `/pdf?${query.toString()}`,
    "PDF",
    (b64) => resourceResult("pdf://export", "application/pdf", b64),
    context,
  );
}

async function handleNetwork(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const networkAction = normalized.networkAction || "list";

  if (networkAction === "clear") {
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    return textResult(await pinchtabFetch(cfg, `/network/clear?${query.toString()}`, { method: "POST" }, context));
  }

  if (networkAction === "get" && normalized.requestId) {
    return textResult(await pinchtabFetch(cfg, `/network/${encodeURIComponent(normalized.requestId)}`, {}, context));
  }

  if (networkAction === "export") {
    const query = new URLSearchParams();
    if (normalized.tabId) query.set("tabId", normalized.tabId);
    if (normalized.exportFormat === "json") query.set("format", "json");
    const result = await pinchtabFetch(cfg, `/network/export?${query.toString()}`, {}, context);
    if (normalized.exportFormat === "har" || !normalized.exportFormat) {
      return resourceResult("har://export", "application/json", Buffer.from(JSON.stringify(result)).toString("base64"));
    }
    return textResult(result);
  }

  if (networkAction === "route") {
    const policyResult = enforcePolicyOrReturn(checkNetworkInterceptPolicy(cfg));
    if (policyResult) return policyResult;
    if (!normalized.tabId) return textResult({ error: "route requires tabId" });
    if (!normalized.pattern) return textResult({ error: "route requires pattern" });
    const payload: Record<string, any> = {
      pattern: normalized.pattern,
      action: normalized.routeAction || "continue",
    };
    if (normalized.responseBody !== undefined) payload.body = normalized.responseBody;
    if (normalized.contentType) payload.contentType = normalized.contentType;
    if (typeof normalized.responseStatus === "number") payload.status = normalized.responseStatus;
    if (normalized.resourceType) payload.resourceType = normalized.resourceType;
    if (normalized.method) payload.method = normalized.method;
    return textResult(await pinchtabFetch(cfg, `/tabs/${encodeURIComponent(normalized.tabId)}/network/route`, {
      method: "POST",
      body: payload,
    }, context));
  }

  if (networkAction === "unroute") {
    const policyResult = enforcePolicyOrReturn(checkNetworkInterceptPolicy(cfg));
    if (policyResult) return policyResult;
    if (!normalized.tabId) return textResult({ error: "unroute requires tabId" });
    const query = new URLSearchParams();
    if (normalized.pattern) query.set("pattern", normalized.pattern);
    const qs = query.toString();
    const path = `/tabs/${encodeURIComponent(normalized.tabId)}/network/route${qs ? `?${qs}` : ""}`;
    return textResult(await pinchtabFetch(cfg, path, { method: "DELETE" }, context));
  }

  // list
  const query = new URLSearchParams();
  if (normalized.tabId) query.set("tabId", normalized.tabId);
  if (normalized.method) query.set("method", normalized.method);
  if (normalized.status) query.set("status", normalized.status);
  if (normalized.resourceType) query.set("type", normalized.resourceType);
  if (normalized.limit) query.set("limit", String(normalized.limit));
  return textResult(await pinchtabFetch(cfg, `/network?${query.toString()}`, {}, context));
}

async function handleDownload(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const policyResult = enforcePolicyOrReturn(checkDownloadPolicy(cfg));
  if (policyResult) return policyResult;

  const query = new URLSearchParams();
  if (normalized.downloadUrl) query.set("url", normalized.downloadUrl);
  if (normalized.tabId) query.set("tabId", normalized.tabId);
  if (normalized.savePath) {
    query.set("output", "file");
    query.set("path", normalized.savePath);
    return textResult(await pinchtabFetch(cfg, `/download?${query.toString()}`, {}, context));
  }
  query.set("raw", "true");
  return fetchBinaryResult(
    cfg,
    `/download?${query.toString()}`,
    "download",
    (b64, res) => resourceResult("download://file", res.headers.get("content-type") || "application/octet-stream", b64),
    context,
  );
}

async function handleUpload(cfg: PluginConfig, normalized: any, context?: PluginRuntimeContext): Promise<any> {
  const policyResult = enforcePolicyOrReturn(checkUploadPolicy(cfg));
  if (policyResult) return policyResult;

  const body: any = {};
  if (normalized.tabId) body.tabId = normalized.tabId;
  if (normalized.selector) body.selector = normalized.selector;
  if (normalized.files) body.files = normalized.files;
  if (normalized.paths) body.paths = normalized.paths;
  return textResult(await pinchtabFetch(cfg, "/upload", { body }, context));
}

async function handleHealth(cfg: PluginConfig, _normalized: any, context?: PluginRuntimeContext): Promise<any> {
  return textResult(await getEnhancedHealth(cfg, context));
}

const actionHandlers: Record<string, ActionHandler> = {
  navigate: handleNavigate,
  snapshot: handleSnapshot,
  ...Object.fromEntries(elementActions.map((a) => [a, handleElementAction])),
  text: handleText,
  wait: handleWait,
  handoff: handleHandoff,
  tabs: handleTabs,
  screenshot: handleScreenshot,
  evaluate: handleEvaluate,
  pdf: handlePdf,
  network: handleNetwork,
  download: handleDownload,
  upload: handleUpload,
  health: handleHealth,
};

export async function executePinchtabAction(rawCfg: PluginConfig, params: any, context?: PluginRuntimeContext): Promise<any> {
  rememberRuntimeContext(context);
  const cfg = await resolveEffectiveConfig(rawCfg);
  const normalized = normalizeActionParams(params);
  const { action } = normalized;

  // Auto-start / readiness — cached so routine actions skip the per-call probe.
  if (action !== "health") {
    const ready = await ensureReady(cfg, context);
    if (!ready.ok) {
      return textResult({ error: ready.error });
    }
  }

  // Session tab persistence
  if (cfg.persistSessionTabs !== false && !normalized.tabId && getLastTabId(context)) {
    normalized.tabId = getLastTabId(context);
  }

  const handler = actionHandlers[action];
  if (!handler) {
    return textResult({ error: `Unknown action: ${action}` });
  }
  return handler(cfg, normalized, context);
}
