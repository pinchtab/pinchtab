import type { BackendConfig, BackendConfigState } from "../../types";
import { normalizeBackendConfigState } from "../../types";
import { request } from "./client";

export async function fetchBackendConfig(): Promise<BackendConfigState> {
  return normalizeBackendConfigState(
    await request<BackendConfigState>("/api/config"),
  );
}

export async function saveBackendConfig(
  config: BackendConfig,
): Promise<BackendConfigState> {
  return normalizeBackendConfigState(
    await request<BackendConfigState>("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    }),
  );
}
