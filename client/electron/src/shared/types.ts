// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

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
  ScaleTailIPs?: string[];
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
  ScaleTailIPs?: string[];
  ExitNodeStatus?: {
    ID?: string;
    Online?: boolean;
  };
}

export interface Prefs {
  ControlURL?: string;
  Hostname?: string;
  WantRunning?: boolean;
  LoggedOut?: boolean;
  RouteAll?: boolean;
  CorpDNS?: boolean;
  ExitNodeID?: string;
  AdvertiseRoutes?: string[];
  [key: string]: unknown;
}

export interface ConnectRequest {
  serverIP: string;
  serverPort: string;
  useHTTPS: boolean;
  hostname: string;
  username: string;
  password: string;
  acceptRoutes: boolean;
  acceptDNS: boolean;
}

export interface ConnectResponse {
  ok: boolean;
  controlURL: string;
  message: string;
  passwordChangeRequired?: boolean;
  passwordChangeRequiresRegistrationSession?: boolean;
}

export interface ChangeExpiredPasswordRequest extends ConnectRequest {
  newPassword: string;
  requireRegistrationSession: boolean;
}

export type PasswordChangeProgress = "preparing" | "updating" | "connecting";

export interface ClientReportConfig {
  enabled: boolean;
  intervalSeconds: number;
  flowEnabled: boolean;
  quotaGuardEnabled: boolean;
}

export interface ClientUpdateInfo {
  has_update: boolean;
  id?: number;
  policy_revision?: number;
  version?: string;
  platform?: string;
  update_type?: "suggested" | "forced" | "clear" | string;
  forced?: boolean;
  title?: string;
  description?: string;
  download_url?: string;
  sha256?: string;
  signature?: string;
  file_size?: number;
  release_notes?: string;
  created_at?: string;
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

export interface ScaleTailAPI {
  getStatus(peers?: boolean): Promise<Status>;
  getPrefs(): Promise<Prefs>;
  connect(req: ConnectRequest): Promise<ConnectResponse>;
  changeExpiredPassword(req: ChangeExpiredPasswordRequest): Promise<ConnectResponse>;
  disconnect(): Promise<{ ok: boolean; message: string }>;
  reconnect(): Promise<{ ok: boolean; message: string }>;
  logout(): Promise<{ ok: boolean }>;
  setExitNode(id: string): Promise<{ ok: boolean }>;
  setAdvertiseRoutes(routes: string[]): Promise<{ ok: boolean }>;
  runNetcheck(): Promise<NetcheckReport>;
  getServiceStatus(): Promise<ServiceOverview>;
  startService(): Promise<ServiceOverview>;
  cancelPasswordChange(): Promise<{ cancelled: boolean }>;
  openDashboard(): Promise<void>;
  openConnect(): Promise<void>;
  closeWindow(): Promise<void>;
  onNavigate(cb: (route: "dashboard" | "connect" | "nodes") => void): () => void;
  onDaemonEvent(cb: (event: unknown) => void): () => void;
  onPasswordChangeProgress(cb: (stage: PasswordChangeProgress) => void): () => void;
}
