import type {
  Agent,
  ActivityEvent,
  DashboardHealth,
  Instance,
  InstanceMetrics,
  InstanceTab,
  Profile,
  Settings,
} from "../generated/types";
import type { FileConfig } from "../generated/config";

export type {
  Profile,
  Instance,
  InstanceTab,
  Agent,
  ActivityEvent,
  DashboardHealth,
};

export interface MonitoringServerMetrics {
  goHeapAllocMB: number;
  goNumGoroutine: number;
  rateBucketHosts: number;
}

export interface MonitoringSnapshot {
  timestamp: number;
  instances: Instance[];
  tabs: InstanceTab[];
  metrics: InstanceMetrics[];
  serverMetrics: MonitoringServerMetrics;
}

type Settled<T> = T extends readonly unknown[]
  ? T
  : T extends object
    ? { [K in keyof T]-?: Settled<T[K]> }
    : T;

export type BackendConfig = Settled<FileConfig>;

export interface BackendConfigState {
  config: BackendConfig;
  configPath: string;
  tokenConfigured: boolean;
  restartRequired: boolean;
  restartReasons: string[];
}

export function normalizeBackendConfigState(
  input: BackendConfigState,
): BackendConfigState {
  const { browser, security, autoSolver } = input.config;
  return {
    ...input,
    restartReasons: input.restartReasons ?? [],
    config: {
      ...input.config,
      browser: { ...browser, extensionPaths: browser.extensionPaths ?? [] },
      security: {
        ...security,
        allowedDomains: security.allowedDomains ?? [],
        trustedProxyCIDRs: security.trustedProxyCIDRs ?? [],
        trustedResolveCIDRs: security.trustedResolveCIDRs ?? [],
        attach: {
          ...security.attach,
          allowHosts: security.attach.allowHosts ?? [],
          allowSchemes: security.attach.allowSchemes ?? [],
        },
        idpi: {
          ...security.idpi,
          customPatterns: security.idpi.customPatterns ?? [],
        },
      },
      autoSolver: { ...autoSolver, solvers: autoSolver.solvers ?? [] },
    },
  };
}

export function normalizeDashboardHealth(
  input: DashboardHealth,
): DashboardHealth {
  return {
    ...input,
    authRequired: input.authRequired ?? false,
    restartRequired: input.restartRequired ?? false,
    restartReasons: input.restartReasons ?? [],
  };
}

export function normalizeMonitoringSnapshot(
  input: Partial<MonitoringSnapshot>,
): MonitoringSnapshot {
  return {
    timestamp: input.timestamp ?? Date.now(),
    instances: (input.instances ?? []).map((instance) => ({
      ...instance,
      mode: instance.mode ?? (instance.headless ? "headless" : "headed"),
    })),
    tabs: input.tabs ?? [],
    metrics: input.metrics ?? [],
    serverMetrics: {
      goHeapAllocMB: input.serverMetrics?.goHeapAllocMB ?? 0,
      goNumGoroutine: input.serverMetrics?.goNumGoroutine ?? 0,
      rateBucketHosts: input.serverMetrics?.rateBucketHosts ?? 0,
    },
  };
}

export type { Settings as LocalDashboardSettings };
