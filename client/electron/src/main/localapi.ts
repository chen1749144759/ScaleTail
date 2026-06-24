import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import path from "node:path";
import { app } from "electron";
import { ConnectRequest, NetcheckReport, Prefs, Status } from "../shared/types";

const helperName = process.platform === "win32" ? "scaletail-localapi.exe" : "scaletail-localapi";

export class LocalAPIError extends Error {
  constructor(message: string, public statusCode?: number) {
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
): Promise<T> {
  const payload = body === undefined ? undefined : JSON.stringify(body);
  return new Promise<T>((resolve, reject) => {
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
    const timer = setTimeout(() => {
      child.kill();
      reject(new Error("LocalAPI 请求超时"));
    }, timeoutMS + 3000);

    child.stdout.on("data", (chunk) => stdout.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)));
    child.stderr.on("data", (chunk) => stderr.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)));
    child.on("error", (err) => {
      clearTimeout(timer);
      reject(err);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      if (code !== 0) {
        const message = Buffer.concat(stderr).toString("utf8").trim() || `LocalAPI helper 退出码 ${code}`;
        reject(new LocalAPIError(errorMessage(message, "LocalAPI 请求失败"), parseHTTPStatus(message)));
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

function bundledBinaryPath(name: string): string {
  if (app.isPackaged) {
    return path.join(path.dirname(process.execPath), name);
  }
  return path.resolve(app.getAppPath(), "../../dist/windows-amd64", name);
}

export async function getStatus(peers = true): Promise<Status> {
  return localRequest<Status>("GET", `/localapi/v0/status${peers ? "" : "?peers=false"}`);
}

export async function getPrefs(): Promise<Prefs> {
  return localRequest<Prefs>("GET", "/localapi/v0/prefs");
}

export async function patchPrefs(body: unknown): Promise<Prefs> {
  return localRequest<Prefs>("PATCH", "/localapi/v0/prefs", body);
}

export async function startDaemonUp(
  controlURL: string,
  hostname: string,
  authKey: string,
  acceptRoutes: boolean,
): Promise<void> {
  await localRequest<void>("POST", "/localapi/v0/scaletail-up", {
    ControlURL: controlURL,
    Hostname: hostname,
    AuthKey: authKey,
    AcceptRoutes: acceptRoutes,
    AcceptDNS: true,
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
  return `${scheme}://${host}:${numericPort}`;
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

function errorMessage(raw: string, fallback: string): string {
  if (!raw) {
    return fallback;
  }
  try {
    const parsed = JSON.parse(raw) as { Error?: string; error?: string };
    return parsed.Error || parsed.error || raw;
  } catch {
    return raw;
  }
}

function parseHTTPStatus(message: string): number | undefined {
  const match = message.match(/^HTTP\s+(\d{3})\b/i);
  return match ? Number(match[1]) : undefined;
}
