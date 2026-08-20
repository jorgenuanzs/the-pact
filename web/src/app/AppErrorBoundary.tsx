import { Component, type ErrorInfo, type ReactNode } from "react";

interface AppErrorBoundaryProps {
  children: ReactNode;
}

interface AppErrorBoundaryState {
  failed: boolean;
}

export class AppErrorBoundary extends Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  state: AppErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): AppErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("PACT interface failed", error, info.componentStack);
  }

  private reload = () => {
    window.location.reload();
  };

  private returnHome = () => {
    if (window.location.hash) {
      window.location.hash = "#/";
    } else {
      window.location.assign("/admin/");
    }
    window.location.reload();
  };

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="app-failure" role="alert">
        <div>
          <span>PACT</span>
          <h1>No se pudo mostrar esta sección</h1>
          <p>La aplicación encontró un problema local. Puedes volver al inicio o recargarla sin perder el trabajo del servidor.</p>
          <div>
            <button type="button" onClick={this.returnHome}>Volver al inicio</button>
            <button type="button" onClick={this.reload}>Recargar PACT</button>
          </div>
        </div>
      </main>
    );
  }
}
