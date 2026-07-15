import type {
  DesktopBootstrap,
  DesktopPod,
  PodInputWritable,
} from "../../generated/desktopservice/types.gen";

export type BootstrapState = DesktopBootstrap;
export type PodSummary = DesktopPod;
export type PodInput = PodInputWritable;

export interface RuntimeContext {
  context?: {
    name: string;
    description?: string;
    endpoint: string;
    local_public_key: string;
  };
  private_key_base64?: string;
  admin_server_id?: string;
  admin_servers?: Array<{
    id: string;
    name: string;
    context: NonNullable<RuntimeContext["context"]>;
    private_key_base64: string;
  }>;
}

export interface DesktopAPI {
  Bootstrap(): Promise<DesktopBootstrap>;
  CreatePod(input: PodInputWritable): Promise<DesktopPod>;
  DeletePod(id: string): Promise<void>;
  GetPod(id: string): Promise<DesktopPod>;
  ListPods(): Promise<DesktopPod[]>;
  OpenAdmin(podID: string, serverID: string): Promise<void>;
  OpenPlay(podID: string): Promise<void>;
  RefreshPodHealth(id: string): Promise<DesktopPod>;
  RevealPod(id: string): Promise<void>;
  RestartLocalServer(id: string): Promise<DesktopPod>;
  StartLocalServer(id: string): Promise<DesktopPod>;
  StopLocalServer(id: string): Promise<DesktopPod>;
  UpdatePod(input: PodInputWritable): Promise<DesktopPod>;
}
