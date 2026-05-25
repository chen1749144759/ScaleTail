export type BackendState =
  | "NoState"
  | "NeedsLogin"
  | "NeedsMachineAuth"
  | "Starting"
  | "Running"
  | "Stopped"
  | string;

export interface PeerStatus {
  ID?: string;
  PublicKey?: string;
  HostName?: string;
  DNSName?: string;
  TailscaleIPs?: string[];
  Online?: boolean;
  ExitNodeOption?: boolean;
  CurAddr?: string;
  Relay?: string;
  RxBytes?: number;
  TxBytes?: number;
}

export interface Status {
  BackendState?: BackendState;
  HaveNodeKey?: boolean;
  AuthURL?: string;
  Self?: PeerStatus;
  Peer?: Record<string, PeerStatus>;
  CurrentTailnet?: {
    Name?: string;
  };
  TailscaleIPs?: string[];
  ExitNodeStatus?: {
    ID?: string;
    Online?: boolean;
  };
}

export interface Prefs {
  ControlURL?: string;
  Hostname?: string;
  WantRunning?: boolean;
  RouteAll?: boolean;
  ExitNodeID?: string;
  AdvertiseRoutes?: string[];
  [key: string]: unknown;
}

export interface ConnectRequest {
  serverIP: string;
  serverPort: string;
  useHTTPS: boolean;
  hostname: string;
  authKey: string;
  acceptRoutes: boolean;
}

export interface ConnectResponse {
  ok: boolean;
  controlURL: string;
  message: string;
}

export interface ServiceState {
  name: string;
  exists: boolean;
  state: "unknown" | "missing" | "stopped" | "start_pending" | "stop_pending" | "running" | "disabled";
  code?: number;
  raw?: string;
}

export interface ServiceOverview {
  service: ServiceState;
  dependencies: ServiceState[];
}

export interface NetcheckReport {
  UDP?: boolean;
  IPv4?: boolean;
  IPv6?: boolean;
  IPv6CanSend?: boolean;
  IPv6CanReceive?: boolean;
  MappingVariesByDestIP?: boolean;
  HairPinning?: boolean;
  UPnP?: boolean;
  PMP?: boolean;
  PCP?: boolean;
  PreferredDERP?: number;
  RegionLatency?: Record<string, number>;
  RegionV4Latency?: Record<string, number>;
  RegionV6Latency?: Record<string, number>;
  DERPLatency?: Record<string, number>;
  GlobalV4?: string;
  GlobalV6?: string;
  CaptivePortal?: string;
  [key: string]: unknown;
}

export interface TailscaleAPI {
  getStatus(peers?: boolean): Promise<Status>;
  getPrefs(): Promise<Prefs>;
  connect(req: ConnectRequest): Promise<ConnectResponse>;
  logout(): Promise<{ ok: boolean }>;
  setExitNode(id: string): Promise<{ ok: boolean }>;
  setAdvertiseRoutes(routes: string[]): Promise<{ ok: boolean }>;
  runNetcheck(): Promise<NetcheckReport>;
  getServiceStatus(): Promise<ServiceOverview>;
  startService(): Promise<ServiceOverview>;
  openDashboard(): Promise<void>;
  openConnect(): Promise<void>;
  closeWindow(): Promise<void>;
  onNavigate(cb: (route: "dashboard" | "connect" | "nodes") => void): () => void;
  onDaemonEvent(cb: (event: unknown) => void): () => void;
}
