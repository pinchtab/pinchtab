const AUTH_REQUIRED_EVENT = "pinchtab-auth-required";
const AUTH_STATE_CHANGED_EVENT = "pinchtab-auth-state-changed";
const CREDENTIAL_USERNAME_PREFIX = "pinchtab";

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

export function sameOriginUrl(url: string): string {
  const absolute = new URL(url, window.location.origin);
  return absolute.pathname + absolute.search;
}

export function credentialUsername(): string {
  if (typeof window === "undefined") {
    return CREDENTIAL_USERNAME_PREFIX;
  }
  return `${CREDENTIAL_USERNAME_PREFIX}@${window.location.host}`;
}

type PasswordCredentialConstructor = new (data: {
  id: string;
  password: string;
  name?: string;
}) => Credential;

export async function storeTokenCredential(token: string): Promise<void> {
  const trimmed = token.trim();
  if (
    trimmed === "" ||
    typeof window === "undefined" ||
    navigator.credentials?.store === undefined
  ) {
    return;
  }

  const PasswordCredentialImpl = (
    window as Window & { PasswordCredential?: PasswordCredentialConstructor }
  ).PasswordCredential;
  if (!PasswordCredentialImpl) {
    return;
  }

  try {
    const credential = new PasswordCredentialImpl({
      id: credentialUsername(),
      password: trimmed,
      name: `PinchTab ${window.location.host}`,
    });
    await navigator.credentials.store(credential);
  } catch {
    // Ignore password-manager failures and continue with the session flow.
  }
}

export { AUTH_REQUIRED_EVENT, AUTH_STATE_CHANGED_EVENT };
