import { useEffect } from "react";
import {
  HashRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
} from "react-router-dom";
import { useAppStore } from "./stores/useAppStore";
import { NavBar } from "./components/molecules";
import { MonitoringPage, ProfilesPage, SettingsPage } from "./pages";
import * as api from "./services/api";

function AppContent() {
  const { setInstances, setProfiles, setAgents, setServerInfo } = useAppStore();
  const location = useLocation();

  useEffect(() => {
    document.documentElement.setAttribute("data-site-mode", "agent");
  }, []);

  // Log navigation for debugging
  useEffect(() => {
    console.log("📍 Navigation:", location.pathname);
  }, [location]);

  // Initial load
  useEffect(() => {
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
  }, [setInstances, setProfiles, setServerInfo]);

  // Subscribe to SSE events
  useEffect(() => {
    const unsubscribe = api.subscribeToEvents({
      onInit: (agents) => {
        setAgents(agents);
      },
      onSystem: async (event) => {
        console.log("System event:", event);
        if (event.type.startsWith("instance.")) {
          try {
            const instances = await api.fetchInstances();
            setInstances(instances);
            const profiles = await api.fetchProfiles();
            setProfiles(profiles);
          } catch (e) {
            console.error("Failed to refresh after event", e);
          }
        }
      },
      onAgent: (event) => {
        console.log("Agent event:", event);
      },
    });

    return unsubscribe;
  }, [setInstances, setProfiles, setAgents]);

  return (
    <div className="dashboard-shell flex h-screen flex-col bg-bg-app">
      <NavBar />
      <main className="dashboard-grid flex-1 overflow-hidden">
        <Routes>
          <Route path="/" element={<Navigate to="/monitoring" replace />} />
          <Route path="/monitoring" element={<MonitoringPage />} />
          <Route path="/profiles" element={<ProfilesPage />} />
          <Route
            path="/agents"
            element={<Navigate to="/monitoring" replace />}
          />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <HashRouter>
      <AppContent />
    </HashRouter>
  );
}
