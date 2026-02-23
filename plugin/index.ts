/**
 * Pinchtab OpenClaw Plugin
 *
 * Exposes browser control as agent tools: navigate, snapshot, action, text, screenshot.
 * Wraps Pinchtab's HTTP API so agents get structured tool calls instead of shelling out to curl.
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

function getConfig(api: PluginApi): PluginConfig {
  return api.config?.plugins?.entries?.pinchtab?.config ?? {};
}

async function pinchtabFetch(
  cfg: PluginConfig,
  path: string,
  opts: { method?: string; body?: unknown } = {},
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
    const text = await res.text();
    if (!res.ok) {
      return { error: `${res.status} ${res.statusText}`, body: text };
    }
    try {
      return JSON.parse(text);
    } catch {
      return { text };
    }
  } finally {
    clearTimeout(timer);
  }
}

export default function register(api: PluginApi) {
  // --- navigate ---
  api.registerTool(
    {
      name: "pinchtab_navigate",
      description:
        "Navigate to a URL in Pinchtab browser. Returns page info after navigation.",
      parameters: {
        type: "object",
        properties: {
          url: { type: "string", description: "URL to navigate to" },
          tabId: { type: "string", description: "Target tab ID (optional)" },
          newTab: {
            type: "boolean",
            description: "Open in new tab",
          },
          blockImages: {
            type: "boolean",
            description: "Block image loading",
          },
          timeout: {
            type: "number",
            description: "Navigation timeout in seconds",
          },
        },
        required: ["url"],
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const body: any = { url: params.url };
        if (params.tabId) body.tabId = params.tabId;
        if (params.newTab) body.newTab = true;
        if (params.blockImages) body.blockImages = true;
        if (params.timeout) body.timeout = params.timeout;
        const result = await pinchtabFetch(cfg, "/navigate", { body });
        return {
          content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
        };
      },
    },
    { optional: true },
  );

  // --- snapshot ---
  api.registerTool(
    {
      name: "pinchtab_snapshot",
      description:
        "Get accessibility tree snapshot from current page. Returns refs (e.g. e0, e5) for use with pinchtab_action. Use filter=interactive and format=compact for best token efficiency.",
      parameters: {
        type: "object",
        properties: {
          tabId: { type: "string", description: "Target tab ID" },
          filter: {
            type: "string",
            enum: ["interactive", "all"],
            description:
              "Filter nodes: 'interactive' for buttons/links/inputs only (recommended)",
          },
          format: {
            type: "string",
            enum: ["json", "compact", "text", "yaml"],
            description:
              "Output format: 'compact' is most token-efficient (recommended)",
          },
          selector: {
            type: "string",
            description: "CSS selector to scope snapshot (e.g. 'main')",
          },
          maxTokens: {
            type: "number",
            description: "Truncate output to ~N tokens",
          },
          depth: { type: "number", description: "Max tree depth" },
          diff: {
            type: "boolean",
            description:
              "Only return changes since last snapshot (great for multi-step workflows)",
          },
        },
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const query = new URLSearchParams();
        if (params.tabId) query.set("tabId", params.tabId);
        if (params.filter) query.set("filter", params.filter);
        if (params.format) query.set("format", params.format);
        if (params.selector) query.set("selector", params.selector);
        if (params.maxTokens) query.set("maxTokens", String(params.maxTokens));
        if (params.depth) query.set("depth", String(params.depth));
        if (params.diff) query.set("diff", "true");
        const qs = query.toString();
        const result = await pinchtabFetch(cfg, `/snapshot${qs ? `?${qs}` : ""}`);
        const text =
          typeof result === "string"
            ? result
            : result?.text ?? JSON.stringify(result, null, 2);
        return { content: [{ type: "text", text }] };
      },
    },
    { optional: true },
  );

  // --- action ---
  api.registerTool(
    {
      name: "pinchtab_action",
      description:
        "Perform an action on a page element by ref (from snapshot). Supports click, type, press, fill, hover, scroll, select, focus.",
      parameters: {
        type: "object",
        properties: {
          kind: {
            type: "string",
            enum: [
              "click",
              "type",
              "press",
              "fill",
              "hover",
              "scroll",
              "select",
              "focus",
            ],
            description: "Action type",
          },
          ref: {
            type: "string",
            description: "Element ref from snapshot (e.g. e5)",
          },
          text: {
            type: "string",
            description: "Text to type or fill",
          },
          key: {
            type: "string",
            description: "Key to press (e.g. Enter, Tab, Escape)",
          },
          selector: {
            type: "string",
            description: "CSS selector (alternative to ref)",
          },
          value: {
            type: "string",
            description: "Value for select dropdown",
          },
          scrollY: {
            type: "number",
            description: "Pixels to scroll vertically",
          },
          tabId: { type: "string", description: "Target tab ID" },
          waitNav: {
            type: "boolean",
            description: "Wait for navigation after action",
          },
        },
        required: ["kind"],
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const body: any = { kind: params.kind };
        for (const k of [
          "ref",
          "text",
          "key",
          "selector",
          "value",
          "scrollY",
          "tabId",
          "waitNav",
        ]) {
          if (params[k] !== undefined) body[k] = params[k];
        }
        const result = await pinchtabFetch(cfg, "/action", { body });
        return {
          content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
        };
      },
    },
    { optional: true },
  );

  // --- text ---
  api.registerTool(
    {
      name: "pinchtab_text",
      description:
        "Extract readable text from current page (readability mode). Cheapest option at ~1K tokens. Returns url, title, text.",
      parameters: {
        type: "object",
        properties: {
          tabId: { type: "string", description: "Target tab ID" },
          mode: {
            type: "string",
            enum: ["readability", "raw"],
            description: "Extraction mode (default: readability)",
          },
        },
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const query = new URLSearchParams();
        if (params.tabId) query.set("tabId", params.tabId);
        if (params.mode) query.set("mode", params.mode);
        const qs = query.toString();
        const result = await pinchtabFetch(cfg, `/text${qs ? `?${qs}` : ""}`);
        return {
          content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
        };
      },
    },
    { optional: true },
  );

  // --- tabs ---
  api.registerTool(
    {
      name: "pinchtab_tabs",
      description:
        "List open browser tabs or manage tabs (new, close). Returns tab IDs for use with other tools.",
      parameters: {
        type: "object",
        properties: {
          action: {
            type: "string",
            enum: ["list", "new", "close"],
            description: "Tab action (default: list)",
          },
          url: {
            type: "string",
            description: "URL for new tab",
          },
          tabId: {
            type: "string",
            description: "Tab ID to close",
          },
        },
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const action = params.action || "list";
        if (action === "list") {
          const result = await pinchtabFetch(cfg, "/tabs");
          return {
            content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
          };
        }
        const body: any = { action };
        if (params.url) body.url = params.url;
        if (params.tabId) body.tabId = params.tabId;
        const result = await pinchtabFetch(cfg, "/tab", { body });
        return {
          content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
        };
      },
    },
    { optional: true },
  );

  // --- screenshot ---
  api.registerTool(
    {
      name: "pinchtab_screenshot",
      description:
        "Take a screenshot of the current page. Returns JPEG image as base64.",
      parameters: {
        type: "object",
        properties: {
          tabId: { type: "string", description: "Target tab ID" },
          quality: {
            type: "number",
            description: "JPEG quality 1-100 (default: 80)",
          },
        },
      },
      async execute(_id: string, params: any) {
        const cfg = getConfig(api);
        const query = new URLSearchParams();
        if (params.tabId) query.set("tabId", params.tabId);
        if (params.quality) query.set("quality", String(params.quality));
        const qs = query.toString();
        const base = cfg.baseUrl || "http://localhost:9867";
        const url = `${base}/screenshot${qs ? `?${qs}` : ""}`;
        const headers: Record<string, string> = {};
        if (cfg.token) headers["Authorization"] = `Bearer ${cfg.token}`;

        const controller = new AbortController();
        const timeout = cfg.timeout || 30000;
        const timer = setTimeout(() => controller.abort(), timeout);

        try {
          const res = await fetch(url, { headers, signal: controller.signal });
          if (!res.ok) {
            const text = await res.text();
            return {
              content: [
                {
                  type: "text",
                  text: `Screenshot failed: ${res.status} ${text}`,
                },
              ],
            };
          }
          const buf = await res.arrayBuffer();
          const b64 = Buffer.from(buf).toString("base64");
          return {
            content: [
              {
                type: "image",
                data: b64,
                mimeType: "image/jpeg",
              },
            ],
          };
        } finally {
          clearTimeout(timer);
        }
      },
    },
    { optional: true },
  );

  // --- health ---
  api.registerTool(
    {
      name: "pinchtab_health",
      description:
        "Check if Pinchtab is running and responsive. Use to verify connectivity before other operations.",
      parameters: {
        type: "object",
        properties: {},
      },
      async execute() {
        const cfg = getConfig(api);
        const result = await pinchtabFetch(cfg, "/health");
        return {
          content: [{ type: "text", text: JSON.stringify(result, null, 2) }],
        };
      },
    },
    { optional: true },
  );
}
