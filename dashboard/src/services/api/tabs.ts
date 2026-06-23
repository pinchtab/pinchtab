import type { InstanceTab } from "../../generated/types";
import { request, requestBlob } from "./client";

export async function fetchTabScreenshot(
  tabId: string,
  format: "jpeg" | "png" = "jpeg",
): Promise<Blob> {
  return requestBlob(
    `/tabs/${encodeURIComponent(tabId)}/screenshot?raw=true&format=${format}`,
  );
}

export async function fetchTabPdf(tabId: string): Promise<Blob> {
  return requestBlob(`/tabs/${encodeURIComponent(tabId)}/pdf?raw=true`);
}

export async function closeTab(tabId: string): Promise<void> {
  await request(`/tabs/${encodeURIComponent(tabId)}/close`, { method: "POST" });
}

export async function resumeTab(
  tabId: string,
  status = "human_completed",
): Promise<void> {
  await request(`/tabs/${encodeURIComponent(tabId)}/resume`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  });
}

export interface ConsoleLogEntry {
  timestamp: string;
  level: string;
  message: string;
  source?: string;
}

export interface ErrorLogEntry {
  timestamp: string;
  message: string;
  type?: string;
  url?: string;
  line?: number;
  column?: number;
  stack?: string;
}

export async function fetchConsoleLogs(
  tabId: string,
): Promise<ConsoleLogEntry[]> {
  const res = await request<{ console: ConsoleLogEntry[] }>(
    `/console?tabId=${encodeURIComponent(tabId)}`,
  );
  return res.console || [];
}

export async function fetchErrorLogs(tabId: string): Promise<ErrorLogEntry[]> {
  const res = await request<{ errors: ErrorLogEntry[] }>(
    `/errors?tabId=${encodeURIComponent(tabId)}`,
  );
  return res.errors || [];
}

export async function navigateTab(
  tabId: string,
  url: string,
): Promise<unknown> {
  return request("/navigate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tabId, url }),
  });
}

export async function sendAction(
  body: Record<string, unknown>,
): Promise<unknown> {
  return request("/action", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function fetchAllTabs(): Promise<InstanceTab[]> {
  return request<InstanceTab[]>("/instances/tabs");
}
