import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { useAppStore } from "../../stores/useAppStore";
import "./NavBar.css";

interface Tab {
  id: string;
  path: string;
  label: string;
}

const tabs: Tab[] = [
  { id: "monitoring", path: "/monitoring", label: "Monitoring" },
  { id: "agents", path: "/agents", label: "Agents" },
  { id: "profiles", path: "/profiles", label: "Profiles" },
  { id: "settings", path: "/settings", label: "Settings" },
];

interface NavBarProps {
  onRefresh?: () => void;
}

export default function NavBar({ onRefresh }: NavBarProps) {
  const { serverInfo } = useAppStore();
  const [refreshing, setRefreshing] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const tabsRef = useRef<HTMLElement>(null);
  const location = useLocation();

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

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!e.metaKey && !e.ctrlKey) return;
      const num = parseInt(e.key);
      if (num >= 1 && num <= tabs.length) {
        e.preventDefault();
        window.location.hash = tabs[num - 1].path;
        return;
      }
      if (e.key === "r" && onRefresh) {
        e.preventDefault();
        handleRefresh();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onRefresh, handleRefresh]);

  return (
    <header className="sticky top-0 z-50 border-b border-border-subtle bg-bg-app">
      <div className="flex h-[52px] items-center gap-0 px-4">
        <span className="min-w-32 text-sm font-semibold tracking-wide text-text-primary">
          PinchTab
        </span>

        {/* Desktop nav */}
        <nav className="ml-6 hidden items-center gap-0.5 sm:flex" ref={tabsRef}>
          {tabs.map((tab, i) => (
            <NavLink
              key={tab.id}
              to={tab.path}
              className={({ isActive }) =>
                `navbar-tab relative cursor-pointer border-none bg-transparent px-3.5 py-3.5 text-sm font-medium leading-none whitespace-nowrap transition-colors duration-150 hover:text-text-primary focus-visible:rounded focus-visible:shadow-[0_0_0_2px_var(--primary)/25] focus-visible:outline-none ${
                  isActive ? "active text-text-primary" : "text-text-secondary"
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
            <div className="mr-2 flex items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1">
              <div className="h-1.5 w-1.5 rounded-full bg-success animate-pulse" />
              <span className="text-[10px] font-bold text-success uppercase tracking-wider">
                Running
              </span>
            </div>
          )}
          {onRefresh && (
            <button
              className={`navbar-icon-btn flex h-8 w-8 cursor-pointer items-center justify-center rounded-md border border-transparent bg-transparent text-base text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-elevated hover:text-text-secondary focus-visible:shadow-[0_0_0_2px_var(--primary)/25] focus-visible:outline-none ${
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
            className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-md border border-transparent bg-transparent text-lg text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-elevated hover:text-text-secondary sm:hidden"
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
                    ? "bg-bg-elevated text-text-primary"
                    : "text-text-secondary hover:bg-bg-elevated hover:text-text-primary"
                }`
              }
            >
              {tab.label}
            </NavLink>
          ))}
        </nav>
      )}
    </header>
  );
}
