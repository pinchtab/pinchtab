import type {
  Agent,
  ActivityEvent,
  BrowserSettings,
  CreateProfileRequest,
  Instance,
  LaunchInstanceRequest,
  Profile,
  ScreencastSettings,
  Settings,
} from "../generated/types";

export type {
  Profile,
  Instance,
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
  profiles: number;
  instances: number;
  agents: number;
  restartRequired?: boolean;
  restartReasons?: string[];
}

export interface BackendServerConfig {
  port: string;
  bind: string;
  token: string;
  stateDir: string;
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
}

export interface BackendProfilesConfig {
  baseDir: string;
  defaultProfile: string;
}

export interface BackendMultiInstanceConfig {
  strategy: "simple" | "explicit" | "simple-autorestart";
  allocationPolicy: "fcfs" | "round_robin" | "random";
  instancePortStart: number;
  instancePortEnd: number;
}

export interface BackendAttachConfig {
  enabled: boolean;
  allowHosts: string[];
  allowSchemes: string[];
}

export interface BackendTimeoutsConfig {
  actionSec: number;
  navigateSec: number;
  shutdownSec: number;
  waitNavMs: number;
}

export interface BackendConfig {
  server: BackendServerConfig;
  browser: BackendBrowserConfig;
  instanceDefaults: BackendInstanceDefaultsConfig;
  security: BackendSecurityConfig;
  profiles: BackendProfilesConfig;
  multiInstance: BackendMultiInstanceConfig;
  attach: BackendAttachConfig;
  timeouts: BackendTimeoutsConfig;
}

export interface BackendConfigState {
  config: BackendConfig;
  configPath: string;
  restartRequired: boolean;
  restartReasons: string[];
}

export const defaultBackendConfig: BackendConfig = {
  server: {
    port: "9867",
    bind: "127.0.0.1",
    token: "",
    stateDir: "",
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
    tabEvictionPolicy: "reject",
  },
  security: {
    allowEvaluate: false,
    allowMacro: false,
    allowScreencast: false,
    allowDownload: false,
    allowUpload: false,
  },
  profiles: {
    baseDir: "",
    defaultProfile: "default",
  },
  multiInstance: {
    strategy: "simple",
    allocationPolicy: "fcfs",
    instancePortStart: 9868,
    instancePortEnd: 9968,
  },
  attach: {
    enabled: false,
    allowHosts: ["127.0.0.1", "localhost", "::1"],
    allowSchemes: ["ws", "wss"],
  },
  timeouts: {
    actionSec: 30,
    navigateSec: 60,
    shutdownSec: 10,
    waitNavMs: 1000,
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
    },
    profiles: {
      ...defaultBackendConfig.profiles,
      ...(input?.profiles ?? {}),
    },
    multiInstance: {
      ...defaultBackendConfig.multiInstance,
      ...(input?.multiInstance ?? {}),
    },
    attach: {
      ...defaultBackendConfig.attach,
      ...(input?.attach ?? {}),
      allowHosts:
        input?.attach?.allowHosts ?? defaultBackendConfig.attach.allowHosts,
      allowSchemes:
        input?.attach?.allowSchemes ?? defaultBackendConfig.attach.allowSchemes,
    },
    timeouts: {
      ...defaultBackendConfig.timeouts,
      ...(input?.timeouts ?? {}),
    },
  };
}

export function normalizeBackendConfigState(
  input: Partial<BackendConfigState>,
): BackendConfigState {
  return {
    config: normalizeBackendConfig(input.config),
    configPath: input.configPath ?? "",
    restartRequired: input.restartRequired ?? false,
    restartReasons: input.restartReasons ?? [],
  };
}

export function normalizeDashboardServerInfo(
  input: DashboardServerInfo,
): DashboardServerInfo {
  return {
    ...input,
    restartRequired: input.restartRequired ?? false,
    restartReasons: input.restartReasons ?? [],
  };
}

export type {
  Settings as LocalDashboardSettings,
  ScreencastSettings as LocalScreencastSettings,
  BrowserSettings as LocalBrowserSettings,
};
