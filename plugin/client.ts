import type { PluginConfig, ToolResult } from "./types.js";

export async function pinchtabFetch(
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
  const timeout = cfg.timeoutMs ?? cfg.timeout ?? 30000;
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

export function textResult(data: any): ToolResult {
  const text =
    typeof data === "string" ? data : data?.text ?? JSON.stringify(data, null, 2);
  return { content: [{ type: "text", text }] };
}

export function imageResult(b64: string, mimeType: string): ToolResult {
  return { content: [{ type: "image", data: b64, mimeType }] };
}

export function resourceResult(uri: string, mimeType: string, blob: string): ToolResult {
  return { content: [{ type: "resource", resource: { uri, mimeType, blob } }] };
}

export function isRefToken(value: unknown): value is string {
  return typeof value === "string" && /^e\d+$/i.test(value.trim());
}

export function normalizeActionParams(input: any): any {
  const params = { ...input };
  if (!params.ref && isRefToken(params.selector)) {
    params.ref = String(params.selector).trim();
    delete params.selector;
  }
  if (typeof params.ref === "string") {
    params.ref = params.ref.trim();
  }
  return params;
}

export function actionErrorText(result: any): string {
  return `${result?.error || ""} ${result?.body || ""}`.toLowerCase();
}

export function looksLikeStaleRef(result: any): boolean {
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
