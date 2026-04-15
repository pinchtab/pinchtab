/**
 * Pinchtab OpenClaw Plugin
 *
 * Single-tool design: one `pinchtab` tool with an `action` parameter.
 * Minimal context bloat — one tool definition covers all browser operations.
 */

interface PluginConfig {
  baseUrl?: string;
  token?: string;
  timeout?: number;
}

interface PluginApi {
  config: { plugins?: { entries?: Record<string, { config?: PluginConfig }> } };
  registerTool: (tool: any, opts?: { optional?: boolean }) => void;
}

function isRefToken(value: unknown): value is string {
  return typeof value === "string" && /^e\d+$/i.test(value.trim());
}

function normalizeActionParams(input: any): any {
  const params = { ...input };
  // Some agents send ref-like values (e.g. "e56") in selector.
  // Normalize to ref so action routing stays stable.
  if (!params.ref && isRefToken(params.selector)) {
    params.ref = String(params.selector).trim();
    delete params.selector;
  }
  if (typeof params.ref === "string") {
    params.ref = params.ref.trim();
  }
  return params;
}

function actionErrorText(result: any): string {
  return `${result?.error || ""} ${result?.body || ""}`.toLowerCase();
}

function looksLikeStaleRef(result: any): boolean {
  if (!result?.error) return false;
  const text = actionErrorText(result);
  return (
    text.includes("stale") ||
    text.includes("context canceled") ||
    text.includes("ref") ||
    text.includes("not found") ||
    text.includes("no node") ||
    text.includes("unknown element")
  );
}

function getConfig(api: PluginApi): PluginConfig {
  return api.config?.plugins?.entries?.pinchtab?.config ?? {};
}

async function pinchtabFetch(
  cfg: PluginConfig,
  path: string,
  opts: { method?: string; body?: unknown; rawResponse?: boolean } = {},
): Promise<any> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const url = `${base}${path}`;
  const headers: Record<string, string> = {};
  if (cfg.token) headers["Authorization"] = `Bearer ${cfg.token}`;
  if (opts.body) headers["Content-Type"] = "application/json";

  const controller = new AbortController();
  const timeout = cfg.timeout || 30000;
  const timer = setTimeout(() => controller.abort(), timeout);

  try {
    const res = await fetch(url, {
      method: opts.method || (opts.body ? "POST" : "GET"),
      headers,
      body: opts.body ? JSON.stringify(opts.body) : undefined,
      signal: controller.signal,
    });
    if (opts.rawResponse) return res;
    const text = await res.text();
    if (!res.ok) {
      return { error: `${res.status} ${res.statusText}`, body: text };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { text };
    }
  } catch (err: any) {
    if (err?.name === "AbortError") {
      return { error: `Request timed out after ${timeout}ms: ${path}` };
    }
    return {
      error: `Connection failed: ${err?.message || err}. Is Pinchtab running at ${base}?`,
    };
  } finally {
    clearTimeout(timer);
  }
}

function textResult(data: any): any {
  const text =
    typeof data === "string" ? data : data?.text ?? JSON.stringify(data, null, 2);
  return { content: [{ type: "text", text }] };
}

export default function register(api: PluginApi) {
  api.registerTool(
    {
      name: "pinchtab",
      description: `Browser control via Pinchtab. Actions:
- navigate: go to URL (url, tabId?, newTab?, blockImages?, timeout?)
- snapshot: accessibility tree (filter?, format?, selector?, maxTokens?, depth?, diff?, tabId?)
    - click/type/press/fill/hover/scroll/select/focus: act on element (ref, text?, key?, value?, scrollY?, waitNav?, tabId?)
    - mousemove/mousedown/mouseup/mousewheel: low-level mouse controls (ref|selector|x+y, button?, wheelDeltaX?, wheelDeltaY?, tabId?)
- text: extract readable text (mode?, tabId?)
- wait: pause until condition (selector|text|url|load|fn|ms, tabId?, timeout?, state?)
- handoff: request human intervention mid-flow (captcha/login/2FA/credentials), optionally wait for resume condition
- tabs: list/new/close tabs (tabAction?, url?, tabId?)
- screenshot: JPEG screenshot (quality?, tabId?)
- evaluate: run JS (expression, tabId?)
- pdf: export page as PDF (landscape?, scale?, tabId?)
- health: check connectivity

Token strategy: use "text" for reading (~800 tokens), "snapshot" with filter=interactive&format=compact for interactions (~3,600), diff=true on subsequent snapshots.`,
      parameters: {
        type: "object",
        properties: {
          action: {
            type: "string",
            enum: [
              "navigate",
              "snapshot",
              "click",
              "type",
              "press",
              "fill",
              "hover",
              "mousemove",
              "mousedown",
              "mouseup",
              "mousewheel",
              "scroll",
              "select",
              "focus",
              "text",
              "wait",
              "handoff",
              "tabs",
              "screenshot",
              "evaluate",
              "pdf",
              "health",
            ],
            description: "Action to perform",
          },
          url: { type: "string", description: "URL for navigate or new tab" },
          ref: {
            type: "string",
            description: "Element ref from snapshot (e.g. e5)",
          },
          text: { type: "string", description: "Text to type or fill" },
          key: {
            type: "string",
            description: "Key to press (e.g. Enter, Tab, Escape)",
          },
          expression: {
            type: "string",
            description: "JavaScript expression for evaluate",
          },
          selector: {
            type: "string",
            description: "CSS selector for snapshot scope or action target",
          },
          filter: {
            type: "string",
            enum: ["interactive", "all"],
            description: "Snapshot filter: interactive = buttons/links/inputs only",
          },
          format: {
            type: "string",
            enum: ["json", "compact", "text", "yaml"],
            description: "Snapshot format: compact is most token-efficient",
          },
          maxTokens: {
            type: "number",
            description: "Truncate snapshot to ~N tokens",
          },
          depth: { type: "number", description: "Max snapshot tree depth" },
          diff: {
            type: "boolean",
            description: "Snapshot diff: only changes since last snapshot",
          },
          value: { type: "string", description: "Value for select dropdown" },
          scrollY: {
            type: "number",
            description: "Pixels to scroll vertically",
          },
          x: {
            type: "number",
            description: "Mouse X coordinate (used by low-level mouse actions)",
          },
          y: {
            type: "number",
            description: "Mouse Y coordinate (used by low-level mouse actions)",
          },
          button: {
            type: "string",
            enum: ["left", "right", "middle"],
            description: "Mouse button for mousedown/mouseup",
          },
          wheelDeltaX: {
            type: "number",
            description: "Mouse wheel horizontal delta for mousewheel",
          },
          wheelDeltaY: {
            type: "number",
            description: "Mouse wheel vertical delta for mousewheel",
          },
          waitNav: {
            type: "boolean",
            description: "Wait for navigation after action",
          },
          tabId: { type: "string", description: "Target tab ID" },
          tabAction: {
            type: "string",
            enum: ["list", "new", "close"],
            description: "Tab sub-action (default: list)",
          },
          newTab: { type: "boolean", description: "Open URL in new tab" },
          blockImages: { type: "boolean", description: "Block image loading" },
          timeout: {
            type: "number",
            description: "Navigation timeout in seconds",
          },
          quality: {
            type: "number",
            description: "JPEG quality 1-100 (default: 80)",
          },
          mode: {
            type: "string",
            enum: ["readability", "raw"],
            description: "Text extraction mode",
          },
          ms: {
            type: "number",
            description: "Wait milliseconds for wait/handoff actions",
          },
          state: {
            type: "string",
            enum: ["visible", "hidden", "attached", "detached"],
            description: "Wait state for selector waits",
          },
          load: {
            type: "string",
            enum: ["load", "domcontentloaded", "networkidle"],
            description: "Document load state for wait action",
          },
          fn: {
            type: "string",
            description: "JavaScript predicate string for wait action",
          },
          humanReason: {
            type: "string",
            description:
              "Reason for manual handoff (captcha/login/2FA/credential input)",
          },
          humanPrompt: {
            type: "string",
            description: "Instruction shown when handing off to a human",
          },
          landscape: { type: "boolean", description: "PDF landscape orientation" },
          scale: { type: "number", description: "PDF print scale (default: 1.0)" },
        },
        required: ["action"],
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const normalized = normalizeActionParams(params);
        const { action } = normalized;

        // --- navigate ---
        if (action === "navigate") {
          const body: any = { url: normalized.url };
          if (normalized.tabId) body.tabId = normalized.tabId;
          if (normalized.newTab) body.newTab = true;
          if (normalized.blockImages) body.blockImages = true;
          if (normalized.timeout) body.timeout = normalized.timeout;
          return textResult(await pinchtabFetch(cfg, "/navigate", { body }));
        }

        // --- snapshot ---
        if (action === "snapshot") {
          const query = new URLSearchParams();
          if (normalized.tabId) query.set("tabId", normalized.tabId);
          if (normalized.filter) query.set("filter", normalized.filter);
          if (normalized.format) query.set("format", normalized.format);
          if (normalized.selector) query.set("selector", normalized.selector);
          if (normalized.maxTokens) query.set("maxTokens", String(normalized.maxTokens));
          if (normalized.depth) query.set("depth", String(normalized.depth));
          if (normalized.diff) query.set("diff", "true");
          const qs = query.toString();
          return textResult(
            await pinchtabFetch(cfg, `/snapshot${qs ? `?${qs}` : ""}`),
          );
        }

        // --- element actions ---
        const elementActions = [
          "click",
          "type",
          "press",
          "fill",
          "hover",
          "mousemove",
          "mousedown",
          "mouseup",
          "mousewheel",
          "scroll",
          "select",
          "focus",
        ];
        if (elementActions.includes(action)) {
          const body: any = { kind: action };
          for (const k of [
            "ref",
            "text",
            "key",
            "selector",
            "value",
            "scrollY",
            "x",
            "y",
            "button",
            "wheelDeltaX",
            "wheelDeltaY",
            "tabId",
            "waitNav",
          ]) {
            if (normalized[k] !== undefined) body[k] = normalized[k];
          }

          let result = await pinchtabFetch(cfg, "/action", { body });

          // One bounded retry: refresh snapshot once when refs look stale.
          if (body.ref && looksLikeStaleRef(result)) {
            const q = new URLSearchParams();
            q.set("filter", "interactive");
            q.set("format", "compact");
            if (body.tabId) q.set("tabId", body.tabId);
            await pinchtabFetch(cfg, `/snapshot?${q.toString()}`);
            const retried = await pinchtabFetch(cfg, "/action", { body });
            if (!retried?.error) {
              result = {
                ...retried,
                warning:
                  "Action succeeded after one automatic snapshot refresh (stale ref recovery).",
              };
            } else {
              result = {
                ...retried,
                warning:
                  "Action retried once after snapshot refresh but still failed. Refresh refs and retry.",
              };
            }
          }

          // Controlled input fallback: if fill fails, try type once.
          if (
            action === "fill" &&
            result?.error &&
            body.ref &&
            (typeof body.text === "string" || typeof body.value === "string")
          ) {
            const typeBody: any = { ...body, kind: "type" };
            if (typeof typeBody.text !== "string" && typeof typeBody.value === "string") {
              typeBody.text = typeBody.value;
            }
            delete typeBody.value;
            const typed = await pinchtabFetch(cfg, "/action", { body: typeBody });
            if (!typed?.error) {
              result = {
                ...typed,
                warning:
                  "Fill failed; type fallback succeeded (useful for controlled inputs).",
              };
            }
          }

          return textResult(result);
        }

        // --- text ---
        if (action === "text") {
          const query = new URLSearchParams();
          if (normalized.tabId) query.set("tabId", normalized.tabId);
          if (normalized.mode) query.set("mode", normalized.mode);
          const qs = query.toString();
          return textResult(
            await pinchtabFetch(cfg, `/text${qs ? `?${qs}` : ""}`),
          );
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
          const hasWaitCondition = ["selector", "text", "url", "load", "fn", "ms"].some(
            (k) => normalized[k] !== undefined,
          );

          const handoffMeta = {
            status: "human_handoff_required",
            reason:
              normalized.humanReason ||
              "Manual intervention required (captcha/login/2FA/credential entry).",
            instructions:
              normalized.humanPrompt ||
              "Please complete the step in the headed browser, then resume automation.",
          };

          if (!hasWaitCondition) {
            return textResult({
              ...handoffMeta,
              resumed: false,
              next:
                "Call action='handoff' with a wait condition (selector/text/url/load/fn/ms) or use action='wait' to resume when ready.",
            });
          }

          const waitBody: any = {};
          for (const k of ["selector", "text", "url", "load", "fn", "ms", "tabId", "timeout", "state"]) {
            if (normalized[k] !== undefined) waitBody[k] = normalized[k];
          }
          const waitResult = await pinchtabFetch(cfg, "/wait", { body: waitBody });
          return textResult({
            ...handoffMeta,
            resumed: !waitResult?.error,
            waitResult,
          });
        }

        // --- tabs ---
        if (action === "tabs") {
          const tabAction = normalized.tabAction || "list";
          if (tabAction === "list") {
            const listed = await pinchtabFetch(cfg, "/tabs");
            const tabs = Array.isArray(listed?.tabs)
              ? listed.tabs
              : Array.isArray(listed)
                ? listed
                : [];
            if (tabs.length > 0) {
              return textResult(listed);
            }

            // Fallback: when global /tabs is empty, try the first running instance.
            const instances = await pinchtabFetch(cfg, "/instances");
            const list = Array.isArray(instances?.value)
              ? instances.value
              : Array.isArray(instances)
                ? instances
                : [];
            const running = list.find((i: any) => i?.status === "running" && i?.id);
            if (!running) {
              return textResult({
                ...listed,
                warning:
                  "No tabs returned from /tabs and no running instance found for fallback.",
              });
            }
            const instanceTabs = await pinchtabFetch(
              cfg,
              `/instances/${running.id}/tabs`,
            );
            return textResult({
              source: "instance-fallback",
              instanceId: running.id,
              tabs: instanceTabs?.tabs ?? instanceTabs,
              warning:
                "Global /tabs was empty; used /instances/{id}/tabs fallback.",
            });
          }
          const body: any = { action: tabAction };
          if (normalized.url) body.url = normalized.url;
          if (normalized.tabId) body.tabId = normalized.tabId;
          return textResult(await pinchtabFetch(cfg, "/tab", { body }));
        }

        // --- screenshot ---
        if (action === "screenshot") {
          const query = new URLSearchParams();
          if (normalized.tabId) query.set("tabId", normalized.tabId);
          if (normalized.quality) query.set("quality", String(normalized.quality));
          const qs = query.toString();
          try {
            const res = await pinchtabFetch(
              cfg,
              `/screenshot${qs ? `?${qs}` : ""}`,
              { rawResponse: true },
            );
            if (res instanceof Response) {
              if (!res.ok) {
                return textResult({
                  error: `Screenshot failed: ${res.status} ${await res.text()}`,
                });
              }
              const buf = await res.arrayBuffer();
              const b64 = Buffer.from(buf).toString("base64");
              return {
                content: [{ type: "image", data: b64, mimeType: "image/jpeg" }],
              };
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
          if (normalized.tabId) query.set("tabId", normalized.tabId);
          if (normalized.landscape) query.set("landscape", "true");
          if (normalized.scale) query.set("scale", String(normalized.scale));
          const qs = query.toString();
          return textResult(
            await pinchtabFetch(cfg, `/pdf${qs ? `?${qs}` : ""}`),
          );
        }

        // --- health ---
        if (action === "health") {
          return textResult(await pinchtabFetch(cfg, "/health"));
        }

        return textResult({ error: `Unknown action: ${action}` });
      },
    },
    { optional: true },
  );
}
