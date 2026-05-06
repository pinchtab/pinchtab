import type { PluginConfig, PluginRuntimeContext, ToolResult } from "./types.js";

// Pinchtab isolates browser state per session token created by `POST /sessions`
// with a distinct agentId. Without per-agent sessions every OpenClaw agent
// shares the same browser context, so cookies/storage leak across them.
const pinchtabSessionTokens = new Map<string, string>();

const SERVER_TOKEN_PATH_PREFIXES = ["/sessions", "/instances", "/profiles"];

function isServerTokenPath(path: string): boolean {
  const stripped = path.split("?", 1)[0];
  if (stripped === "/health") return true;
  for (const prefix of SERVER_TOKEN_PATH_PREFIXES) {
    if (stripped === prefix || stripped.startsWith(`${prefix}/`)) return true;
  }
  return false;
}

type SessionResolution =
  | { kind: "ok"; token: string }
  | { kind: "no-agent" }
  | { kind: "error"; error: string };

async function ensurePinchtabSession(
  cfg: PluginConfig,
  context: PluginRuntimeContext | undefined,
  forceRefresh = false,
): Promise<SessionResolution> {
  const agentId = context?.agentId;
  if (!agentId) return { kind: "no-agent" };
  if (!cfg.token || !cfg.baseUrl) {
    return { kind: "error", error: "Pinchtab plugin missing baseUrl or token; cannot create per-agent session" };
  }
  if (forceRefresh) pinchtabSessionTokens.delete(agentId);
  const cached = pinchtabSessionTokens.get(agentId);
  if (cached) return { kind: "ok", token: cached };

  const controller = new AbortController();
  const timeout = cfg.timeoutMs ?? cfg.timeout ?? 30000;
  const timer = setTimeout(() => controller.abort(), timeout);
  try {
    const res = await fetch(`${cfg.baseUrl}/sessions`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${cfg.token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ agentId, label: "openclaw" }),
      signal: controller.signal,
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      return {
        kind: "error",
        error: `Pinchtab session creation failed for agent ${agentId}: ${res.status} ${res.statusText}${body ? ` - ${body.slice(0, 200)}` : ""}`,
      };
    }
    const data = (await res.json().catch(() => null)) as { sessionToken?: string } | null;
    const token = data?.sessionToken;
    if (typeof token === "string" && token) {
      pinchtabSessionTokens.set(agentId, token);
      return { kind: "ok", token };
    }
    return { kind: "error", error: `Pinchtab /sessions response missing sessionToken for agent ${agentId}` };
  } catch (err: any) {
    const reason = err?.name === "AbortError" ? `timed out after ${timeout}ms` : (err?.message || String(err));
    return { kind: "error", error: `Pinchtab session creation failed for agent ${agentId}: ${reason}` };
  } finally {
    clearTimeout(timer);
  }
}

function evictPinchtabSession(agentId: string | undefined): void {
  if (agentId) pinchtabSessionTokens.delete(agentId);
}

export function clearPinchtabSessionCache(): void {
  pinchtabSessionTokens.clear();
}

function buildRequestHeaders(
  cfg: PluginConfig,
  context: PluginRuntimeContext | undefined,
  authToken: string | undefined,
  authScheme: "Bearer" | "Session",
): Record<string, string> {
  const headers: Record<string, string> = {};
  if (authToken) headers["Authorization"] = `${authScheme} ${authToken}`;
  if (context?.agentId) headers["X-OpenClaw-Agent-Id"] = context.agentId;
  if (context?.sessionId) headers["X-OpenClaw-Session-Id"] = context.sessionId;
  if (context?.sessionKey) headers["X-OpenClaw-Session-Key"] = context.sessionKey;
  return headers;
}

/**
 * Caller must pass an already-resolved cfg (see `resolveEffectiveConfig` in session.ts).
 * The two tool entry points (`executePinchtabAction`, `executeBrowserAction`) resolve once
 * at the top and pass the resolved cfg through every subsequent call. Skipping resolution
 * here means a raw cfg with empty baseUrl/token will hit `http://localhost:9867` blindly.
 */
export async function pinchtabFetch(
  cfg: PluginConfig,
  path: string,
  opts: { method?: string; body?: unknown; rawResponse?: boolean } = {},
  context?: PluginRuntimeContext,
): Promise<any> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const resolvedCfg = { ...cfg, baseUrl: base };

  return performPinchtabFetch(resolvedCfg, path, opts, context, false);
}

async function performPinchtabFetch(
  cfg: PluginConfig,
  path: string,
  opts: { method?: string; body?: unknown; rawResponse?: boolean },
  context: PluginRuntimeContext | undefined,
  alreadyRetriedSession: boolean,
): Promise<any> {
  const base = cfg.baseUrl as string;
  const url = `${base}${path}`;
  let authToken = cfg.token;
  let authScheme: "Bearer" | "Session" = "Bearer";

  if (!isServerTokenPath(path)) {
    const resolution = await ensurePinchtabSession(cfg, context);
    if (resolution.kind === "error") {
      // Fail closed: do NOT silently fall back to the shared server token.
      // Without a per-agent session, this agent would join the shared browser
      // context, defeating per-agent isolation.
      return { error: resolution.error };
    }
    if (resolution.kind === "ok") {
      authToken = resolution.token;
      authScheme = "Session";
    }
    // kind === "no-agent": legitimate bearer use (no agent context, e.g. CLI smoke).
  }

  const headers = buildRequestHeaders(cfg, context, authToken, authScheme);
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

    // Cached agent-session token may have been revoked or expired upstream; on
    // the first 401 we evict and retry once to recover without restarting.
    if (res.status === 401 && authScheme === "Session" && !alreadyRetriedSession) {
      evictPinchtabSession(context?.agentId);
      // Drain body to free the connection.
      await res.text().catch(() => "");
      return performPinchtabFetch(cfg, path, opts, context, true);
    }

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

const STALE_REF_PATTERNS: RegExp[] = [
  /\bstale\b/,
  /\bref\b/,
  /\bno node\b/,
  /\bunknown element\b/,
  /\bcontext canceled\b/,
  // Pinchtab backend-node failures: the cached ref points at a CDP node id
  // that's been detached/garbage-collected.
  /backend[\s-]?node/,
  /\belement not found in dom\b/,
  /\belement no longer (?:in|attached)\b/,
];

export function looksLikeStaleRef(result: any): boolean {
  if (!result?.error) return false;
  const text = actionErrorText(result);
  return STALE_REF_PATTERNS.some((re) => re.test(text));
}
