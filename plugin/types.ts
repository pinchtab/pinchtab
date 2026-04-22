export interface PluginConfig {
  baseUrl?: string;
  token?: string;
  timeoutMs?: number;
  /** @deprecated Use timeoutMs instead */
  timeout?: number;
  autoStart?: boolean;
  binaryPath?: string;
  startupTimeoutMs?: number;
  allowEvaluate?: boolean;
  allowedDomains?: string[];
  allowDownloads?: boolean;
  allowUploads?: boolean;
  defaultSnapshotFormat?: string;
  defaultSnapshotFilter?: string;
  screenshotFormat?: string;
  screenshotQuality?: number;
  persistSessionTabs?: boolean;
  registerBrowserTool?: boolean;
  defaultProfile?: string;
  profiles?: Record<string, { instanceId?: string; attach?: boolean }>;
}

export interface PluginApi {
  config: { plugins?: { entries?: Record<string, { config?: PluginConfig }> } };
  registerTool: (tool: any, opts?: { optional?: boolean }) => void;
}

export interface ToolResult {
  content: Array<
    | { type: "text"; text: string }
    | { type: "image"; data: string; mimeType: string }
    | { type: "resource"; resource: { uri: string; mimeType: string; blob: string } }
  >;
}
