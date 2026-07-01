export interface ContextSummary {
  current: boolean;
  description?: string;
  endpoint: string;
  local_public_key?: string;
  name: string;
  server_public_key: string;
}

export interface RuntimeContext {
  context?: ContextSummary;
  private_key_base64?: string;
  signaling_url?: string;
}

export interface AppState {
  selected_context?: string;
  selected_view?: string;
}

export interface AppPaths {
  config_root: string;
  context_dir: string;
  state_file: string;
}

export interface BootstrapState {
  contexts: ContextSummary[];
  paths: AppPaths;
  runtime: RuntimeContext;
  state: AppState;
}

export interface CreateContextRequest {
  description?: string;
  endpoint: string;
  name: string;
  server_public_key: string;
}

export interface DesktopAPI {
  Bootstrap(): Promise<BootstrapState>;
  CreateContext(req: CreateContextRequest): Promise<RuntimeContext>;
  ListContexts(): Promise<ContextSummary[]>;
  RuntimeContext(): Promise<RuntimeContext>;
  SelectContext(name: string): Promise<RuntimeContext>;
  SetSelectedView(view: string): Promise<AppState>;
}

export type DesktopView = "admin" | "play";
