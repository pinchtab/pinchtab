import type {
  Agent,
  ActivityEvent,
  BrowserSettings,
  CreateProfileRequest,
  Instance,
  InstanceMetrics,
  InstanceTab,
  LaunchInstanceRequest,
  Profile,
  ScreencastSettings,
  Settings,
} from "../generated/types";

export type {
  Profile,
  Instance,
  InstanceTab,
  InstanceMetrics,
  Agent,
  ActivityEvent,
  Settings,
  ScreencastSettings,
  BrowserSettings,
  CreateProfileRequest,
  LaunchInstanceRequest,
};

export interface DashboardServerInfo {
  status: string;
  mode: string;
  version: string;
  uptime: number;
  authRequired?: boolean;
  profiles: number;
  instances: number;
  agents: number;
  restartRequired?: boolean;
  restartReasons?: string[];
}

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

export interface BackendServerConfig {
  port: string;
  bind: string;
  token: string;
  stateDir: string;
  trustProxyHeaders: boolean;
  cookieSecure?: boolean;
}

export interface BackendDashboardSessionConfig {
  persist: boolean;
  idleTimeoutSec: number;
  maxLifetimeSec: number;
  elevationWindowSec: number;
  persistElevationAcrossRestart: boolean;
  requireElevation: boolean;
}

export interface BackendSessionsConfig {
  dashboard: BackendDashboardSessionConfig;
}

export interface BackendActivityEventsConfig {
  dashboard: boolean;
  server: boolean;
  bridge: boolean;
  orchestrator: boolean;
  scheduler: boolean;
  mcp: boolean;
  other: boolean;
}

export interface BackendActivityConfig {
  enabled: boolean;
  sessionIdleSec: number;
  retentionDays: number;
  stateDir: string;
  events: BackendActivityEventsConfig;
}

export interface BackendObservabilityConfig {
  activity: BackendActivityConfig;
}

export interface BackendBrowserConfig {
  version: string;
  binary: string;
  extraFlags: string;
  extensionPaths: string[];
}

export interface BackendInstanceDefaultsConfig {
  mode: "headless" | "headed";
  noRestore: boolean;
  timezone: string;
  blockImages: boolean;
  blockMedia: boolean;
  blockAds: boolean;
  maxTabs: number;
  maxParallelTabs: number;
  userAgent: string;
  noAnimations: boolean;
  stealthLevel: "light" | "medium" | "full";
  tabEvictionPolicy: "reject" | "close_oldest" | "close_lru";
}

export interface BackendSecurityConfig {
  allowEvaluate: boolean;
  allowMacro: boolean;
  allowScreencast: boolean;
  allowDownload: boolean;
  allowUpload: boolean;
  allowedDomains: string[];
  trustedProxyCIDRs: string[];
  trustedResolveCIDRs: string[];
  attach: BackendAttachConfig;
  idpi: BackendIDPIConfig;
}

export interface BackendProfilesConfig {
  baseDir: string;
  defaultProfile: string;
}

export interface BackendMultiInstanceConfig {
  strategy:
    | "simple"
    | "explicit"
    | "simple-autorestart"
    | "always-on"
    | "no-instance";
  allocationPolicy: "fcfs" | "round_robin" | "random";
  instancePortStart: number;
  instancePortEnd: number;
  restart: BackendMultiInstanceRestartConfig;
}

export interface BackendMultiInstanceRestartConfig {
  maxRestarts: number;
  initBackoffSec: number;
  maxBackoffSec: number;
  stableAfterSec: number;
}

export interface BackendAttachConfig {
  enabled: boolean;
  allowHosts: string[];
  allowSchemes: string[];
}

export interface BackendIDPIConfig {
  enabled: boolean;
  strictMode: boolean;
  scanContent: boolean;
  wrapContent: boolean;
  customPatterns: string[];
}

export interface BackendTimeoutsConfig {
  actionSec: number;
  navigateSec: number;
  shutdownSec: number;
  waitNavMs: number;
}

export interface BackendAutoSolverConfig {
  enabled: boolean;
  maxAttempts: number;
  solvers: string[];
  llmProvider: string;
  llmFallback: boolean;
}

export interface BackendConfig {
  server: BackendServerConfig;
  browser: BackendBrowserConfig;
  instanceDefaults: BackendInstanceDefaultsConfig;
  security: BackendSecurityConfig;
  profiles: BackendProfilesConfig;
  multiInstance: BackendMultiInstanceConfig;
  timeouts: BackendTimeoutsConfig;
  observability: BackendObservabilityConfig;
  sessions: BackendSessionsConfig;
  autoSolver: BackendAutoSolverConfig;
}

export interface BackendConfigState {
  config: BackendConfig;
  configPath: string;
  tokenConfigured: boolean;
  restartRequired: boolean;
  restartReasons: string[];
}

export const defaultBackendConfig: BackendConfig = {
  server: {
    port: "9867",
    bind: "127.0.0.1",
    token: "",
    stateDir: "",
    trustProxyHeaders: false,
    cookieSecure: undefined,
  },
  browser: {
    version: "144.0.7559.133",
    binary: "",
    extraFlags: "",
    extensionPaths: [],
  },
  instanceDefaults: {
    mode: "headless",
    noRestore: false,
    timezone: "",
    blockImages: false,
    blockMedia: false,
    blockAds: false,
    maxTabs: 20,
    maxParallelTabs: 0,
    userAgent: "",
    noAnimations: false,
    stealthLevel: "light",
    tabEvictionPolicy: "close_lru",
  },
  security: {
    allowEvaluate: false,
    allowMacro: false,
    allowScreencast: false,
    allowDownload: false,
    allowUpload: false,
    allowedDomains: ["127.0.0.1", "localhost", "::1"],
    trustedProxyCIDRs: [],
    trustedResolveCIDRs: [],
    attach: {
      enabled: false,
      allowHosts: ["127.0.0.1", "localhost", "::1"],
      allowSchemes: ["ws", "wss"],
    },
    idpi: {
      enabled: true,
      strictMode: true,
      scanContent: true,
      wrapContent: true,
      customPatterns: [],
    },
  },
  profiles: {
    baseDir: "",
    defaultProfile: "default",
  },
  multiInstance: {
    strategy: "always-on",
    allocationPolicy: "fcfs",
    instancePortStart: 9868,
    instancePortEnd: 9968,
    restart: {
      maxRestarts: 20,
      initBackoffSec: 2,
      maxBackoffSec: 60,
      stableAfterSec: 300,
    },
  },
  timeouts: {
    actionSec: 30,
    navigateSec: 60,
    shutdownSec: 10,
    waitNavMs: 1000,
  },
  observability: {
    activity: {
      enabled: true,
      sessionIdleSec: 1800,
      retentionDays: 30,
      stateDir: "",
      events: {
        dashboard: false,
        server: false,
        bridge: false,
        orchestrator: false,
        scheduler: false,
        mcp: false,
        other: false,
      },
    },
  },
  sessions: {
    dashboard: {
      persist: true,
      idleTimeoutSec: 7 * 24 * 60 * 60,
      maxLifetimeSec: 7 * 24 * 60 * 60,
      elevationWindowSec: 15 * 60,
      persistElevationAcrossRestart: false,
      requireElevation: false,
    },
  },
  autoSolver: {
    enabled: false,
    maxAttempts: 8,
    solvers: ["cloudflare", "semantic"],
    llmProvider: "",
    llmFallback: false,
  },
};

export function normalizeBackendConfig(
  input?: Partial<BackendConfig> | null,
): BackendConfig {
  return {
    server: {
      ...defaultBackendConfig.server,
      ...(input?.server ?? {}),
    },
    browser: {
      ...defaultBackendConfig.browser,
      ...(input?.browser ?? {}),
      extensionPaths:
        input?.browser?.extensionPaths ??
        defaultBackendConfig.browser.extensionPaths,
    },
    instanceDefaults: {
      ...defaultBackendConfig.instanceDefaults,
      ...(input?.instanceDefaults ?? {}),
    },
    security: {
      ...defaultBackendConfig.security,
      ...(input?.security ?? {}),
      allowedDomains:
        input?.security?.allowedDomains ??
        defaultBackendConfig.security.allowedDomains,
      attach: {
        ...defaultBackendConfig.security.attach,
        ...(input?.security?.attach ?? {}),
        allowHosts:
          input?.security?.attach?.allowHosts ??
          defaultBackendConfig.security.attach.allowHosts,
        allowSchemes:
          input?.security?.attach?.allowSchemes ??
          defaultBackendConfig.security.attach.allowSchemes,
      },
      idpi: {
        ...defaultBackendConfig.security.idpi,
        ...(input?.security?.idpi ?? {}),
        customPatterns:
          input?.security?.idpi?.customPatterns ??
          defaultBackendConfig.security.idpi.customPatterns,
      },
    },
    profiles: {
      ...defaultBackendConfig.profiles,
      ...(input?.profiles ?? {}),
    },
    multiInstance: {
      ...defaultBackendConfig.multiInstance,
      ...(input?.multiInstance ?? {}),
    },
    timeouts: {
      ...defaultBackendConfig.timeouts,
      ...(input?.timeouts ?? {}),
    },
    observability: {
      ...defaultBackendConfig.observability,
      ...(input?.observability ?? {}),
      activity: {
        ...defaultBackendConfig.observability.activity,
        ...(input?.observability?.activity ?? {}),
        events: {
          ...defaultBackendConfig.observability.activity.events,
          ...(input?.observability?.activity?.events ?? {}),
        },
      },
    },
    sessions: {
      ...defaultBackendConfig.sessions,
      ...(input?.sessions ?? {}),
      dashboard: {
        ...defaultBackendConfig.sessions.dashboard,
        ...(input?.sessions?.dashboard ?? {}),
      },
    },
    autoSolver: {
      ...defaultBackendConfig.autoSolver,
      ...(input?.autoSolver ?? {}),
      solvers:
        input?.autoSolver?.solvers ?? defaultBackendConfig.autoSolver.solvers,
    },
  };
}

export function normalizeBackendConfigState(
  input: Partial<BackendConfigState>,
): BackendConfigState {
  return {
    config: normalizeBackendConfig(input.config),
    configPath: input.configPath ?? "",
    tokenConfigured: input.tokenConfigured ?? false,
    restartRequired: input.restartRequired ?? false,
    restartReasons: input.restartReasons ?? [],
  };
}

export function normalizeDashboardServerInfo(
  input: DashboardServerInfo,
): DashboardServerInfo {
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

export type {
  Settings as LocalDashboardSettings,
  ScreencastSettings as LocalScreencastSettings,
  BrowserSettings as LocalBrowserSettings,
};
