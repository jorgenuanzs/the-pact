import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { BrowserRouter, HashRouter } from "react-router-dom";

import { ToastProvider } from "@/components/ui/Toast";
import { AuthProvider } from "@/features/auth";
import { DesktopGate } from "@/features/desktop/DesktopGate";
import { isDesktopRuntime } from "@/platform/desktop";

import { AppErrorBoundary } from "./AppErrorBoundary";
import { queryClient } from "./queryClient";

export function AppProviders({ children }: { children: ReactNode }) {
  const content = (
    <AppErrorBoundary>
      <ToastProvider>
        <DesktopGate>
          <AuthProvider>{children}</AuthProvider>
        </DesktopGate>
      </ToastProvider>
    </AppErrorBoundary>
  );
  return (
    <QueryClientProvider client={queryClient}>
      {isDesktopRuntime() ? (
        <HashRouter>{content}</HashRouter>
      ) : (
        <BrowserRouter basename="/admin">{content}</BrowserRouter>
      )}
    </QueryClientProvider>
  );
}
