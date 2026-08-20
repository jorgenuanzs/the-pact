export type Identifier = string;

export type OrganizationRole = "owner" | "admin" | "member" | "observer" | string;

export interface Principal {
  id: Identifier;
  display_name?: string;
  email?: string;
  username?: string;
  organization_role?: OrganizationRole;
}

export interface ProjectSummary {
  id: Identifier;
  workspace_id?: Identifier;
  name: string;
  slug?: string;
  description?: string;
  status?: string;
  version?: number;
  root_repository?: string;
  root_repository_remote_url?: string;
  default_branch?: string;
  canonical_revision?: string;
  [key: string]: unknown;
}

export interface Workspace {
  id: Identifier;
  name: string;
  slug: string;
  description?: string;
  color?: string;
  status?: string;
  version?: number;
  projects?: ProjectSummary[];
  created_at?: string;
  updated_at?: string;
}

export interface GitHubStatus {
  configured?: boolean;
  connected?: boolean;
  suspended?: boolean;
  install_url?: string;
  account_login?: string;
  installation_id?: number;
  repositories?: number;
  message?: string;
  [key: string]: unknown;
}

export interface Actor {
  id?: Identifier;
  actor_id?: Identifier;
  principal_id?: Identifier;
  display_name?: string;
  name?: string;
  kind?: string;
  client?: string;
  status?: string;
  heartbeat_at?: string;
  last_seen_at?: string;
  [key: string]: unknown;
}

export interface Intent {
  id: Identifier;
  actor_id?: Identifier;
  actor?: Actor;
  objective?: string;
  summary?: string;
  status?: string;
  branch?: string;
  base_revision?: string;
  repository?: string;
  repository_id?: Identifier;
  scopes?: Array<string | Record<string, unknown>>;
  heartbeat_at?: string;
  created_at?: string;
  updated_at?: string;
  [key: string]: unknown;
}

export interface PactEvent {
  id?: Identifier;
  project_id?: Identifier;
  sequence?: string | number;
  type?: string;
  event_type?: string;
  actor_id?: Identifier;
  session_id?: Identifier;
  intent_id?: Identifier;
  actor?: Actor;
  occurred_at?: string;
  created_at?: string;
  data?: Record<string, unknown>;
  payload?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface ProjectOverview {
  project?: ProjectSummary;
  intents?: Intent[];
  active_work?: Intent[];
  work_items?: Array<Record<string, unknown>>;
  work?: Intent[];
  sessions?: Actor[];
  actors?: Actor[];
  events?: PactEvent[];
  recent_events?: PactEvent[];
  handoffs?: Array<Record<string, unknown>>;
  counts?: Record<string, number>;
  attention?: Record<string, unknown> | Array<Record<string, unknown>>;
  activity?: Record<string, unknown>;
  code_activity?: Record<string, unknown>;
  repository_sync?: Record<string, unknown>;
  generated_at?: string;
  [key: string]: unknown;
}

export interface WorkspaceContext {
  decisions?: Array<Record<string, unknown>>;
  requirements?: Array<Record<string, unknown>>;
  constraints?: Array<Record<string, unknown>>;
  open_questions?: Array<Record<string, unknown>>;
  risks?: Array<Record<string, unknown>>;
  resources?: Array<Record<string, unknown>>;
  warnings?: string[];
  handoffs?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export interface Room {
  id: Identifier;
  workspace_id?: Identifier;
  name: string;
  description?: string;
  managed_default?: boolean;
  message_count?: number;
  last_message_at?: string;
  [key: string]: unknown;
}

export interface RoomMessage {
  id: Identifier;
  room_id?: Identifier;
  author_actor_id?: Identifier;
  author_display_name?: string;
  author_kind?: string;
  actor_id?: Identifier;
  actor?: Actor;
  display_name?: string;
  body?: string;
  content?: string;
  reply_to_message_id?: Identifier;
  created_at?: string;
  [key: string]: unknown;
}

export interface RoomMention {
  id: Identifier;
  workspace_id: Identifier;
  room_id: Identifier;
  message_id?: Identifier;
  status?: string;
  room_name?: string;
  message_excerpt?: string;
  created_at?: string;
  [key: string]: unknown;
}

export interface Repository {
  id: Identifier;
  project_id?: Identifier;
  github_repository_id?: number;
  full_name?: string;
  github_full_name?: string;
  name?: string;
  owner?: string;
  description?: string;
  purpose?: string;
  required?: boolean;
  primary?: boolean;
  default_branch?: string;
  canonical_revision?: string;
  sync_status?: string;
  synced_at?: string;
  last_success_at?: string;
  html_url?: string;
  [key: string]: unknown;
}

export interface AvailableRepository {
  id: number;
  full_name: string;
  name?: string;
  owner?: string;
  private?: boolean;
  default_branch?: string;
  html_url?: string;
  attached?: boolean;
  [key: string]: unknown;
}

export interface ProjectAccess {
  members?: Actor[];
  agents?: Actor[];
  sessions?: Actor[];
  [key: string]: unknown;
}

export interface AdminUser {
  principal_id: Identifier;
  display_name?: string;
  email?: string;
  username?: string;
  organization_role?: OrganizationRole;
  status?: string;
  active?: boolean;
  project_roles?: Record<Identifier, string> | Array<Record<string, unknown>>;
  sessions?: number;
  devices?: number;
  last_seen_at?: string;
  created_at?: string;
  [key: string]: unknown;
}

export interface Invitation {
  id: Identifier;
  email?: string;
  organization_role?: OrganizationRole;
  project_id?: Identifier;
  project_role?: string;
  status?: string;
  expires_at?: string;
  created_at?: string;
  secret?: string;
  [key: string]: unknown;
}

export interface AdminAuditEvent extends PactEvent {
  action?: string;
  target?: string;
  detail?: string;
}

export interface UserDirectory {
  users: AdminUser[];
  invitations: Invitation[];
  events: AdminAuditEvent[];
}

export interface SetupStatus {
  required: boolean;
  configured?: boolean;
}

export interface InvitationPreview {
  email?: string;
  organization_role?: OrganizationRole;
  expires_at?: string;
}
