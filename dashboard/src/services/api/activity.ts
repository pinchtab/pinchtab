import type {
  ActivityQuery,
  DashboardActivityResponse,
} from "../../activities/types";
import { request } from "./client";

export async function fetchActivity(
  query?: ActivityQuery,
): Promise<DashboardActivityResponse> {
  const params = new URLSearchParams();
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === "") {
        continue;
      }
      params.set(key, String(value));
    }
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return request<DashboardActivityResponse>(`/api/activity${suffix}`);
}
