// withAuthedFetch runs `fn` with a `doFetch` helper that injects bearer auth and
// shares a single AbortController whose timeout bounds the WHOLE exchange —
// including reading the response body — and is always cleared afterwards. The
// SDK's request and health-probe paths previously re-implemented this same
// auth-header + timeout boilerplate.
export async function withAuthedFetch<T>(
  token: string | null,
  timeoutMs: number,
  fn: (doFetch: (url: string, init?: RequestInit) => Promise<Response>) => Promise<T>
): Promise<T> {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  const doFetch = (url: string, init: RequestInit = {}): Promise<Response> => {
    const headers: Record<string, string> = {
      ...(init.headers as Record<string, string> | undefined),
    };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    return fetch(url, { ...init, headers, signal: controller.signal as AbortSignal });
  };

  try {
    return await fn(doFetch);
  } finally {
    clearTimeout(timeoutId);
  }
}
