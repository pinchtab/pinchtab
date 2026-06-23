import { readFile, stat } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import type { PluginConfig, PluginRuntimeContext } from "./types.js";
import { rememberRuntimeContext } from "./session_state.js";

export function normalizeDiscoveredHost(bind: string): string {
  if (bind === "0.0.0.0") return "127.0.0.1";
  if (bind === "::") return "::1";
  return bind;
}

export function formatHostForUrl(host: string): string {
  if (host.includes(":") && !host.startsWith("[") && !host.endsWith("]")) {
    return `[${host}]`;
  }
  return host;
}

export function isLocalHost(baseUrl: string): boolean {
  try {
    const url = new URL(baseUrl);
    const host = url.hostname.toLowerCase();
    return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]";
  } catch {
    return false;
  }
}

export function formatDiscoveredBaseUrl(bind: string, port: string | number): string {
  return `http://${formatHostForUrl(normalizeDiscoveredHost(bind))}:${port}`;
}

export function resolveProfile(
  cfg: PluginConfig,
  profile?: string,
  context?: PluginRuntimeContext,
): { instanceId?: string; attach?: boolean } {
  rememberRuntimeContext(context);
  const name = profile || cfg.defaultProfile || "openclaw";
  if (cfg.profiles?.[name]) {
    return cfg.profiles[name];
  }
  if (name === "user") {
    return { attach: true };
  }
  return {};
}

// Cache the parsed ~/.pinchtab/config.json keyed by file mtime so resolveEffectiveConfig
// stops paying readFile + JSON.parse on every tool call. An unchanged file is served from
// cache (stat only); a changed file is re-parsed; a missing/unreadable file clears the cache.
let discoveredCache: {
  mtimeMs: number;
  value: { baseUrl?: string; token?: string } | null;
} | null = null;

async function discoverPinchtabConfig(): Promise<{ baseUrl?: string; token?: string } | null> {
  const path = join(homedir(), ".pinchtab", "config.json");
  try {
    const info = await stat(path);
    if (discoveredCache && discoveredCache.mtimeMs === info.mtimeMs) {
      return discoveredCache.value;
    }

    const raw = await readFile(path, "utf8");
    const parsed = JSON.parse(raw);
    const bind = parsed?.server?.bind || "127.0.0.1";
    const port = parsed?.server?.port;
    const token = parsed?.server?.token;

    let baseUrl: string | undefined;
    if (port) {
      const host = formatHostForUrl(normalizeDiscoveredHost(bind));
      baseUrl = `http://${host}:${port}`;
    }

    const value = {
      baseUrl,
      token: typeof token === "string" && token ? token : undefined,
    };
    discoveredCache = { mtimeMs: info.mtimeMs, value };
    return value;
  } catch {
    discoveredCache = null;
  }
  return null;
}

export async function resolveEffectiveConfig(cfg: PluginConfig): Promise<PluginConfig> {
  if (cfg.baseUrl && cfg.token) return cfg;
  const discovered = await discoverPinchtabConfig();
  return {
    ...cfg,
    baseUrl: cfg.baseUrl || discovered?.baseUrl || "http://localhost:9867",
    token: cfg.token || discovered?.token,
  };
}
