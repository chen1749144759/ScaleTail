// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import path from "node:path";
import { app } from "electron";
import type { ConnectRequest, NetcheckReport, Prefs, Status } from "../shared/types";

const helperName = process.platform === "win32" ? "scaletail-localapi.exe" : "scaletail-localapi";
const maxHelperRequestBytes = 1 << 20;
const maxHelperStdoutBytes = 16 << 20;
const maxHelperStderrBytes = 256 << 10;

export type PasswordAuthErrorCode =
  | "invalid_credentials"
  | "account_locked"
  | "account_disabled"
  | "account_expired"
  | "password_expired"
  | "network_not_assigned"
  | "node_limit_reached"
  | "tags_not_supported"
  | "auth_session_expired"
  | "invalid_auth_session"
  | "machine_mismatch"
  | "auth_session_consumed"
  | "too_many_attempts"
  | "invalid_request"
  | "registration_failed"
  | "route_approval_failed"
  | "internal_error"
  | "authentication_failed"
  | "network_error"
  | "invalid_password"
  | "password_reused"
  | "password_not_expired"
  | "account_changed";

export class LocalAPIError extends Error {
  constructor(
    message: string,
    public statusCode?: number,
    public code?: string,
  ) {
    super(message);
    this.name = "LocalAPIError";
  }
}

export async function localRequest<T>(
  method: string,
  path: string,
  body?: unknown,
  expectedStatus = 200,
  timeoutMS = 15000,
  signal?: AbortSignal,
): Promise<T> {
  const payload = body === undefined ? undefined : JSON.stringify(body);
  if (payload && Buffer.byteLength(payload, "utf8") > maxHelperRequestBytes) {
    throw new LocalAPIError("LocalAPI 请求体过大");
  }
  return new Promise<T>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new Error("操作已取消"));
      return;
    }
    const child = spawn(
      helperPath(),
      [
        "request",
        "-method",
        method,
        "-path",
        path,
        "-expect",
        String(expectedStatus),
        "-timeout-ms",
        String(timeoutMS),
      ],
      { windowsHide: true },
    );
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    let stdoutBytes = 0;
    let stderrBytes = 0;
    let failed = false;
    const stopOnAbort = () => {
      if (failed) return;
      failed = true;
      child.kill();
      reject(new Error("操作已取消"));
    };
    signal?.addEventListener("abort", stopOnAbort, { once: true });
    const cleanup = () => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", stopOnAbort);
    };
    const timer = setTimeout(() => {
      failed = true;
      child.kill();
      signal?.removeEventListener("abort", stopOnAbort);
      reject(new Error("LocalAPI 请求超时"));
    }, timeoutMS + 3000);

    child.stdout.on("data", (chunk) => {
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      stdoutBytes += buffer.length;
      if (stdoutBytes > maxHelperStdoutBytes) {
        failed = true;
        child.kill();
        reject(new LocalAPIError("LocalAPI 响应体过大"));
        return;
      }
      stdout.push(buffer);
    });
    child.stderr.on("data", (chunk) => {
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      stderrBytes += buffer.length;
      if (stderrBytes > maxHelperStderrBytes) {
        failed = true;
        child.kill();
        reject(new LocalAPIError("LocalAPI 错误响应过大"));
        return;
      }
      stderr.push(buffer);
    });
    child.on("error", (err) => {
      cleanup();
      failed = true;
      reject(err);
    });
    child.on("close", (code) => {
      cleanup();
      if (failed) {
        return;
      }
      if (code !== 0) {
        const raw = Buffer.concat(stderr).toString("utf8").trim() || `LocalAPI helper 退出码 ${code}`;
        const parsed = parseLocalAPIError(raw, "LocalAPI 请求失败");
        reject(new LocalAPIError(parsed.message, parsed.statusCode, parsed.code));
        return;
      }
      const raw = Buffer.concat(stdout).toString("utf8").trim();
      if (!raw) {
        resolve(undefined as T);
        return;
      }
      try {
        resolve(JSON.parse(raw) as T);
      } catch {
        resolve(raw as T);
      }
    });
    if (payload) {
      child.stdin.write(payload);
    }
    child.stdin.end();
  });
}

export function watchIPNBus(onNotify: (notify: unknown) => void, onError: (err: Error) => void): () => void {
  let stopped = false;
  let child: ChildProcessWithoutNullStreams | undefined;

  const connect = () => {
    if (stopped) {
      return;
    }
    child = spawn(
      helperPath(),
      ["watch", "-path", "/localapi/v0/watch-ipn-bus?mask=0"],
      { windowsHide: true },
    );

    let pending = "";
    const stderr: Buffer[] = [];
    child.stdout.on("data", (chunk) => {
      pending += chunk.toString("utf8");
      let newline = pending.indexOf("\n");
      while (newline >= 0) {
        const line = pending.slice(0, newline).trim();
        pending = pending.slice(newline + 1);
        if (line) {
          try {
            onNotify(JSON.parse(line));
          } catch {
            // Ignore partial or unknown daemon messages.
          }
        }
        newline = pending.indexOf("\n");
      }
    });
    child.stderr.on("data", (chunk) => stderr.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)));
    child.on("error", (err) => {
      if (!stopped) {
        onError(err instanceof Error ? err : new Error(String(err)));
        setTimeout(connect, 3000);
      }
    });
    child.on("close", (code) => {
      if (!stopped) {
        if (code !== 0) {
          const message = Buffer.concat(stderr).toString("utf8").trim() || `LocalAPI helper 退出码 ${code}`;
          onError(new Error(message));
        }
        setTimeout(connect, 3000);
      }
    });
  };

  connect();
  return () => {
    stopped = true;
    child?.kill();
  };
}

function helperPath(): string {
  return bundledBinaryPath(helperName);
}

export function bundledBinaryPath(name: string): string {
  if (app.isPackaged) {
    return path.join(path.dirname(process.execPath), name);
  }
  return path.resolve(app.getAppPath(), "../../dist/windows-amd64", name);
}

export interface SignedUpdateInstallRequest {
  installer_path: string;
  policy_revision: number;
  update_type: "suggested" | "forced";
  version: string;
  platform: string;
  sha256: string;
  file_size: number;
  download_url: string;
  signature: string;
  marker_id: string;
}

export interface SignedUpdatePolicy {
  policy_revision: number;
  update_type: "suggested" | "forced" | "clear";
  version: string;
  platform: string;
  sha256: string;
  file_size: number;
  download_url: string;
  signature: string;
}

export interface UpdatePolicyStatus {
  active: boolean;
  current_version: string;
  resume_pending: boolean;
  policy: SignedUpdatePolicy;
}

export interface TrafficShaperRequest {
  upload_bits_per_second: number;
  download_bits_per_second: number;
  quota_exceeded: boolean;
  exceed_action: "alert" | "throttle" | "block";
}

export interface TrafficShaperStatus {
  config: {
    upload_bits_per_second: number;
    download_bits_per_second: number;
  };
  active: boolean;
  updated_at: string;
  upload_bytes: number;
  download_bytes: number;
  upload_wait_nanos: number;
  download_wait_nanos: number;
}

export interface TrafficShaperResponse {
  status: TrafficShaperStatus;
  quota_exceeded: boolean;
  exceed_action: "alert" | "throttle" | "block";
  blocked: boolean;
  warning?: string;
}

export async function installSignedUpdate(request: SignedUpdateInstallRequest): Promise<void> {
  await localRequest<{ accepted: boolean }>(
    "POST",
    "/localapi/v0/scaletail-update/install",
    request,
    202,
    120000,
  );
}

export async function getUpdatePolicyStatus(): Promise<UpdatePolicyStatus> {
  return localRequest<UpdatePolicyStatus>("GET", "/localapi/v0/scaletail-update/policy");
}

export async function applyUpdatePolicy(request: SignedUpdatePolicy): Promise<UpdatePolicyStatus> {
  return localRequest<UpdatePolicyStatus>("PUT", "/localapi/v0/scaletail-update/policy", request);
}

export async function acknowledgeUpdateResume(): Promise<UpdatePolicyStatus> {
  return localRequest<UpdatePolicyStatus>("PATCH", "/localapi/v0/scaletail-update/policy", {});
}

export async function setTrafficShaper(request: TrafficShaperRequest): Promise<TrafficShaperResponse> {
  return localRequest<TrafficShaperResponse>("POST", "/localapi/v0/traffic-shaper", request);
}

export async function getTrafficShaper(): Promise<TrafficShaperResponse> {
  return localRequest<TrafficShaperResponse>("GET", "/localapi/v0/traffic-shaper");
}

export async function reportScaleForgeTraffic<T>(body: unknown): Promise<T> {
  return localRequest<T>("POST", "/localapi/v0/scaleforge/traffic", body, 200, 20000);
}

export async function fetchScaleForgePolicy<T>(): Promise<T> {
  return localRequest<T>("GET", "/localapi/v0/scaleforge/policy", undefined, 200, 20000);
}

export async function reportScaleForgePolicyState<T>(body: unknown): Promise<T> {
  return localRequest<T>("POST", "/localapi/v0/scaleforge/policy-state", body, 200, 20000);
}

export async function fetchScaleForgeClientUpdate<T>(currentVersion: string, platform: string, currentRevision = 0): Promise<T> {
  const query = new URLSearchParams({
    current_version: currentVersion,
    platform,
    current_revision: String(currentRevision),
  });
  return localRequest<T>(
    "GET",
    `/localapi/v0/scaleforge/client-update?${query.toString()}`,
    undefined,
    200,
    20000,
  );
}

export async function getStatus(peers = true): Promise<Status> {
  return localRequest<Status>("GET", `/localapi/v0/status${peers ? "" : "?peers=false"}`);
}

export async function getPrefs(): Promise<Prefs> {
  return localRequest<Prefs>("GET", "/localapi/v0/prefs");
}

export interface MaskedPrefsRequest extends Partial<Prefs> {
  ExitNodeIDSet?: boolean;
  AdvertiseRoutesSet?: boolean;
  WantRunningSet?: boolean;
  LoggedOutSet?: boolean;
}

export function buildWantRunningPrefsPatch(
  wantRunning: boolean,
  ensureLoggedIn = false,
): MaskedPrefsRequest {
  const patch: MaskedPrefsRequest = {
    WantRunning: wantRunning,
    WantRunningSet: true,
  };
  if (ensureLoggedIn) {
    patch.LoggedOut = false;
    patch.LoggedOutSet = true;
  }
  return patch;
}

export async function patchPrefs(body: MaskedPrefsRequest): Promise<Prefs> {
  return localRequest<Prefs>("PATCH", "/localapi/v0/prefs", body);
}

export async function startDaemonUp(
  controlURL: string,
  hostname: string,
  acceptRoutes: boolean,
  acceptDNS: boolean,
): Promise<void> {
  await localRequest<void>("POST", "/localapi/v0/scaletail-up", {
    ControlURL: controlURL,
    Hostname: hostname,
    AcceptRoutes: acceptRoutes,
    AcceptDNS: acceptDNS,
  }, 204, 70000);
}

export async function logout(): Promise<void> {
  await localRequest<void>("POST", "/localapi/v0/logout", undefined, 204, 20000);
}

export async function setUseExitNode(enabled: boolean): Promise<Prefs> {
	return localRequest<Prefs>(
		"POST",
		`/localapi/v0/set-use-exit-node-enabled?enabled=${enabled ? "true" : "false"}`,
	);
}

export async function runNetcheck(): Promise<NetcheckReport> {
	return localRequest<NetcheckReport>("POST", "/localapi/v0/netcheck", undefined, 200, 45000);
}

export function buildControlURL(req: ConnectRequest): string {
  let host = req.serverIP.trim();
  let port = req.serverPort.trim();
  let useHTTPS = req.useHTTPS;

  if (host.includes("://")) {
    const parsed = new URL(host);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      throw new Error("服务端 URL 协议必须是 http 或 https");
    }
    if (parsed.username || parsed.password
      || (parsed.pathname && parsed.pathname !== "/")
      || parsed.search || parsed.hash) {
      throw new Error("服务端 URL 不能包含凭据、路径、查询参数或片段");
    }
    host = parsed.hostname;
    useHTTPS = parsed.protocol === "https:";
    if (!port) {
      port = parsed.port || (useHTTPS ? "443" : "80");
    }
  }

  if (!host) {
    throw new Error("请输入服务端地址");
  }
  if (!port) {
    port = useHTTPS ? "443" : "80";
  }
  const numericPort = Number(port);
  if (!Number.isInteger(numericPort) || numericPort < 1 || numericPort > 65535) {
    throw new Error("服务端端口必须在 1 到 65535 之间");
  }

  const scheme = useHTTPS ? "https" : "http";
  const urlHost = host.includes(":") && !host.startsWith("[") ? `[${host}]` : host;
  let parsed: URL;
  try {
    parsed = new URL(`${scheme}://${urlHost}:${numericPort}`);
  } catch {
    throw new Error("服务端地址格式无效");
  }
  return parsed.origin;
}

export function validateHostname(hostname: string): string {
  const value = hostname.trim();
  if (!value) {
    return "";
  }
  if (value.length > 253 || !/^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/.test(value)) {
    throw new Error("设备名称无效，只能使用字母、数字、短横线和点号");
  }
  return value;
}

function parseLocalAPIError(
  raw: string,
  fallback: string,
): { message: string; statusCode?: number; code?: string } {
  const statusCode = parseHTTPStatus(raw);
  const body = raw.replace(/^HTTP\s+\d{3}:\s*/i, "").trim();
  if (!body) {
    return { message: fallback, statusCode };
  }
  try {
    const parsed = JSON.parse(body) as {
      Code?: string;
      code?: string;
      Error?: string;
      error?: string;
      Message?: string;
      message?: string;
    };
    const code = parsed.code || parsed.Code;
    const message = parsed.message || parsed.Message || parsed.Error || parsed.error || fallback;
    return { message, statusCode, code };
  } catch {
    return { message: body, statusCode };
  }
}

function parseHTTPStatus(message: string): number | undefined {
  const match = message.match(/^HTTP\s+(\d{3})\b/i);
  return match ? Number(match[1]) : undefined;
}
