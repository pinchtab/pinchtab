import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import * as api from "../../services/api";
import { dispatchAuthRequired } from "../../services/auth";
import { useAppStore } from "../../stores/useAppStore";
import "./NavBar.css";

interface Tab {
  id: string;
  path: string;
  label: string;
}

const tabs: Tab[] = [
  { id: "monitoring", path: "/dashboard/monitoring", label: "Monitoring" },
  { id: "agents", path: "/dashboard/agents", label: "Agents" },
  { id: "profiles", path: "/dashboard/profiles", label: "Profiles" },
  { id: "settings", path: "/dashboard/settings", label: "Settings" },
];

interface NavBarProps {
  onRefresh?: () => void;
  showLogout?: boolean;
}

export default function NavBar({ onRefresh, showLogout = false }: NavBarProps) {
  const { serverInfo } = useAppStore();
  const [refreshing, setRefreshing] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const tabsRef = useRef<HTMLElement>(null);
  const location = useLocation();
  const navigate = useNavigate();

  // Close mobile menu on route change
  useEffect(() => {
    setMobileMenuOpen(false);
  }, [location]);

  const handleRefresh = useCallback(() => {
    if (!onRefresh || refreshing) return;
    setRefreshing(true);
    onRefresh();
    setTimeout(() => setRefreshing(false), 800);
  }, [onRefresh, refreshing]);

  const handleLogout = useCallback(async () => {
    try {
      await api.logout();
      setMobileMenuOpen(false);
      dispatchAuthRequired("logout");
    } catch (error) {
      console.error("Logout failed", error);
    }
  }, []);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!e.metaKey && !e.ctrlKey) return;
      const num = parseInt(e.key);
      if (num >= 1 && num <= tabs.length) {
        e.preventDefault();
        navigate(tabs[num - 1].path);
        return;
      }
      if (e.key === "r" && onRefresh) {
        e.preventDefault();
        handleRefresh();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [navigate, onRefresh, handleRefresh]);

  return (
    <header className="sticky top-0 z-50 border-b border-border-subtle bg-bg-app/95 backdrop-blur">
      <div className="flex h-[60px] items-center gap-0 px-4 sm:px-5">
        <span className="min-w-32 text-sm font-semibold tracking-[0.2em] text-text-primary uppercase">
          PinchTab
        </span>

        {/* Desktop nav */}
        <nav className="ml-6 hidden items-center gap-0.5 sm:flex" ref={tabsRef}>
          {tabs.map((tab, i) => (
            <NavLink
              key={tab.id}
              to={tab.path}
              className={({ isActive }) =>
                `navbar-tab relative cursor-pointer rounded-sm border border-transparent bg-transparent px-3.5 py-2.5 text-sm font-medium leading-none whitespace-nowrap transition-all duration-150 hover:border-border-subtle hover:bg-bg-hover/70 hover:text-text-primary focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 ${
                  isActive
                    ? "active border-primary/20 bg-primary/10 text-text-primary"
                    : "text-text-secondary"
                }`
              }
              title={`${tab.label} (⌘${i + 1})`}
            >
              {tab.label}
            </NavLink>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-1.5">
          {serverInfo && (
            <div
              className={`mr-2 flex items-center gap-1.5 rounded-full px-2.5 py-1 ${
                serverInfo.restartRequired
                  ? "border border-warning/25 bg-warning/10"
                  : "border border-success/20 bg-success/10"
              }`}
              title={
                serverInfo.restartRequired
                  ? serverInfo.restartReasons?.join(", ") || "Restart required"
                  : "Server running"
              }
            >
              <div
                className={`h-1.5 w-1.5 rounded-full ${
                  serverInfo.restartRequired
                    ? "bg-warning"
                    : "bg-success animate-pulse"
                }`}
              />
              <span
                className={`text-[10px] font-bold uppercase tracking-wider ${
                  serverInfo.restartRequired ? "text-warning" : "text-success"
                }`}
              >
                {serverInfo.restartRequired ? "Restart Required" : "Running"}
              </span>
            </div>
          )}
          {showLogout && (
            <button
              type="button"
              className="mr-2 rounded-sm border border-transparent px-3 py-1.5 text-xs font-medium uppercase tracking-[0.08em] text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-hover hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
              onClick={() => {
                void handleLogout();
              }}
            >
              Logout
            </button>
          )}
          {onRefresh && (
            <button
              className={`navbar-icon-btn flex h-8 w-8 cursor-pointer items-center justify-center rounded-sm border border-transparent bg-transparent text-base text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-hover hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 ${
                refreshing ? "spinning" : ""
              }`}
              onClick={handleRefresh}
              title="Refresh (⌘R)"
            >
              ↻
            </button>
          )}
          {/* Mobile menu button */}
          <button
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-sm border border-transparent bg-transparent text-lg text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-hover hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 sm:hidden"
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            aria-label="Toggle menu"
          >
            {mobileMenuOpen ? "✕" : "☰"}
          </button>
        </div>
      </div>

      {/* Mobile menu dropdown */}
      {mobileMenuOpen && (
        <nav className="flex flex-col border-t border-border-subtle bg-bg-surface sm:hidden">
          {tabs.map((tab) => (
            <NavLink
              key={tab.id}
              to={tab.path}
              className={({ isActive }) =>
                `px-4 py-3 text-sm font-medium transition-colors duration-150 ${
                  isActive
                    ? "bg-primary/10 text-text-primary"
                    : "text-text-secondary hover:bg-bg-elevated hover:text-text-primary"
                }`
              }
            >
              {tab.label}
            </NavLink>
          ))}
          {showLogout && (
            <button
              type="button"
              className="border-t border-border-subtle px-4 py-3 text-left text-sm font-medium text-text-secondary transition-colors duration-150 hover:bg-bg-elevated hover:text-text-primary"
              onClick={() => {
                void handleLogout();
              }}
            >
              Logout
            </button>
          )}
        </nav>
      )}
    </header>
  );
}
