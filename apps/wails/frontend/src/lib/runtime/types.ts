import type {
  CreateDesktopContextRequest,
  DesktopBootstrap,
  DesktopContext,
  DesktopInjectedRuntime,
  DesktopView,
  DesktopViewId,
  DesktopViewSession,
  StartDesktopViewSessionRequest,
} from "../../generated/desktopservice/types.gen";

export type BootstrapState = DesktopBootstrap;
export type ContextSummary = DesktopContext;
export type CreateContextRequest = CreateDesktopContextRequest;
export type RuntimeContext = Partial<DesktopInjectedRuntime>;
export type { DesktopView, DesktopViewId, DesktopViewSession, StartDesktopViewSessionRequest };

export interface DesktopAPI {
  Bootstrap(): Promise<DesktopBootstrap>;
  CreateContext(req: CreateDesktopContextRequest): Promise<DesktopContext>;
  EndViewSession(): Promise<DesktopViewSession>;
  GetViewSession(): Promise<DesktopViewSession>;
  InjectedRuntime(): Promise<DesktopInjectedRuntime>;
  ListContexts(): Promise<DesktopContext[]>;
  ListViews(): Promise<DesktopView[]>;
  SelectContext(name: string): Promise<DesktopContext>;
  StartViewSession(req: StartDesktopViewSessionRequest): Promise<DesktopViewSession>;
}
