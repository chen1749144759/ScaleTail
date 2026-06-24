import fs from "node:fs";
import path from "node:path";
import { app } from "electron";
import type { ClientReportConfig } from "../shared/types";

const configFileName = "client-report.json";
const bundledConfigFileName = "report-config.json";
const defaultIntervalSeconds = 15;

export function readClientReportConfig(): ClientReportConfig {
  const fromFile = readConfigFile();
  const fromBundle = readBundledConfigFile();
  const envBaseURL = process.env.SCALETAIL_REPORT_URL || "";
  const envToken = process.env.SCALETAIL_CLIENT_TOKEN || "";
  const defaultEnabled =
    Boolean(envBaseURL && envToken) || Boolean(fromBundle.enabled ?? (fromBundle.baseURL && fromBundle.token));

  return {
    enabled: fromFile.enabled ?? defaultEnabled,
    baseURL: fromFile.baseURL || envBaseURL || fromBundle.baseURL || "",
    token: fromFile.token || envToken || fromBundle.token || "",
    intervalSeconds: normalizeInterval(fromFile.intervalSeconds ?? fromBundle.intervalSeconds),
    flowEnabled: fromFile.flowEnabled ?? fromBundle.flowEnabled ?? true,
    quotaGuardEnabled: fromFile.quotaGuardEnabled ?? fromBundle.quotaGuardEnabled ?? true,
  };
}

export function saveClientReportConfig(input: ClientReportConfig): ClientReportConfig {
  const next: ClientReportConfig = {
    enabled: Boolean(input.enabled),
    baseURL: String(input.baseURL || "").trim(),
    token: String(input.token || "").trim(),
    intervalSeconds: normalizeInterval(input.intervalSeconds),
    flowEnabled: Boolean(input.flowEnabled),
    quotaGuardEnabled: Boolean(input.quotaGuardEnabled),
  };
  if (next.enabled && (!next.baseURL || !next.token)) {
    throw new Error("启用上报时必须填写管理平台地址和上报密钥。");
  }
  if (next.baseURL && !/^https?:\/\//i.test(next.baseURL)) {
    throw new Error("管理平台地址必须以 http:// 或 https:// 开头。");
  }

  const file = configPath();
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(next, null, 2), { encoding: "utf8" });
  return next;
}

function normalizeInterval(value: unknown): number {
  const n = Math.round(Number(value || defaultIntervalSeconds));
  if (!Number.isFinite(n)) {
    return defaultIntervalSeconds;
  }
  return Math.max(5, Math.min(3600, n));
}

function readConfigFile(): Partial<ClientReportConfig> {
  return readJSONConfig(configPath());
}

function readBundledConfigFile(): Partial<ClientReportConfig> {
  const resourcesPath = (process as typeof process & { resourcesPath?: string }).resourcesPath;
  const candidates = [
    resourcesPath ? path.join(resourcesPath, "app", "resources", bundledConfigFileName) : "",
    path.join(app.getAppPath(), "resources", bundledConfigFileName),
  ].filter(Boolean);

  for (const file of [...new Set(candidates)]) {
    const parsed = readJSONConfig(file);
    if (Object.keys(parsed).length > 0) {
      return parsed;
    }
  }
  return {};
}

function readJSONConfig(file: string): Partial<ClientReportConfig> {
  try {
    const raw = fs.readFileSync(file, "utf8");
    const parsed = JSON.parse(raw) as Partial<ClientReportConfig>;
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

function configPath(): string {
  return path.join(app.getPath("userData"), configFileName);
}
