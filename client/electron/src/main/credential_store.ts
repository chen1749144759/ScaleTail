// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import fs from "node:fs";
import path from "node:path";
import { app, safeStorage } from "electron";

export interface StoredAccountCredential {
  controlURL: string;
  username: string;
  password: string;
}

const fileName = "account-credential.json";
const credentialSchemaVersion = 2;
const accountPasswordControlCharacters = /[\u0000-\u001f\u007f-\u009f]/;

export function saveAccountCredential(credential: StoredAccountCredential): void {
  if (!safeStorage.isEncryptionAvailable()) {
    throw new Error("系统凭据加密不可用，无法安全保存账号密码");
  }
  const controlURL = canonicalCredentialControlURL(credential.controlURL);
  if (!controlURL) {
    throw new Error("无法保存账号密码：控制服务器地址无效");
  }
  const password = validateAccountPassword(credential.password);
  const encrypted = safeStorage.encryptString(JSON.stringify({
    controlURL,
    username: credential.username,
    password,
  }));
  const target = credentialPath();
  const temporary = `${target}.tmp`;
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(temporary, JSON.stringify({
    version: credentialSchemaVersion,
    encrypted: encrypted.toString("base64"),
  }), {
    encoding: "utf8",
    mode: 0o600,
  });
  fs.renameSync(temporary, target);
}

export function readAccountCredential(controlURL: string): StoredAccountCredential | undefined {
  try {
    if (!safeStorage.isEncryptionAvailable()) return undefined;
    const expectedControlURL = canonicalCredentialControlURL(controlURL);
    if (!expectedControlURL) return undefined;
    const stored = JSON.parse(fs.readFileSync(credentialPath(), "utf8")) as {
      version?: number;
      encrypted?: string;
    };
    if (stored.version !== credentialSchemaVersion || !stored.encrypted) return undefined;
    const value = JSON.parse(
      safeStorage.decryptString(Buffer.from(stored.encrypted, "base64")),
    ) as Partial<StoredAccountCredential>;
    const storedControlURL = canonicalCredentialControlURL(value.controlURL || "");
    if (storedControlURL !== expectedControlURL || !value.username || !value.password) return undefined;
    return {
      controlURL: storedControlURL,
      username: value.username,
      password: validateAccountPassword(value.password),
    };
  } catch {
    return undefined;
  }
}

export function canonicalCredentialControlURL(raw: string): string | undefined {
  try {
    const parsed = new URL(String(raw || "").trim());
    if ((parsed.protocol !== "https:" && parsed.protocol !== "http:")
      || !parsed.hostname
      || parsed.username
      || parsed.password
      || parsed.pathname !== "/"
      || parsed.search
      || parsed.hash) {
      return undefined;
    }
    return parsed.origin;
  } catch {
    return undefined;
  }
}

export function validateAccountPassword(password: string): string {
  if (!password
    || Buffer.byteLength(password, "utf8") > 72
    || accountPasswordControlCharacters.test(password)) {
    throw new Error("请输入有效密码，长度不能超过 72 字节，且不能包含控制字符");
  }
  return password;
}

export function clearAccountCredential(): void {
  try {
    fs.rmSync(credentialPath(), { force: true });
  } catch {
    // Missing or already-removed credentials need no user-facing error.
  }
}

function credentialPath(): string {
  return path.join(app.getPath("userData"), fileName);
}
