import { useEffect, useState } from "react";
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { ActivityPage } from "./activities";
import { useAppStore } from "./stores/useAppStore";
import { NavBar } from "./components/molecules";
import { LoginPage, MonitoringPage, ProfilesPage, SettingsPage } from "./pages";
import * as api from "./services/api";
import {
  AUTH_REQUIRED_EVENT,
  clearStoredAuthToken,
  getStoredAuthToken,
  setStoredAuthToken,
} from "./services/auth";

type AuthMode = "probing" | "required" | "open" | "unreachable";
const AUTH_RETRY_DELAYS_MS = [1000, 2000, 4000, 8000, 15000] as const;

function AppContent() {
  const {
    setInstances,
    setProfiles,
    setAgents,
    setServerInfo,
    applyMonitoringSnapshot,
    settings,
  } = useAppStore();
  const location = useLocation();
  const navigate = useNavigate();
  const memoryMetricsEnabled = settings.monitoring?.memoryMetrics ?? false;
  // Auto-login from ?token= query parameter (e.g. from wizard URL)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get("token");
    if (urlToken) {
      setStoredAuthToken(urlToken);
      const clean = new URL(window.location.href);
      clean.searchParams.delete("token");
      window.history.replaceState({}, "", clean.pathname + clean.hash);
      window.location.reload();
    }
  }, []);

  const authToken = getStoredAuthToken();
  const hasStoredToken = authToken !== "";
  const [authMode, setAuthMode] = useState<AuthMode>(
    hasStoredToken ? "required" : "probing",
  );
  const [authRetryCount, setAuthRetryCount] = useState(0);
  const dashboardAccessible = hasStoredToken || authMode === "open";
  const loginRequired = !hasStoredToken && authMode === "required";

  useEffect(() => {
    document.documentElement.setAttribute("data-site-mode", "agent");
  }, []);

  useEffect(() => {
    if (hasStoredToken) {
      setAuthMode("required");
      setAuthRetryCount(0);
      return;
    }

    if (authMode === "open" || authMode === "unreachable") {
      return;
    }

    let cancelled = false;
    api
      .probeBackendAuth()
      .then((result) => {
        if (cancelled) {
          return;
        }
        setAuthMode(result.requiresAuth ? "required" : "open");
        setAuthRetryCount(0);
        if (result.health) {
          setServerInfo(result.health);
        }
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        console.error("Failed to probe backend auth", error);
        setAuthMode("unreachable");
      });

    return () => {
      cancelled = true;
    };
  }, [authMode, hasStoredToken, setServerInfo]);

  useEffect(() => {
    if (
      hasStoredToken ||
      authMode !== "unreachable" ||
      authRetryCount >= AUTH_RETRY_DELAYS_MS.length
    ) {
      return;
    }

    const timer = window.setTimeout(() => {
      setAuthRetryCount((count) => count + 1);
      setAuthMode("probing");
    }, AUTH_RETRY_DELAYS_MS[authRetryCount]);

    return () => {
      window.clearTimeout(timer);
    };
  }, [authMode, authRetryCount, hasStoredToken]);

  useEffect(() => {
    const handleAuthRequired = () => {
      clearStoredAuthToken();
      setAuthMode("required");
      setAuthRetryCount(0);
      navigate("/login", {
        replace: true,
        state: { from: location.pathname },
      });
    };
    window.addEventListener(AUTH_REQUIRED_EVENT, handleAuthRequired);
    return () =>
      window.removeEventListener(AUTH_REQUIRED_EVENT, handleAuthRequired);
  }, [location.pathname, navigate]);

  useEffect(() => {
    if (loginRequired && location.pathname !== "/login") {
      navigate("/login", {
        replace: true,
        state: { from: location.pathname },
      });
    }
  }, [location.pathname, loginRequired, navigate]);

  // Initial load
  useEffect(() => {
    if (!dashboardAccessible) {
      return;
    }
    const load = async () => {
      try {
        const [instances, profiles, health] = await Promise.all([
          api.fetchInstances(),
          api.fetchProfiles(),
          api.fetchHealth(),
        ]);
        setInstances(instances);
        setProfiles(profiles);
        setServerInfo(health);
      } catch (e) {
        console.error("Failed to load initial data", e);
      }
    };
    load();
  }, [dashboardAccessible, setInstances, setProfiles, setServerInfo]);

  // Subscribe to SSE events
  useEffect(() => {
    if (!dashboardAccessible) {
      return;
    }
    const unsubscribe = api.subscribeToEvents(
      {
        onInit: (agents) => {
          setAgents(agents);
        },
        onSystem: (event) => {
          console.log("System event:", event);
        },
        onAgent: (event) => {
          console.log("Agent event:", event);
        },
        onMonitoring: (snapshot) => {
          applyMonitoringSnapshot(snapshot, memoryMetricsEnabled);
        },
      },
      {
        includeMemory: memoryMetricsEnabled,
      },
    );

    return unsubscribe;
  }, [
    dashboardAccessible,
    applyMonitoringSnapshot,
    memoryMetricsEnabled,
    setAgents,
    setInstances,
    setProfiles,
  ]);

  if (!hasStoredToken && authMode === "probing") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg-app px-4">
        <div className="rounded-sm border border-border-subtle bg-black/10 px-4 py-3 text-sm text-text-muted">
          Checking server authentication...
        </div>
      </div>
    );
  }

  if (!hasStoredToken && authMode === "unreachable") {
    const nextRetryDelay =
      authRetryCount < AUTH_RETRY_DELAYS_MS.length
        ? AUTH_RETRY_DELAYS_MS[authRetryCount]
        : null;

    return (
      <div className="flex min-h-screen items-center justify-center bg-bg-app px-4">
        <div className="max-w-md space-y-3 rounded-sm border border-border-subtle bg-black/10 px-4 py-3 text-sm text-text-muted">
          <div>
            PinchTab is restarting or unreachable.
            {nextRetryDelay !== null
              ? ` Retrying in ${Math.ceil(nextRetryDelay / 1000)}s...`
              : " Automatic retries stopped."}
          </div>
          {nextRetryDelay === null && (
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="rounded-sm border border-border-subtle px-3 py-2 text-sm text-text-primary transition-all duration-150 hover:border-primary/30 hover:bg-bg-elevated"
                onClick={() => {
                  setAuthRetryCount(0);
                  setAuthMode("probing");
                }}
              >
                Retry now
              </button>
              <button
                type="button"
                className="rounded-sm border border-border-subtle px-3 py-2 text-sm text-text-primary transition-all duration-150 hover:border-primary/30 hover:bg-bg-elevated"
                onClick={() => window.location.reload()}
              >
                Refresh
              </button>
            </div>
          )}
        </div>
      </div>
    );
  }

  if (loginRequired) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    );
  }

  return (
    <div className="dashboard-shell flex h-screen flex-col bg-bg-app">
      <NavBar />
      <main className="dashboard-grid flex-1 overflow-hidden">
        <Routes>
          <Route
            path="/"
            element={<Navigate to="/dashboard/monitoring" replace />}
          />
          <Route
            path="/login"
            element={<Navigate to="/dashboard/monitoring" replace />}
          />
          <Route
            path="/dashboard"
            element={<Navigate to="/dashboard/monitoring" replace />}
          />
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
          <Route path="/dashboard/activity" element={<ActivityPage />} />
          <Route path="/dashboard/profiles" element={<ProfilesPage />} />
          <Route
            path="/dashboard/agents"
            element={<Navigate to="/dashboard/activity" replace />}
          />
          <Route path="/dashboard/settings" element={<SettingsPage />} />
          <Route
            path="*"
            element={<Navigate to="/dashboard/monitoring" replace />}
          />
        </Routes>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  );
}
