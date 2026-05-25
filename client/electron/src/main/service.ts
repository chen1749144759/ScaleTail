import { spawn } from "node:child_process";
import path from "node:path";
import { ServiceOverview, ServiceState } from "../shared/types";

const scaleTailService = "ScaleTail";
const dependencies = ["Dnscache", "iphlpsvc", "netprofm", "WinHttpAutoProxySvc"];

const stateMap: Record<number, ServiceState["state"]> = {
  1: "stopped",
  2: "start_pending",
  3: "stop_pending",
  4: "running",
};

export async function getServiceOverview(): Promise<ServiceOverview> {
  const [service, ...deps] = await Promise.all([scaleTailService, ...dependencies].map(queryService));
  return {
    service,
    dependencies: deps,
  };
}

export async function startScaleTailService(checkReady: () => Promise<void>): Promise<ServiceOverview> {
  const deps = await Promise.all(dependencies.map(ensureServiceRunning));
  await ensureServiceRunning(scaleTailService);

  const deadline = Date.now() + 20000;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      await checkReady();
      return getServiceOverview();
    } catch (err) {
      lastErr = err;
      await delay(700);
    }
  }

  const overview = await getServiceOverview();
  const blocked = deps.find((d) => d.state !== "running");
  if (blocked) {
    throw new Error(`依赖服务 ${blocked.name} 未就绪，请确认该服务未被禁用并以管理员身份启动。`);
  }
  throw new Error(`ScaleTail 服务已尝试启动，但 LocalAPI 仍不可用: ${stringifyError(lastErr)}`);
}

export async function queryService(name: string): Promise<ServiceState> {
  const result = await runSC(["queryex", name], 8000);
  const raw = result.stdout + result.stderr;
  if (result.code !== 0) {
    return {
      name,
      exists: false,
      state: raw.includes("1060") ? "missing" : "unknown",
      raw,
    };
  }
  const stateCode = parseStateCode(raw);
  return {
    name,
    exists: true,
    state: stateCode ? stateMap[stateCode] || "unknown" : "unknown",
    code: stateCode,
    raw,
  };
}

async function ensureServiceRunning(name: string): Promise<ServiceState> {
  const current = await queryService(name);
  if (current.state === "running") {
    return current;
  }
  if (!current.exists) {
    throw new Error(name === scaleTailService ? "ScaleTail 服务未安装，请重新运行安装包。" : `依赖服务 ${name} 不存在。`);
  }

  const start = await runSC(["start", name], 12000);
  if (start.code !== 0 && !`${start.stdout}\n${start.stderr}`.includes("1056")) {
    throw new Error(`无法启动服务 ${name}: ${start.stderr || start.stdout || `退出码 ${start.code}`}`);
  }

  const deadline = Date.now() + 15000;
  let latest = current;
  while (Date.now() < deadline) {
    latest = await queryService(name);
    if (latest.state === "running") {
      return latest;
    }
    await delay(600);
  }
  return latest;
}

function runSC(args: string[], timeoutMS: number): Promise<{ code: number; stdout: string; stderr: string }> {
  const systemRoot = process.env.SystemRoot || "C:\\Windows";
  const sc = path.join(systemRoot, "System32", "sc.exe");
  return new Promise((resolve) => {
    const child = spawn(sc, args, { windowsHide: true });
    const out: Buffer[] = [];
    const err: Buffer[] = [];
    const timer = setTimeout(() => {
      child.kill();
    }, timeoutMS);
    child.stdout.on("data", (chunk) => out.push(Buffer.from(chunk)));
    child.stderr.on("data", (chunk) => err.push(Buffer.from(chunk)));
    child.on("close", (code) => {
      clearTimeout(timer);
      resolve({
        code: code ?? -1,
        stdout: Buffer.concat(out).toString("utf8"),
        stderr: Buffer.concat(err).toString("utf8"),
      });
    });
    child.on("error", (error) => {
      clearTimeout(timer);
      resolve({ code: -1, stdout: "", stderr: error.message });
    });
  });
}

function parseStateCode(raw: string): number | undefined {
  const direct = raw.match(/STATE\s*:\s*(\d+)/i);
  if (direct) {
    return Number(direct[1]);
  }
  const loose = raw.match(/:\s*(\d+)\s+(?:STOPPED|START_PENDING|STOP_PENDING|RUNNING)/i);
  return loose ? Number(loose[1]) : undefined;
}

function stringifyError(err: unknown): string {
  return err instanceof Error ? err.message : String(err || "未知错误");
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
