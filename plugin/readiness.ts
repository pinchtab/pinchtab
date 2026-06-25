import type { PluginConfig, PluginRuntimeContext } from "./types.js";
import { pinchtabFetch } from "./client.js";
import { getAgentSessionState } from "./session_state.js";
import { isLocalHost } from "./discovered_config.js";

const instanceReadyRetryDelayMs = 500;
const instanceReadyMaxWaitMs = 12000;
const readinessTtlMs = 30 * 1000;

// Positive readiness latch keyed by base URL: once the server + a running
// instance are confirmed, routine actions skip the per-call /health + /instances
// probe until the latch expires. A negative result clears the latch so the next
// action re-probes; readiness is a property of the server, not the agent.
const readinessCache = new Map<string, number>();

export async function getEnhancedHealth(cfg: PluginConfig, context?: PluginRuntimeContext): Promise<any> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const health = await pinchtabFetch(cfg, "/health", {}, context);
  const serverOk = !health?.error;
  const agentSession = getAgentSessionState(context);

  const result: any = {
    server: serverOk ? "ok" : "unreachable",
    baseUrl: base,
    defaultProfile: cfg.defaultProfile || "openclaw",
    policies: {
      allowEvaluate: cfg.allowEvaluate === true,
      allowDownloads: cfg.allowDownloads === true,
      allowUploads: cfg.allowUploads === true,
      allowedDomains: cfg.allowedDomains?.length ? cfg.allowedDomains : "all",
    },
  };

  if (agentSession) {
    result.agentSession = {
      agentId: agentSession.agentId,
      sessionId: agentSession.sessionId,
      lastTabId: agentSession.lastTabId,
    };
  }

  if (serverOk) {
    result.serverHealth = health;
    if (health?.version) result.serverVersion = health.version;
  } else {
    result.error = health?.error;
    if (isLocalHost(base)) {
      result.hint = `Pinchtab is not reachable at ${base}. Start it with: pinchtab server`;
    }
  }

  const warnings: string[] = [];
  if (cfg.allowEvaluate) warnings.push("evaluate enabled - JS execution allowed");
  if (!cfg.allowedDomains?.length) warnings.push("no domain restrictions");
  if (warnings.length) result.warnings = warnings;

  return result;
}

export async function ensureServerRunning(
  cfg: PluginConfig,
  context?: PluginRuntimeContext,
): Promise<{ ok: boolean; error?: string }> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const healthCheck = await pinchtabFetch(cfg, "/health", {}, context);
  if (!healthCheck?.error) {
    return { ok: true };
  }

  const hint = isLocalHost(base)
    ? ` Pinchtab is not running at ${base}. Start it with: pinchtab server`
    : "";
  return { ok: false, error: `${healthCheck.error}${hint}` };
}

export async function waitForInstanceReady(
  cfg: PluginConfig,
  instanceId?: string,
  context?: PluginRuntimeContext,
): Promise<{ ok: boolean; error?: string }> {
  const start = Date.now();
  let lastError = "instance not ready";

  while (Date.now() - start < instanceReadyMaxWaitMs) {
    const health = await pinchtabFetch(cfg, "/health", {}, context);
    if (!health?.error) {
      const instances = await pinchtabFetch(cfg, "/instances", {}, context);
      const list = Array.isArray(instances?.value)
        ? instances.value
        : Array.isArray(instances)
          ? instances
          : [];
      const running = instanceId
        ? list.find((instance: any) => instance?.id === instanceId && instance?.status === "running")
        : list.find((instance: any) => instance?.status === "running" && instance?.id);
      if (running) {
        return { ok: true };
      }
      if (instanceId && list.some((instance: any) => instance?.id === instanceId)) {
        lastError = `instance ${instanceId} not ready`;
      }
    } else {
      lastError = health.error || lastError;
    }

    if (instanceId) {
      await new Promise((resolve) => setTimeout(resolve, instanceReadyRetryDelayMs));
      continue;
    }

    const tabs = await pinchtabFetch(cfg, "/tabs", {}, context);
    if (!tabs?.error) {
      return { ok: true };
    }

    const text = `${tabs?.error || ""} ${tabs?.body || ""}`.toLowerCase();
    lastError = tabs?.error || lastError;

    if (!text.includes("instance not ready") && !text.includes("may be restarting") && !text.includes("503")) {
      return { ok: false, error: tabs?.error || "unknown readiness error" };
    }

    await new Promise((resolve) => setTimeout(resolve, instanceReadyRetryDelayMs));
  }

  return { ok: false, error: `Timed out waiting for PinchTab instance readiness: ${lastError}` };
}

function readinessKey(cfg: PluginConfig): string {
  return cfg.baseUrl || "http://localhost:9867";
}

// invalidateReadiness drops the readiness latch so the next ensureReady re-probes
// — call after a request fails because the server/instance went away.
export function invalidateReadiness(cfg: PluginConfig): void {
  readinessCache.delete(readinessKey(cfg));
}

// ensureReady confirms the server is reachable and an instance is running,
// caching a positive result for readinessTtlMs so steady-state actions skip the
// /health + /instances probe. The first action (and any after the latch expires
// or is invalidated) still pays the full check, so correctness is preserved.
export async function ensureReady(
  cfg: PluginConfig,
  context?: PluginRuntimeContext,
): Promise<{ ok: boolean; error?: string }> {
  const key = readinessKey(cfg);
  const readyUntil = readinessCache.get(key);
  if (readyUntil !== undefined && Date.now() < readyUntil) {
    return { ok: true };
  }
  readinessCache.delete(key);

  const serverCheck = await ensureServerRunning(cfg, context);
  if (!serverCheck.ok) return serverCheck;

  const readyCheck = await waitForInstanceReady(cfg, undefined, context);
  if (!readyCheck.ok) return readyCheck;

  readinessCache.set(key, Date.now() + readinessTtlMs);
  return { ok: true };
}
