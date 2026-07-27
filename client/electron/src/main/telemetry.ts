import { spawn } from "node:child_process";
import { readClientReportConfig } from "./report_config";
import {
  setTrafficShaper,
  type TrafficShaperRequest,
  type TrafficShaperResponse,
} from "./localapi";
import type { ClientReportConfig, NetcheckReport, Status } from "../shared/types";

type StatusReader = (peers?: boolean) => Promise<Status>;
type NetcheckReader = () => Promise<NetcheckReport>;

interface TelemetryReporterOptions {
  getStatus: StatusReader;
  runNetcheck: NetcheckReader;
  intervalMS?: number;
  netcheckIntervalMS?: number;
}

interface ReportConfig {
  baseURL: string;
  token: string;
  intervalSeconds: number;
  flowEnabled: boolean;
  quotaGuardEnabled: boolean;
}

interface FlowSummary {
  window_start: string;
  window_seconds: number;
  dst_ip: string;
  dst_port?: number;
  protocol: string;
  direction: string;
  bytes: number;
  packets: number;
  connection_count: number;
  state?: string;
  process_id?: number;
  process_name?: string;
}

interface RawFlowRecord {
  dst_ip?: unknown;
  dst_port?: unknown;
  protocol?: unknown;
  direction?: unknown;
  state?: unknown;
  process_id?: unknown;
  process_name?: unknown;
}

interface TrafficReportPayload {
  machine_name: string;
  group_name: string;
  scaletail_ips: string[];
  rx_bytes_total: number;
  tx_bytes_total: number;
  derp: boolean;
  endpoint_type: string;
  public_ip: string;
  flows: FlowSummary[];
}

interface EffectivePolicy {
  rate_up_mbps?: number | null;
  rate_down_mbps?: number | null;
  monthly_quota_gb?: number | null;
  month_bytes?: number | null;
  quota_exceeded?: boolean;
  exceed_action?: string;
}

interface PolicyResponse {
  data?: {
    effective?: EffectivePolicy;
    matched_policies?: Array<{ id?: number }>;
  };
}

interface PolicyApplyResult {
  applied: boolean;
  error: string;
  effectivePolicy: EffectivePolicy;
  policyID?: number;
}

interface QuotaGuard {
  quotaBytes: number;
  monthBytes: number;
  baseTrafficBytes: number;
  exceedAction: string;
  effectivePolicy: EffectivePolicy;
  policyID?: number;
}

let timer: NodeJS.Timeout | undefined;
let running = false;
let lastNetcheckAt = 0;
let lastNetcheck: NetcheckReport | undefined;
let quotaGuard: QuotaGuard | undefined;
let disabledPolicyCleared = false;
let policyEpoch = 0;
let trafficShaperQueue: Promise<void> = Promise.resolve();

export function resetTelemetryPolicyState(): void {
  quotaGuard = undefined;
  disabledPolicyCleared = false;
  const epoch = ++policyEpoch;
  void enqueueTrafficShaper(emptyTrafficShaperRequest(), epoch).catch((err) => {
    console.warn("ScaleTail policy reset failed:", err);
  });
}

export function startTelemetryReporter(options: TelemetryReporterOptions): () => void {
  const stored = readClientReportConfig();
  const intervalMS = options.intervalMS ?? secondsToMS(stored.intervalSeconds);
  void collectAndReport(options);
  timer = setInterval(() => void collectAndReport(options), intervalMS);

  return () => {
    if (timer) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

async function collectAndReport(options: TelemetryReporterOptions): Promise<void> {
  const epoch = policyEpoch;
  const config = readReportConfig();
  if (!config) {
    if (!disabledPolicyCleared) {
      try {
        const response = await enqueueTrafficShaper(emptyTrafficShaperRequest(), epoch);
        if (response && epoch === policyEpoch) {
          quotaGuard = undefined;
          disabledPolicyCleared = true;
        }
      } catch (err) {
        console.warn("ScaleTail policy clear failed:", err);
      }
    }
    return;
  }
  disabledPolicyCleared = false;
  if (running) {
    return;
  }
  running = true;
  try {
    const status = await options.getStatus(true);
    if (status.BackendState !== "Running") {
      quotaGuard = undefined;
      return;
    }

    const now = Date.now();
    const netcheckIntervalMS = options.netcheckIntervalMS ?? 10 * 60_000;
    if (!lastNetcheck || now - lastNetcheckAt > netcheckIntervalMS) {
      try {
        lastNetcheck = await options.runNetcheck();
        lastNetcheckAt = now;
      } catch {
        lastNetcheck = undefined;
      }
    }

    const report = await buildTrafficReport(status, config, lastNetcheck);
    await postJSON(endpoint(config.baseURL, "/traffic"), config.token, report);
    if (epoch !== policyEpoch) {
      return;
    }

    if (config.quotaGuardEnabled) {
      const guardResult = await enforceLocalQuotaGuard(report, epoch);
      if (guardResult) {
        await reportPolicyState(config, report, guardResult);
        return;
      }
    }

    try {
      const policy = await fetchPolicy(config, report);
      if (epoch !== policyEpoch) {
        return;
      }
      const result = await applyPolicy(policy, epoch);
      if (!result) {
        return;
      }
      updateQuotaGuard(config, report, result);
      await reportPolicyState(config, report, result);
    } catch (err) {
      console.warn("ScaleTail policy fetch failed:", err);
    }
  } catch (err) {
    console.warn("ScaleTail telemetry report failed:", err);
  } finally {
    running = false;
  }
}

function readReportConfig(): ReportConfig | undefined {
  const stored = readClientReportConfig();
  const baseURL = stored.baseURL.trim();
  const token = stored.token.trim();
  if (!stored.enabled || !baseURL || !token) {
    return undefined;
  }
  return {
    baseURL,
    token,
    intervalSeconds: normalizeInterval(stored.intervalSeconds),
    flowEnabled: stored.flowEnabled,
    quotaGuardEnabled: stored.quotaGuardEnabled,
  };
}

async function buildTrafficReport(
  status: Status,
  config: ReportConfig,
  netcheck?: NetcheckReport,
): Promise<TrafficReportPayload> {
  const peers = Object.values(status.Peer || {});
  const self = status.Self || {};
  const rxBytesTotal = peers.reduce((sum, peer) => sum + Number(peer.RxBytes || 0), 0);
  const txBytesTotal = peers.reduce((sum, peer) => sum + Number(peer.TxBytes || 0), 0);
  const publicIP = String(netcheck?.GlobalV4 || netcheck?.GlobalV6 || "");
  const scaleTailIPs = status.ScaleTailIPs?.length ? status.ScaleTailIPs : (self.ScaleTailIPs || []);

  return {
    machine_name: self.DNSName?.split(".")[0] || self.HostName || "",
    group_name: status.CurrentTailnet?.Name || "",
    scaletail_ips: scaleTailIPs,
    rx_bytes_total: rxBytesTotal,
    tx_bytes_total: txBytesTotal,
    derp: peers.some((peer) => Boolean(peer.Relay)),
    endpoint_type: peers.some((peer) => Boolean(peer.CurAddr)) ? "direct" : "derp",
    public_ip: publicIP,
    flows: config.flowEnabled ? await collectFlowSummaries(config.intervalSeconds, scaleTailIPs) : [],
  };
}

async function fetchPolicy(config: ReportConfig, report: TrafficReportPayload): Promise<PolicyResponse> {
  const params = new URLSearchParams();
  if (report.machine_name) {
    params.set("machine_name", report.machine_name);
  }
  if (report.group_name) {
    params.set("group_name", report.group_name);
  }
  const url = `${endpoint(config.baseURL, "/policy")}?${params.toString()}`;
  const res = await fetch(url, {
    headers: { "X-ScaleTail-Token": config.token },
  });
  if (!res.ok) {
    throw new Error(`policy fetch failed: HTTP ${res.status}`);
  }
  return await res.json() as PolicyResponse;
}

async function applyPolicy(policy: PolicyResponse, epoch: number): Promise<PolicyApplyResult | undefined> {
  const effectivePolicy = policy.data?.effective || {};
  const policyID = policy.data?.matched_policies?.[0]?.id;
  const rateUp = positiveNumber(effectivePolicy.rate_up_mbps);
  const rateDown = positiveNumber(effectivePolicy.rate_down_mbps);
  const quotaExceeded = Boolean(effectivePolicy.quota_exceeded);
  const exceedAction = normalizeExceedAction(effectivePolicy.exceed_action);
  const request: TrafficShaperRequest = {
    upload_bits_per_second: mbpsToBitsPerSecond(rateUp),
    download_bits_per_second: mbpsToBitsPerSecond(rateDown),
    quota_exceeded: quotaExceeded,
    exceed_action: exceedAction,
  };

  try {
    const response = await enqueueTrafficShaper(request, epoch);
    if (!response) {
      return undefined;
    }
    const error = response.warning || "";
    return {
      applied: !error,
      error,
      effectivePolicy,
      policyID,
    };
  } catch (err) {
    return {
      applied: false,
      error: `ScaleTail 核心限速策略应用失败：${messageOf(err)}`,
      effectivePolicy,
      policyID,
    };
  }
}

async function enforceLocalQuotaGuard(
  report: TrafficReportPayload,
  epoch: number,
): Promise<PolicyApplyResult | undefined> {
  if (!quotaGuard) {
    return undefined;
  }
  const observedMonthBytes = quotaGuard.monthBytes + Math.max(0, trafficBytes(report) - quotaGuard.baseTrafficBytes);
  if (observedMonthBytes <= quotaGuard.quotaBytes) {
    return undefined;
  }
  const effectivePolicy: EffectivePolicy = {
    ...quotaGuard.effectivePolicy,
    month_bytes: observedMonthBytes,
    quota_exceeded: true,
    exceed_action: quotaGuard.exceedAction,
  };
  return applyPolicy({
    data: {
      effective: effectivePolicy,
      matched_policies: quotaGuard.policyID ? [{ id: quotaGuard.policyID }] : [],
    },
  }, epoch);
}

function updateQuotaGuard(config: ReportConfig, report: TrafficReportPayload, result: PolicyApplyResult): void {
  if (!config.quotaGuardEnabled) {
    quotaGuard = undefined;
    return;
  }
  const quotaGB = positiveNumber(result.effectivePolicy.monthly_quota_gb);
  if (!quotaGB) {
    quotaGuard = undefined;
    return;
  }
  quotaGuard = {
    quotaBytes: Math.round(quotaGB * 1024 * 1024 * 1024),
    monthBytes: Math.max(0, Number(result.effectivePolicy.month_bytes || 0)),
    baseTrafficBytes: trafficBytes(report),
    exceedAction: result.effectivePolicy.exceed_action || "alert",
    effectivePolicy: result.effectivePolicy,
    policyID: result.policyID,
  };
}

async function reportPolicyState(
  config: ReportConfig,
  report: TrafficReportPayload,
  result: PolicyApplyResult,
): Promise<void> {
  await postJSON(endpoint(config.baseURL, "/policy-state"), config.token, {
    policy_id: result.policyID,
    machine_name: report.machine_name,
    applied: result.applied,
    effective_policy: result.effectivePolicy,
    error: result.error,
  });
}

async function collectFlowSummaries(windowSeconds: number, scaleTailIPs: string[]): Promise<FlowSummary[]> {
  try {
    if (process.platform === "win32") {
      return aggregateFlows(await collectWindowsTCPFlows(scaleTailIPs), windowSeconds);
    }
  } catch (err) {
    console.warn("ScaleTail flow collection failed:", err);
  }
  return [];
}

async function collectWindowsTCPFlows(scaleTailIPs: string[]): Promise<RawFlowRecord[]> {
  const cleanIPs = scaleTailIPs.map((value) => value.trim()).filter(Boolean);
  if (cleanIPs.length === 0) {
    return [];
  }
  const ipList = cleanIPs.map(powerShellQuote).join(",");
  const script = `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [Console]::OutputEncoding
$scaleTailIPs = @(${ipList})
$scaleTailInterfaces = @(
  Get-NetIPAddress -ErrorAction SilentlyContinue |
    Where-Object { $scaleTailIPs -contains [string]$_.IPAddress } |
    Select-Object -ExpandProperty InterfaceIndex -Unique
)
if ($scaleTailInterfaces.Count -eq 0) {
  ConvertTo-Json -InputObject @() -Compress
  exit 0
}
$proc = @{}
$routeCache = @{}
Get-Process | ForEach-Object { $proc[[int]$_.Id] = $_.ProcessName }
$items = @(
  Get-NetTCPConnection -State Established -ErrorAction SilentlyContinue |
    Where-Object {
      $_.RemoteAddress -and $_.RemotePort -and
      $_.RemoteAddress -notin @('127.0.0.1','::1','0.0.0.0','::')
    } |
    ForEach-Object {
      $remote = [string]$_.RemoteAddress
      if (-not $routeCache.ContainsKey($remote)) {
        $route = Find-NetRoute -RemoteIPAddress $remote -ErrorAction SilentlyContinue |
          Select-Object -First 1
        $routeCache[$remote] = if ($route) { [int]$route.InterfaceIndex } else { -1 }
      }
      if ($scaleTailInterfaces -contains [int]$routeCache[$remote]) {
        [pscustomobject]@{
          dst_ip = $remote
          dst_port = [int]$_.RemotePort
          protocol = 'tcp'
          direction = 'outbound'
          state = [string]$_.State
          process_id = [int]$_.OwningProcess
          process_name = [string]$proc[[int]$_.OwningProcess]
        }
      }
    }
)
ConvertTo-Json -InputObject $items -Depth 3 -Compress
`;
  return parseJSONRecords(await runCommandCapture("powershell.exe", [
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-Command",
    script,
  ], 10_000));
}

function aggregateFlows(records: RawFlowRecord[], windowSeconds: number): FlowSummary[] {
  const windowStart = new Date(Date.now() - secondsToMS(windowSeconds)).toISOString();
  const buckets = new Map<string, FlowSummary>();
  for (const record of records) {
    const dstIP = String(record.dst_ip || "").trim();
    if (!dstIP || isLocalAddress(dstIP)) {
      continue;
    }
    const dstPort = toOptionalNumber(record.dst_port);
    const protocol = String(record.protocol || "tcp").toLowerCase();
    const direction = String(record.direction || "outbound").toLowerCase();
    const state = String(record.state || "").trim();
    const processID = toOptionalNumber(record.process_id);
    const processName = String(record.process_name || "").trim();
    const key = [dstIP, dstPort || "", protocol, direction, state, processID || "", processName].join("|");
    const current = buckets.get(key) || {
      window_start: windowStart,
      window_seconds: windowSeconds,
      dst_ip: dstIP,
      dst_port: dstPort,
      protocol,
      direction,
      bytes: 0,
      packets: 0,
      connection_count: 0,
      state,
      process_id: processID,
      process_name: processName,
    };
    current.packets += 1;
    current.connection_count += 1;
    buckets.set(key, current);
  }
  return [...buckets.values()]
    .sort((a, b) => b.connection_count - a.connection_count)
    .slice(0, 500);
}

function runCommandCapture(command: string, args: string[], timeoutMS: number): Promise<string> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { windowsHide: true });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    const timer = setTimeout(() => {
      child.kill();
      reject(new Error(`${command} 执行超时`));
    }, timeoutMS);
    child.stdout.on("data", (chunk) => stdout.push(Buffer.from(chunk)));
    child.stderr.on("data", (chunk) => stderr.push(Buffer.from(chunk)));
    child.on("error", (err) => {
      clearTimeout(timer);
      reject(err);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      const out = Buffer.concat(stdout).toString("utf8").trim();
      if (code === 0) {
        resolve(out);
        return;
      }
      const detail = Buffer.concat(stderr).toString("utf8").trim() || out || `退出码 ${code}`;
      reject(new Error(detail));
    });
  });
}

async function postJSON(url: string, token: string, body: unknown): Promise<void> {
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-ScaleTail-Token": token,
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`HTTP ${res.status}: ${text}`);
  }
}

function endpoint(baseURL: string, pathName: string): string {
  const cleanBase = baseURL.endsWith("/") ? baseURL.slice(0, -1) : baseURL;
  const cleanPath = pathName.startsWith("/") ? pathName : `/${pathName}`;
  if (cleanBase.endsWith("/api/client-reports")) {
    return `${cleanBase}${cleanPath}`;
  }
  return `${cleanBase}/api/client-reports${cleanPath}`;
}

function parseJSONRecords(raw: string): RawFlowRecord[] {
  if (!raw) {
    return [];
  }
  const parsed = JSON.parse(raw) as RawFlowRecord | RawFlowRecord[];
  return Array.isArray(parsed) ? parsed : [parsed];
}

function trafficBytes(report: TrafficReportPayload): number {
  return Number(report.rx_bytes_total || 0) + Number(report.tx_bytes_total || 0);
}

function normalizeInterval(value: unknown): number {
  const n = Math.round(Number(value || 15));
  if (!Number.isFinite(n)) {
    return 15;
  }
  return Math.max(5, Math.min(3600, n));
}

function secondsToMS(seconds: number): number {
  return normalizeInterval(seconds) * 1000;
}

function positiveNumber(value: unknown): number | undefined {
  const n = Number(value || 0);
  return Number.isFinite(n) && n > 0 ? n : undefined;
}

function toOptionalNumber(value: unknown): number | undefined {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : undefined;
}

function isLocalAddress(value: string): boolean {
  return value === "127.0.0.1"
    || value === "::1"
    || value === "0.0.0.0"
    || value === "::"
    || value.startsWith("127.")
    || value.startsWith("169.254.");
}

function powerShellQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function mbpsToBitsPerSecond(value?: number): number {
  if (!value) {
    return 0;
  }
  return Math.min(1_000_000_000_000, Math.max(1, Math.round(value * 1_000_000)));
}

function normalizeExceedAction(value: unknown): "alert" | "throttle" | "block" {
  const action = String(value || "alert").toLowerCase();
  if (action === "throttle" || action === "block") {
    return action;
  }
  return "alert";
}

function emptyTrafficShaperRequest(): TrafficShaperRequest {
  return {
    upload_bits_per_second: 0,
    download_bits_per_second: 0,
    quota_exceeded: false,
    exceed_action: "alert",
  };
}

function enqueueTrafficShaper(
  request: TrafficShaperRequest,
  epoch: number,
): Promise<TrafficShaperResponse | undefined> {
  const operation = trafficShaperQueue.then(async () => {
    if (epoch !== policyEpoch) {
      return undefined;
    }
    return setTrafficShaper(request);
  });
  trafficShaperQueue = operation.then(() => undefined, () => undefined);
  return operation;
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err || "未知错误");
}
