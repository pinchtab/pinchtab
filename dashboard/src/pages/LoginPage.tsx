import { useState } from "react";
import type { ComponentProps } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Button, Card } from "../components/atoms";
import * as api from "../services/api";
import {
  credentialUsername,
  dispatchAuthStateChanged,
  storeTokenCredential,
} from "../services/auth";

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const savedCredentialUsername = credentialUsername();

  const from =
    (location.state as { from?: string } | null)?.from ||
    "/dashboard/monitoring";

  const handleSubmit: NonNullable<ComponentProps<"form">["onSubmit"]> = async (
    event,
  ) => {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await api.login(token);
      void storeTokenCredential(token);
      dispatchAuthStateChanged();
      navigate(from, { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Authentication failed");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg-app px-4">
      <Card className="w-full max-w-md p-6">
        <div className="mb-6">
          <div className="dashboard-section-label mb-2">Authentication</div>
          <h1 className="text-xl font-semibold text-text-primary">
            Enter API token
          </h1>
          <p className="mt-2 text-sm leading-6 text-text-muted">
            This PinchTab server requires a bearer token before the dashboard
            can load protected routes and APIs.
          </p>
        </div>

        <form className="space-y-4" autoComplete="on" onSubmit={handleSubmit}>
          <input
            type="text"
            name="username"
            autoComplete="username"
            value={savedCredentialUsername}
            readOnly
            tabIndex={-1}
            aria-hidden="true"
            className="sr-only"
          />
          <input
            type="password"
            autoFocus
            name="password"
            autoComplete="current-password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            className="w-full rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            placeholder="Paste bearer token"
            spellCheck={false}
            autoCapitalize="none"
          />
          {error && (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive">
              {error}
            </div>
          )}
          <Button type="submit" disabled={submitting || token.trim() === ""}>
            {submitting ? "Authorizing..." : "Continue"}
          </Button>
        </form>
      </Card>
    </div>
  );
}
