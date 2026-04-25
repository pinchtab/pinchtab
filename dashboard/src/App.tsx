import { useEffect, useState } from "react";
import {
  BrowserRouter,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { ActivityPage, AgentsPage } from "./activities";
import { HandoffNotifications, NavBar } from "./components/molecules";
import { LoginPage, MonitoringPage, ProfilesPage, SettingsPage } from "./pages";
import * as api from "./services/api";
import {
  AUTH_REQUIRED_EVENT,
  AUTH_STATE_CHANGED_EVENT,
  INSECURE_DASHBOARD_TRANSPORT_WARNING,
  SERVER_UNREACHABLE_EVENT,
  isInsecureDashboardTransport,
} from "./services/auth";
import { acquireDashboardRealtime } from "./services/dashboardRealtime";
import { useAppStore } from "./stores/useAppStore";

type AuthMode = "probing" | "required" | "open" | "unreachable";
const AUTH_RETRY_DELAYS_MS = [1000, 2000, 4000, 8000, 15000] as const;

function AppContent() {
  const setInstances = useAppStore((state) => state.setInstances);
  const setProfiles = useAppStore((state) => state.setProfiles);
  const setAgents = useAppStore((state) => state.setAgents);
  const setServerInfo = useAppStore((state) => state.setServerInfo);
  const memoryMetricsEnabled = useAppStore(
    (state) => state.settings.monitoring?.memoryMetrics ?? false,
  );
  const location = useLocation();
  const navigate = useNavigate();
  const [authMode, setAuthMode] = useState<AuthMode>("probing");
  const [authProtected, setAuthProtected] = useState(false);
  const [authRetryCount, setAuthRetryCount] = useState(0);
  const dashboardAccessible = authMode === "open";
  const loginRequired = authMode === "required";
  const insecureDashboardTransport = isInsecureDashboardTransport();

  useEffect(() => {
    document.documentElement.setAttribute("data-site-mode", "agent");
  }, []);

  useEffect(() => {
    if (
      authMode === "open" ||
      authMode === "required" ||
      authMode === "unreachable"
    ) {
      return;
    }

    let cancelled = false;
    api
      .probeBackendAuth()
      .then((result) => {
        if (cancelled) {
          return;
        }
        setAuthProtected(result.mode !== "open");
        setAuthMode(result.mode === "required" ? "required" : "open");
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
  }, [authMode, setServerInfo]);

  useEffect(() => {
    if (
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
  }, [authMode, authRetryCount]);

  useEffect(() => {
    const handleAuthRequired = () => {
      setAuthProtected(true);
      setAuthMode("required");
      setAuthRetryCount(0);
      navigate("/login", {
        replace: true,
        state: { from: location.pathname },
      });
    };
    const handleAuthStateChanged = () => {
      setAuthMode("probing");
      setAuthRetryCount(0);
    };
    const handleServerUnreachable = () => {
      setAuthMode("unreachable");
    };

    window.addEventListener(AUTH_REQUIRED_EVENT, handleAuthRequired);
    window.addEventListener(AUTH_STATE_CHANGED_EVENT, handleAuthStateChanged);
    window.addEventListener(SERVER_UNREACHABLE_EVENT, handleServerUnreachable);
    return () => {
      window.removeEventListener(AUTH_REQUIRED_EVENT, handleAuthRequired);
      window.removeEventListener(
        AUTH_STATE_CHANGED_EVENT,
        handleAuthStateChanged,
      );
      window.removeEventListener(
        SERVER_UNREACHABLE_EVENT,
        handleServerUnreachable,
      );
    };
  }, [location.pathname, navigate]);

  useEffect(() => {
    if (loginRequired && location.pathname !== "/login") {
      navigate("/login", {
        replace: true,
        state: { from: location.pathname },
      });
    }
  }, [location.pathname, loginRequired, navigate]);

  useEffect(() => {
    if (dashboardAccessible && location.pathname === "/login") {
      navigate("/dashboard/monitoring", { replace: true });
    }
  }, [dashboardAccessible, location.pathname, navigate]);

  useEffect(() => {
    if (!dashboardAccessible) {
      return;
    }
    const load = async () => {
      try {
        const [instances, profiles, agents, health] = await Promise.all([
          api.fetchInstances(),
          api.fetchProfiles(),
          api.fetchAgents(),
          api.fetchHealth(),
        ]);
        setInstances(instances);
        setProfiles(profiles);
        setAgents(agents);
        setServerInfo(health);
      } catch (e) {
        console.error("Failed to load initial data", e);
      }
    };
    void load();
  }, [
    dashboardAccessible,
    setAgents,
    setInstances,
    setProfiles,
    setServerInfo,
  ]);

  useEffect(() => {
    if (!dashboardAccessible) {
      return;
    }
    return acquireDashboardRealtime(memoryMetricsEnabled);
  }, [dashboardAccessible, memoryMetricsEnabled]);

  if (authMode === "probing") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg-app px-4">
        <div className="rounded-sm border border-border-subtle bg-black/10 px-4 py-3 text-sm text-text-muted">
          Checking server authentication...
        </div>
      </div>
    );
  }

  if (authMode === "unreachable") {
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
      <NavBar showLogout={authProtected} />
      {insecureDashboardTransport && (
        <div className="border-b border-warning/25 bg-warning/10 px-4 py-2 text-sm text-warning">
          {INSECURE_DASHBOARD_TRANSPORT_WARNING}
        </div>
      )}
      <HandoffNotifications />
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
          <Route path="/dashboard/agents" element={<AgentsPage />} />
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
