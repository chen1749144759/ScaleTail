// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import fs from "node:fs";
import path from "node:path";
import { app } from "electron";
import type { ClientReportConfig } from "../shared/types";

const configFileName = "client-report.json";
const defaultIntervalSeconds = 15;

export function readClientReportConfig(): ClientReportConfig {
  const fromFile = readConfigFile();
  const config: ClientReportConfig = {
    enabled: fromFile.enabled ?? true,
    intervalSeconds: normalizeInterval(fromFile.intervalSeconds),
    flowEnabled: fromFile.flowEnabled ?? true,
    quotaGuardEnabled: fromFile.quotaGuardEnabled ?? true,
  };
  const legacy = fromFile as Record<string, unknown>;
  if ("token" in legacy || "baseURL" in legacy) {
    try {
      writeConfigFile(config);
    } catch (err) {
      console.warn("ScaleTail legacy report credential cleanup failed:", err);
    }
  }
  return config;
}

export function saveClientReportConfig(input: ClientReportConfig): ClientReportConfig {
  const next: ClientReportConfig = {
    enabled: Boolean(input.enabled),
    intervalSeconds: normalizeInterval(input.intervalSeconds),
    flowEnabled: Boolean(input.flowEnabled),
    quotaGuardEnabled: Boolean(input.quotaGuardEnabled),
  };

  writeConfigFile(next);
  return next;
}

function writeConfigFile(config: ClientReportConfig): void {
  const file = configPath();
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(config, null, 2), { encoding: "utf8" });
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
