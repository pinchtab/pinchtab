import type { TFunction } from "i18next";
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
  | "timeouts"
  | "autosolver"
  | "observability";

export const sections = [
  {
    id: "dashboard",
    labelKey: "settings.sections.dashboard.label",
    descriptionKey: "settings.sections.dashboard.description",
  },
  {
    id: "defaults",
    labelKey: "settings.sections.defaults.label",
    descriptionKey: "settings.sections.defaults.description",
  },
  {
    id: "orchestration",
    labelKey: "settings.sections.orchestration.label",
    descriptionKey: "settings.sections.orchestration.description",
  },
  {
    id: "security",
    labelKey: "settings.sections.security.label",
    descriptionKey: "settings.sections.security.description",
  },
  {
    id: "security-idpi",
    labelKey: "settings.sections.security-idpi.label",
    descriptionKey: "settings.sections.security-idpi.description",
  },
  {
    id: "profiles",
    labelKey: "settings.sections.profiles.label",
    descriptionKey: "settings.sections.profiles.description",
  },
  {
    id: "network",
    labelKey: "settings.sections.network.label",
    descriptionKey: "settings.sections.network.description",
  },
  {
    id: "browser",
    labelKey: "settings.sections.browser.label",
    descriptionKey: "settings.sections.browser.description",
  },
  {
    id: "timeouts",
    labelKey: "settings.sections.timeouts.label",
    descriptionKey: "settings.sections.timeouts.description",
  },
  {
    id: "autosolver",
    labelKey: "settings.sections.autosolver.label",
    descriptionKey: "settings.sections.autosolver.description",
  },
  {
    id: "observability",
    labelKey: "settings.sections.observability.label",
    descriptionKey: "settings.sections.observability.description",
  },
] as const satisfies ReadonlyArray<{
  id: SectionId;
  labelKey: string;
  descriptionKey: string;
}>;

export const fieldClass =
  "w-full rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

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
  ["allowEvaluate", "settings.endpoints.allowEvaluate.label"],
  ["allowMacro", "settings.endpoints.allowMacro.label"],
  ["allowScreencast", "settings.endpoints.allowScreencast.label"],
  ["allowDownload", "settings.endpoints.allowDownload.label"],
  ["allowCookies", "settings.endpoints.allowCookies.label"],
  ["allowUpload", "settings.endpoints.allowUpload.label"],
  [
    "allowNetworkIntercept",
    "settings.endpoints.allowNetworkIntercept.label",
    "settings.endpoints.allowNetworkIntercept.description",
  ],
  [
    "allowFileScheme",
    "settings.endpoints.allowFileScheme.label",
    "settings.endpoints.allowFileScheme.description",
  ],
] as const satisfies ReadonlyArray<
  | readonly [SecurityEndpointKey, string]
  | readonly [SecurityEndpointKey, string, string]
>;

export const idpiToggleRows = [
  [
    "enabled",
    "settings.idpi.enabled.label",
    "settings.idpi.enabled.description",
  ],
  [
    "strictMode",
    "settings.idpi.strictMode.label",
    "settings.idpi.strictMode.description",
  ],
  [
    "scanContent",
    "settings.idpi.scanContent.label",
    "settings.idpi.scanContent.description",
  ],
  [
    "wrapContent",
    "settings.idpi.wrapContent.label",
    "settings.idpi.wrapContent.description",
  ],
] as const satisfies ReadonlyArray<readonly [IDPIToggleKey, string, string]>;

export const instanceDefaultsBooleanRows = [
  ["blockImages", "settings.instanceDefaults.blockImages"],
  ["blockMedia", "settings.instanceDefaults.blockMedia"],
  ["blockAds", "settings.instanceDefaults.blockAds"],
  ["noAnimations", "settings.instanceDefaults.noAnimations"],
  ["noRestore", "settings.instanceDefaults.noRestore"],
] as const;

export const timeoutRows = [
  [
    "actionSec",
    "settings.timeouts.actionSec.label",
    "settings.timeouts.actionSec.description",
  ],
  [
    "navigateSec",
    "settings.timeouts.navigateSec.label",
    "settings.timeouts.navigateSec.description",
  ],
  [
    "shutdownSec",
    "settings.timeouts.shutdownSec.label",
    "settings.timeouts.shutdownSec.description",
  ],
  [
    "waitNavMs",
    "settings.timeouts.waitNavMs.label",
    "settings.timeouts.waitNavMs.description",
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

export function backendSaveNotice(
  state: BackendConfigState | null,
  t: TFunction,
): string {
  if (state?.restartRequired) {
    return t("settings.saveNotice.restartAdvised");
  }
  return t("settings.saveNotice.saved");
}
