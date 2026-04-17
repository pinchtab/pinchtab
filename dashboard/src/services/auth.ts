const AUTH_REQUIRED_EVENT = "pinchtab-auth-required";
const AUTH_STATE_CHANGED_EVENT = "pinchtab-auth-state-changed";
const SERVER_UNREACHABLE_EVENT = "pinchtab-server-unreachable";
export const INSECURE_DASHBOARD_TRANSPORT_WARNING =
  "Dashboard session is running over insecure HTTP; use HTTPS or localhost for stronger session protection.";

export function dispatchAuthRequired(reason: string): void {
  window.dispatchEvent(
    new CustomEvent(AUTH_REQUIRED_EVENT, {
      detail: { reason },
    }),
  );
}

export function dispatchAuthStateChanged(): void {
  window.dispatchEvent(new Event(AUTH_STATE_CHANGED_EVENT));
}

export function dispatchServerUnreachable(): void {
  window.dispatchEvent(new Event(SERVER_UNREACHABLE_EVENT));
}

export function sameOriginUrl(url: string): string {
  const absolute = new URL(url, window.location.origin);
  return absolute.pathname + absolute.search;
}

function isLoopbackHostname(hostname: string): boolean {
  const host = hostname.trim().toLowerCase();
  if (
    host === "localhost" ||
    host === "::1" ||
    host === "[::1]" ||
    host.endsWith(".localhost")
  ) {
    return true;
  }
  return /^127(?:\.\d{1,3}){3}$/.test(host);
}

export function isInsecureDashboardTransport(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  const { protocol, hostname } = window.location;
  return protocol === "http:" && !isLoopbackHostname(hostname);
}

export {
  AUTH_REQUIRED_EVENT,
  AUTH_STATE_CHANGED_EVENT,
  SERVER_UNREACHABLE_EVENT,
};
