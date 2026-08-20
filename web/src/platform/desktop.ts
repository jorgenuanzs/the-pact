import type { PactEvent, Principal } from "@/api/types";
import { Events, System } from "@wailsio/runtime";

import * as NativeDesktop from "@/generated/desktop-bindings/github.com/jorgenuanzs/the-pact/desktop/desktop";

export interface DesktopStatus {
  configured: boolean;
  connected: boolean;
  server_url?: string;
  principal?: Principal;
  error?: string;
  default_url: string;
}

export interface DesktopDeviceLogin {
  server_url: string;
  device_code: string;
  user_code: string;
  verification_url: string;
  expires_at: string;
  interval_seconds: number;
}

export interface DesktopDeviceLoginResult {
  status: string;
  connected: boolean;
  principal?: Principal;
  expires_at?: string;
}

export interface DesktopUpdateStatus {
  configured: boolean;
  current_version: string;
  commit: string;
  state: string;
  error?: string;
}

export interface LocalClientStatus {
  id: "codex" | "claude";
  name: string;
  detected: boolean;
  detection?: string;
  connected_folders: number;
}

export interface LocalFolder {
  root: string;
  name: string;
  server_url: string;
  project_id: string;
  clients: string[];
  available: boolean;
  status?: string;
  configured_at?: string;
}

export interface LocalComputerStatus {
  hostname: string;
  operating_system: string;
  architecture: string;
  runtime_ready: boolean;
  runtime_path?: string;
  runtime_version?: string;
  runtime_error?: string;
  server_url?: string;
  clients: LocalClientStatus[];
  folders: LocalFolder[];
  managed_server: LocalServerStatus;
}

export interface LocalServerStatus {
  installed: boolean;
  running: boolean;
  ready: boolean;
  server_url?: string;
  image?: string;
  version?: string;
  data_directory?: string;
  error?: string;
}

export interface LocalServerInstallInput {
  port: number;
  image?: string;
}

export interface LocalServerInstallResult {
  status: LocalServerStatus;
  setup_code: string;
}

export interface LocalServerUpgradeResult {
  status: LocalServerStatus;
  backup: string;
}

export interface LocalFolderInspection {
  canceled: boolean;
  connected: boolean;
  root?: string;
  name?: string;
  server_url?: string;
  project_id?: string;
  clients?: string[];
  error?: string;
}

export interface ConnectLocalAgentInput {
  client: "codex" | "claude";
  project_root: string;
}

export interface ConnectLocalAgentResult {
  client: "codex" | "claude";
  project_root: string;
  config_path: string;
  runtime_path: string;
  changed: boolean;
  restart_needed: boolean;
}

export interface DesktopAPIRequest {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: string;
}

export interface DesktopAPIResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
}

export interface DesktopStreamMessage {
  subscription_id: string;
  stream?: "project" | "directory";
  project_id: string;
  kind: "status" | "event";
  status?: "connecting" | "connected" | "reconnecting" | "offline";
  event_id?: string;
  data?: PactEvent;
  error?: string;
}

export interface DesktopBridge {
  Status(): Promise<DesktopStatus>;
  BeginDeviceLogin(serverURL: string): Promise<DesktopDeviceLogin>;
  PollDeviceLogin(serverURL: string, deviceCode: string): Promise<DesktopDeviceLoginResult>;
  OpenExternalURL(address: string): Promise<void>;
  Disconnect(localOnly: boolean): Promise<void>;
  LocalComputerStatus(): Promise<LocalComputerStatus>;
  SelectLocalProjectFolder(): Promise<LocalFolderInspection>;
  InspectLocalProjectFolder(path: string): Promise<LocalFolderInspection>;
  ConnectLocalAgent(input: ConnectLocalAgentInput): Promise<ConnectLocalAgentResult>;
  LocalServerStatus(): Promise<LocalServerStatus>;
  InstallLocalServer(input: LocalServerInstallInput): Promise<LocalServerInstallResult>;
  StartLocalServer(): Promise<LocalServerStatus>;
  StopLocalServer(): Promise<LocalServerStatus>;
  BackupLocalServer(): Promise<string>;
  UpgradeLocalServer(image: string): Promise<LocalServerUpgradeResult>;
  UpdateStatus(): Promise<DesktopUpdateStatus>;
  CheckForUpdates(): Promise<DesktopUpdateStatus>;
  APIRequest(input: DesktopAPIRequest): Promise<DesktopAPIResponse>;
  StartWorkspaceDirectoryStream(): Promise<string>;
  StopWorkspaceDirectoryStream(subscriptionID: string): Promise<void>;
  StartProjectEventStream(projectID: string, cursor: string): Promise<string>;
  StopProjectEventStream(subscriptionID: string): Promise<void>;
}

interface WailsRuntime {
  EventsOn(eventName: string, callback: (message: DesktopStreamMessage) => void): () => void;
}

declare global {
  interface Window {
    go?: { main?: { Desktop?: DesktopBridge } };
    runtime?: WailsRuntime;
  }
}

export const DESKTOP_STREAM_EVENT = "pact:desktop-project-stream";

export function desktopBridge(): DesktopBridge | null {
  if (window.go?.main?.Desktop) return window.go.main.Desktop;
  return System.IsDesktop() ? NativeDesktop as unknown as DesktopBridge : null;
}

export function isDesktopRuntime(): boolean {
  return desktopBridge() !== null;
}

export function onDesktopStreamMessage(listener: (message: DesktopStreamMessage) => void): () => void {
  if (window.runtime?.EventsOn) return window.runtime.EventsOn(DESKTOP_STREAM_EVENT, listener);
  if (!System.IsDesktop()) return () => undefined;
  return Events.On(DESKTOP_STREAM_EVENT, (event) => listener(event.data as DesktopStreamMessage));
}
