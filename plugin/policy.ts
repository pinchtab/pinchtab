import type { PluginConfig } from "./types.js";
import { textResult } from "./client.js";

export function matchesDomain(url: string, patterns: string[]): boolean {
  if (!patterns || patterns.length === 0) return true;
  try {
    const hostname = new URL(url).hostname;
    return patterns.some((p) => {
      if (p.startsWith("*.")) {
        const suffix = p.slice(2);
        return hostname === suffix || hostname.endsWith("." + suffix);
      }
      return hostname === p;
    });
  } catch {
    return false;
  }
}

export function checkNavigationPolicy(cfg: PluginConfig, url?: string): { allowed: boolean; error?: string } {
  if (!url) return { allowed: true };
  if (cfg.allowedDomains && cfg.allowedDomains.length > 0) {
    if (!matchesDomain(url, cfg.allowedDomains)) {
      return { allowed: false, error: `Navigation blocked: ${url} not in allowedDomains` };
    }
  }
  return { allowed: true };
}

export function checkEvaluatePolicy(cfg: PluginConfig): { allowed: boolean; error?: string } {
  if (cfg.allowEvaluate !== true) {
    return { allowed: false, error: "evaluate action blocked by plugin policy (allowEvaluate: false)" };
  }
  return { allowed: true };
}

export function checkDownloadPolicy(cfg: PluginConfig): { allowed: boolean; error?: string } {
  if (cfg.allowDownloads !== true) {
    return { allowed: false, error: "downloads blocked by plugin policy (allowDownloads: false)" };
  }
  return { allowed: true };
}

export function checkUploadPolicy(cfg: PluginConfig): { allowed: boolean; error?: string } {
  if (cfg.allowUploads !== true) {
    return { allowed: false, error: "uploads blocked by plugin policy (allowUploads: false)" };
  }
  return { allowed: true };
}

export function enforcePolicyOrReturn(check: { allowed: boolean; error?: string }): any | null {
  if (!check.allowed) {
    return textResult({ error: check.error });
  }
  return null;
}
