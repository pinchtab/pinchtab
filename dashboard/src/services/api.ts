// Barrel for the dashboard API. The implementation lives in ./api/* — a shared
// transport/auth core (client.ts) plus per-domain modules — so feature changes
// land in focused files. Consumers keep importing from "../services/api".
export {
  ApiError,
  isApiError,
  resetRealtimeAuthProbeStateForTests,
  probeBackendAuth,
  handleRealtimeAuthFailure,
} from "./api/client";

export * from "./api/profiles";
export * from "./api/instances";
export * from "./api/tabs";
export * from "./api/monitoring";
export * from "./api/agents";
export * from "./api/activity";
export * from "./api/auth";
export * from "./api/config";
export * from "./api/realtime";
