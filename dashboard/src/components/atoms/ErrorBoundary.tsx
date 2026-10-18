import { Component, type ReactNode } from "react";
import i18n from "../../i18n";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error("ErrorBoundary caught:", error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return (
        this.props.fallback || (
          <div className="flex h-full items-center justify-center p-4">
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
              <div className="mb-2 text-lg">
                {i18n.t("errors.somethingWentWrong")}
              </div>
              <div className="text-sm text-text-muted">
                {this.state.error?.message || i18n.t("errors.unknownError")}
              </div>
              <button
                className="mt-3 rounded bg-primary px-3 py-1 text-sm text-white"
                onClick={() => this.setState({ hasError: false, error: null })}
              >
                {i18n.t("errors.tryAgain")}
              </button>
            </div>
          </div>
        )
      );
    }

    return this.props.children;
  }
}
