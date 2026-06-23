import type { InstanceMetrics } from "../../generated/types";
import type { DashboardServerInfo, MonitoringServerMetrics } from "../../types";
import { normalizeDashboardServerInfo } from "../../types";
import { request } from "./client";

export async function fetchAllMetrics(): Promise<InstanceMetrics[]> {
  return request<InstanceMetrics[]>("/instances/metrics");
}

export async function fetchServerMetrics(): Promise<MonitoringServerMetrics> {
  const res = await request<{ metrics: MonitoringServerMetrics }>(
    "/api/metrics",
  );
  return res.metrics;
}

// Health
export async function fetchHealth(): Promise<DashboardServerInfo> {
  return normalizeDashboardServerInfo(
    await request<DashboardServerInfo>("/health"),
  );
}
