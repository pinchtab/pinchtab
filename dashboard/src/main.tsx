import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./i18n";
import "./index.css";
import App from "./App";
import { ErrorBoundary } from "./components/atoms";

// Global error handlers for debugging
window.onerror = (message, source, lineno, colno, error) => {
  console.error("💥 Global error:", { message, source, lineno, colno, error });
  return false;
};

window.onunhandledrejection = (event) => {
  console.error("💥 Unhandled promise rejection:", event.reason);
};

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
