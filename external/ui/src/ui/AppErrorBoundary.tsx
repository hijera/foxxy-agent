import React from "react";

type AppErrorBoundaryState = { error: Error | null };

/**
 * AppErrorBoundary catches render-time exceptions anywhere in the SPA tree and
 * shows a minimal recovery screen instead of unmounting everything. Without it
 * a render crash leaves the desktop WebView2 window completely blank while the
 * Go process keeps running. Text is intentionally static English (no i18n):
 * the boundary must render even when providers/localization are the crash cause.
 */
export class AppErrorBoundary extends React.Component<
  { children: React.ReactNode },
  AppErrorBoundaryState
> {
  override state: AppErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): AppErrorBoundaryState {
    return { error };
  }

  override componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Surface the crash in the console (visible via WebView2 CDP / devtools).
    console.error("foxxycode UI crashed:", error, info.componentStack);
  }

  override render() {
    if (!this.state.error) {
      return this.props.children;
    }
    return (
      <div className="app-error-boundary" data-testid="app-error-boundary">
        <h2>Something went wrong</h2>
        <p className="app-error-boundary-message">
          {String(this.state.error.message || this.state.error)}
        </p>
        <button
          type="button"
          className="app-error-boundary-reload"
          onClick={() => window.location.reload()}
        >
          Reload
        </button>
      </div>
    );
  }
}
