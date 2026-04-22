import type { PluginConfig } from "./types.js";
import { pinchtabFetch } from "./client.js";

let serverStarted = false;
let lastTabId: string | undefined;

export function getLastTabId(): string | undefined {
  return lastTabId;
}

export function setLastTabId(tabId: string | undefined): void {
  lastTabId = tabId;
}

export function resolveProfile(cfg: PluginConfig, profile?: string): { instanceId?: string; attach?: boolean } {
  const name = profile || cfg.defaultProfile || "openclaw";
  if (cfg.profiles?.[name]) {
    return cfg.profiles[name];
  }
  if (name === "user") {
    return { attach: true };
  }
  return {};
}

export async function getEnhancedHealth(cfg: PluginConfig): Promise<any> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const health = await pinchtabFetch(cfg, "/health");
  const serverOk = !health?.error;

  const result: any = {
    server: serverOk ? "ok" : "unreachable",
    baseUrl: base,
    autoStart: cfg.autoStart !== false,
    defaultProfile: cfg.defaultProfile || "openclaw",
    policies: {
      allowEvaluate: cfg.allowEvaluate === true,
      allowDownloads: cfg.allowDownloads === true,
      allowUploads: cfg.allowUploads === true,
      allowedDomains: cfg.allowedDomains?.length ? cfg.allowedDomains : "all",
    },
  };

  if (serverOk) {
    result.serverHealth = health;
    if (health?.version) result.serverVersion = health.version;
  } else {
    result.error = health?.error;
  }

  const warnings: string[] = [];
  if (cfg.allowEvaluate) warnings.push("evaluate enabled - JS execution allowed");
  if (!cfg.allowedDomains?.length) warnings.push("no domain restrictions");
  if (warnings.length) result.warnings = warnings;

  return result;
}

export async function ensureServerRunning(cfg: PluginConfig): Promise<{ ok: boolean; error?: string }> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const isLocal = base.includes("localhost") || base.includes("127.0.0.1");

  const healthCheck = await pinchtabFetch(cfg, "/health");
  if (!healthCheck?.error) {
    return { ok: true };
  }

  if (!isLocal || cfg.autoStart === false || serverStarted) {
    return { ok: false, error: healthCheck.error };
  }

  const binary = cfg.binaryPath || "pinchtab";
  const startupTimeout = cfg.startupTimeoutMs ?? 30000;

  try {
    const { spawn } = await import("child_process");
    const proc = spawn(binary, ["server"], {
      detached: true,
      stdio: "ignore",
    });
    proc.unref();
    serverStarted = true;

    const startTime = Date.now();
    while (Date.now() - startTime < startupTimeout) {
      await new Promise((r) => setTimeout(r, 500));
      const check = await pinchtabFetch(cfg, "/health");
      if (!check?.error) {
        return { ok: true };
      }
    }
    return { ok: false, error: `Server failed to start within ${startupTimeout}ms` };
  } catch (err: any) {
    return { ok: false, error: `Failed to start pinchtab: ${err?.message}` };
  }
}
