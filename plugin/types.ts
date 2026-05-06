import type { AnyAgentTool, OpenClawPluginApi, OpenClawPluginToolContext } from "openclaw/plugin-sdk/plugin-entry";

export interface PluginConfig {
  baseUrl?: string;
  token?: string;
  timeoutMs?: number;
  /** @deprecated Use timeoutMs instead */
  timeout?: number;
  allowEvaluate?: boolean;
  allowedDomains?: string[];
  allowDownloads?: boolean;
  allowUploads?: boolean;
  allowNetworkIntercept?: boolean;
  defaultSnapshotFormat?: string;
  defaultSnapshotFilter?: string;
  screenshotFormat?: string;
  screenshotQuality?: number;
  persistSessionTabs?: boolean;
  registerBrowserTool?: boolean;
  defaultProfile?: string;
  profiles?: Record<string, { instanceId?: string; attach?: boolean }>;
}

export interface PluginRuntimeContext {
  agentId?: string;
  sessionId?: string;
  sessionKey?: string;
}

export interface AgentSessionState extends PluginRuntimeContext {
  key: string;
  lastTabId?: string;
  updatedAt?: number;
}

export type PluginApi = OpenClawPluginApi;
export type PluginTool = AnyAgentTool;
export type PluginToolContext = OpenClawPluginToolContext;

export interface ToolResult {
  content: Array<
    | { type: "text"; text: string }
    | { type: "image"; data: string; mimeType: string }
    | { type: "resource"; resource: { uri: string; mimeType: string; blob: string } }
  >;
}
