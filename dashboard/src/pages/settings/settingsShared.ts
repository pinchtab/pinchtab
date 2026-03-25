import type {
  BackendConfig,
  BackendConfigState,
  BackendIDPIConfig,
  BackendSecurityConfig,
} from "../../types";

export type SectionId =
  | "dashboard"
  | "defaults"
  | "orchestration"
  | "security"
  | "security-idpi"
  | "profiles"
  | "network"
  | "browser"
  | "timeouts";

export const sections: Array<{
  id: SectionId;
  label: string;
  description: string;
}> = [
  {
    id: "dashboard",
    label: "Dashboard",
    description: "Local monitoring and screencast preferences.",
  },
  {
    id: "defaults",
    label: "Instance Defaults",
    description: "How new managed browser instances launch.",
  },
  {
    id: "orchestration",
    label: "Orchestration",
    description: "Routing strategy, port range, and allocation policy.",
  },
  {
    id: "security",
    label: "Security",
    description: "Sensitive endpoint gates and access controls.",
  },
  {
    id: "security-idpi",
    label: "Security IDPI",
    description: "Indirect prompt injection website and content defenses.",
  },
  {
    id: "profiles",
    label: "Profiles",
    description: "Shared profile storage and default profile behavior.",
  },
  {
    id: "network",
    label: "Network & Attach",
    description: "Server binding, auth, and attach policy.",
  },
  {
    id: "browser",
    label: "Browser Runtime",
    description: "Chrome binary, version, flags, and extensions.",
  },
  {
    id: "timeouts",
    label: "Timeouts",
    description: "Action, navigation, shutdown, and wait timing.",
  },
];

export const fieldClass =
  "w-full rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

export const selectClass =
  "rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

export type UpdateBackendSection = <K extends keyof BackendConfig>(
  section: K,
  patch: Partial<BackendConfig[K]>,
) => void;

export type SecurityEndpointKey = Exclude<
  keyof BackendSecurityConfig,
  "attach" | "idpi"
>;
export type IDPIToggleKey = Exclude<
  keyof BackendIDPIConfig,
  "allowedDomains" | "customPatterns"
>;

export const securityEndpointRows = [
  ["allowEvaluate", "Allow evaluate"],
  ["allowMacro", "Allow macro"],
  ["allowScreencast", "Allow screencast"],
  ["allowDownload", "Allow download"],
  ["allowUpload", "Allow upload"],
] as const satisfies ReadonlyArray<readonly [SecurityEndpointKey, string]>;

export const idpiToggleRows = [
  ["enabled", "Enable IDPI", "Turn on indirect prompt injection defenses."],
  [
    "strictMode",
    "Strict mode",
    "Block disallowed domains and suspicious content instead of only warning.",
  ],
  [
    "scanContent",
    "Scan content",
    "Inspect extracted text and snapshots for prompt-injection patterns.",
  ],
  [
    "wrapContent",
    "Wrap content",
    "Mark returned page text as untrusted content for downstream consumers.",
  ],
] as const satisfies ReadonlyArray<readonly [IDPIToggleKey, string, string]>;

export const instanceDefaultsBooleanRows = [
  ["blockImages", "Block images"],
  ["blockMedia", "Block media"],
  ["blockAds", "Block ads"],
  ["noAnimations", "Disable CSS animations"],
  ["noRestore", "Skip session restore"],
] as const;

export const timeoutRows = [
  ["actionSec", "Action timeout", "Maximum time for action requests."],
  ["navigateSec", "Navigate timeout", "Maximum time for navigation requests."],
  [
    "shutdownSec",
    "Shutdown timeout",
    "Grace period before force-closing a child process.",
  ],
  [
    "waitNavMs",
    "Wait-after-navigation delay",
    "Post-navigation stabilization delay in milliseconds.",
  ],
] as const;

export function csvToList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function listToCsv(value: string[]): string {
  return value.join(", ");
}

export function backendSaveNotice(state: BackendConfigState | null): string {
  if (state?.restartRequired) {
    return "Backend config saved. Dynamic changes were applied where possible. Restart advised for server-level changes.";
  }
  return "Backend config saved. Dynamic changes were applied where possible.";
}
