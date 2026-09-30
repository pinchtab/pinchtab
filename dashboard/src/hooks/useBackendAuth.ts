import { useCallback, useEffect, useReducer } from "react";
import * as api from "../services/api";
import {
  AUTH_REQUIRED_EVENT,
  AUTH_STATE_CHANGED_EVENT,
  SERVER_UNREACHABLE_EVENT,
} from "../services/auth";
import { useAppStore } from "../stores/useAppStore";

export type AuthMode = "probing" | "required" | "open" | "unreachable";

export const AUTH_RETRY_DELAYS_MS = [1000, 2000, 4000, 8000, 15000] as const;

export interface BackendAuthState {
  mode: AuthMode;
  authProtected: boolean;
  retryCount: number;
}

export type BackendAuthEvent =
  | { type: "probeSucceeded"; mode: "open" | "required" }
  | { type: "probeFailed" }
  | { type: "retryElapsed" }
  | { type: "retryNow" }
  | { type: "authRequired" }
  | { type: "authStateChanged" }
  | { type: "serverUnreachable" };

export const initialBackendAuthState: BackendAuthState = {
  mode: "probing",
  authProtected: false,
  retryCount: 0,
};

export function backendAuthReducer(
  state: BackendAuthState,
  event: BackendAuthEvent,
): BackendAuthState {
  switch (event.type) {
    case "probeSucceeded":
      return {
        mode: event.mode,
        authProtected: event.mode !== "open",
        retryCount: 0,
      };
    case "probeFailed":
    case "serverUnreachable":
      return { ...state, mode: "unreachable" };
    case "retryElapsed":
      return { ...state, mode: "probing", retryCount: state.retryCount + 1 };
    case "retryNow":
    case "authStateChanged":
      return { ...state, mode: "probing", retryCount: 0 };
    case "authRequired":
      return { mode: "required", authProtected: true, retryCount: 0 };
  }
}

export function nextRetryDelayMs(retryCount: number): number | null {
  return retryCount < AUTH_RETRY_DELAYS_MS.length
    ? AUTH_RETRY_DELAYS_MS[retryCount]
    : null;
}

export function useBackendAuth() {
  const setServerInfo = useAppStore((state) => state.setServerInfo);
  const [state, dispatch] = useReducer(
    backendAuthReducer,
    initialBackendAuthState,
  );

  useEffect(() => {
    if (state.mode !== "probing") {
      return;
    }
    let cancelled = false;
    api
      .probeBackendAuth()
      .then((result) => {
        if (cancelled) {
          return;
        }
        dispatch({
          type: "probeSucceeded",
          mode: result.mode === "required" ? "required" : "open",
        });
        if (result.health) {
          setServerInfo(result.health);
        }
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        console.error("Failed to probe backend auth", error);
        dispatch({ type: "probeFailed" });
      });
    return () => {
      cancelled = true;
    };
  }, [state.mode, setServerInfo]);

  const retryDelay =
    state.mode === "unreachable" ? nextRetryDelayMs(state.retryCount) : null;

  useEffect(() => {
    if (retryDelay === null) {
      return;
    }
    const timer = window.setTimeout(
      () => dispatch({ type: "retryElapsed" }),
      retryDelay,
    );
    return () => window.clearTimeout(timer);
  }, [retryDelay, state.retryCount]);

  useEffect(() => {
    const listeners: [string, BackendAuthEvent][] = [
      [AUTH_REQUIRED_EVENT, { type: "authRequired" }],
      [AUTH_STATE_CHANGED_EVENT, { type: "authStateChanged" }],
      [SERVER_UNREACHABLE_EVENT, { type: "serverUnreachable" }],
    ];
    const handlers = listeners.map(([name, event]) => {
      const handler = () => dispatch(event);
      window.addEventListener(name, handler);
      return [name, handler] as const;
    });
    return () => {
      for (const [name, handler] of handlers) {
        window.removeEventListener(name, handler);
      }
    };
  }, []);

  const retryNow = useCallback(() => dispatch({ type: "retryNow" }), []);

  return { ...state, retryDelay, retryNow };
}
