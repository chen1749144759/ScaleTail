// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import { createHash, createPublicKey, randomBytes, verify as verifySignature } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { createWriteStream } from "node:fs";
import { copyFile, mkdtemp, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { once } from "node:events";
import { app, BrowserWindow, screen } from "electron";
import {
  acknowledgeUpdateResume,
  applyUpdatePolicy,
  bundledBinaryPath,
  fetchScaleForgeClientUpdate,
  getPrefs,
  getUpdatePolicyStatus,
  installSignedUpdate,
  type SignedUpdatePolicy,
  type UpdatePolicyStatus,
} from "./localapi";
import type { ClientUpdateInfo } from "../shared/types";

interface UpdateResponse {
  code?: number;
  data?: ClientUpdateInfo;
}

interface RelauncherHandle {
  child: ChildProcess;
  directory: string;
}

interface UpdateCheckerOptions {
  onForcedUpdate?: (info: ClientUpdateInfo) => Promise<boolean>;
  onForcedUpdateCleared?: (resumeNetwork: boolean) => Promise<void>;
}

interface CachedForcedUpdate {
  schema: 1;
  resumeNetwork: boolean;
  info: ClientUpdateInfo;
}

type UpdatePhase = "idle" | "downloading" | "verifying" | "installing" | "error";

const otaPublicKeyBase64 = "vLGmMjFWFdcyPurQt1EZ1cDZgY4FcroH4aRMfDpEP2o=";
const defaultIntervalMS = 10 * 60_000;
const initialDelayMS = 1_500;
const forcedReminderMS = 60_000;
const forcedActionRetryMS = 10_000;
const downloadTimeoutMS = 15 * 60_000;
const maxInstallerSize = 1024 * 1024 * 1024;
const maxUpdateResponseSize = 1024 * 1024;
const maxUpdateCacheSize = 64 * 1024;

let timer: NodeJS.Timeout | undefined;
let initialTimer: NodeJS.Timeout | undefined;
let forcedReminderTimer: NodeJS.Timeout | undefined;
let forcedActionRetryTimer: NodeJS.Timeout | undefined;
let updateWindow: BrowserWindow | undefined;
let latestInfo: ClientUpdateInfo | undefined;
let checking = false;
let lastSuggestedVersion = "";
let updatePhase: UpdatePhase = "idle";
let updateProgress = 0;
let updateMessage = "";
let updateCheckerOptions: UpdateCheckerOptions = {};
let forcedResumeNetwork = false;
let forcedActionVersion = "";
let allowUpdateWindowClose = false;

export async function startClientUpdateChecker(
  options: UpdateCheckerOptions = {},
  intervalMS = defaultIntervalMS,
): Promise<() => void> {
  updateCheckerOptions = options;
  await restoreCachedForcedUpdate();
  timer = setInterval(() => void checkClientUpdate(), intervalMS);
  initialTimer = setTimeout(() => void checkClientUpdate(), initialDelayMS);

  return () => {
    if (initialTimer) {
      clearTimeout(initialTimer);
      initialTimer = undefined;
    }
    if (timer) {
      clearInterval(timer);
      timer = undefined;
    }
    clearForcedReminder();
    clearForcedActionRetry();
    allowUpdateWindowClose = true;
    closeUpdateWindow(true);
  };
}

export function showRequiredClientUpdate(): boolean {
  if (!latestInfo?.forced) {
    return false;
  }
  showUpdateWindow(latestInfo);
  return true;
}

export function assertClientUpdateAllowed(): void {
  if (latestInfo?.forced) {
    showUpdateWindow(latestInfo);
    throw new Error("当前版本已被标记为强制更新，安装新版本后才能继续操作。");
  }
}

async function checkClientUpdate(): Promise<void> {
  if (checking || isUpdating()) {
    return;
  }
  checking = true;
  try {
    const response = await fetchClientUpdate();
    const info = response.data;
    if (!info || !hasSignedPolicy(info)) {
      const status = await getUpdatePolicyStatus();
      if (status.active) {
        await activateForcedUpdate(infoFromPolicyStatus(status), status);
      }
      return;
    }
    const normalized = normalizeInfo(info);
    validatePolicyMetadata(normalized);
    const status = await applyUpdatePolicy(policyFromInfo(normalized));
    resetUpdateState();
    if (status.active) {
      await activateForcedUpdate(normalized, status);
      return;
    }
    await clearForcedUpdateState(status.resume_pending || forcedResumeNetwork);
    if (normalized.has_update && normalized.update_type === "suggested") {
      validateReleaseMetadata(normalized);
      latestInfo = normalized;
      const version = normalized.version || "";
      if (version && version !== lastSuggestedVersion) {
        lastSuggestedVersion = version;
        showUpdateWindow(normalized);
      }
    } else {
      latestInfo = undefined;
    }
  } catch (err) {
    console.warn("ScaleTail client update check failed:", err);
  } finally {
    checking = false;
  }
}

async function fetchClientUpdate(): Promise<UpdateResponse> {
  let currentRevision = 0;
  try {
    currentRevision = Number((await getUpdatePolicyStatus()).policy?.policy_revision || 0);
  } catch {
    // The public endpoint can still bootstrap policy state if LocalAPI is
    // temporarily starting.
  }
  let directError: unknown;
  try {
    const prefs = await getPrefs();
    const controlURL = String(prefs.ControlURL || "").trim();
    if (controlURL) {
      return await fetchPublicClientUpdate(controlURL, currentRevision);
    }
  } catch (err) {
    directError = err;
  }

  try {
    return await fetchScaleForgeClientUpdate<UpdateResponse>(app.getVersion(), platformName(), currentRevision);
  } catch (err) {
    throw directError || err;
  }
}

async function fetchPublicClientUpdate(controlURL: string, currentRevision: number): Promise<UpdateResponse> {
  const origin = validatedControlServerURL(controlURL);
  const query = new URLSearchParams({
    current_version: app.getVersion(),
    platform: platformName(),
    current_revision: String(currentRevision),
  });
  const target = new URL(`/scaletail/v1/client-update?${query.toString()}`, origin);
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20_000);
  try {
    const response = await fetch(target, {
      method: "GET",
      redirect: "manual",
      signal: controller.signal,
      headers: {
        Accept: "application/json",
        "Cache-Control": "no-cache",
      },
    });
    if (!response.ok) {
      throw new Error(`公开更新检查失败：HTTP ${response.status}`);
    }
    const raw = await readLimitedResponse(response, maxUpdateResponseSize);
    const parsed = JSON.parse(raw.toString("utf8")) as UpdateResponse;
    if (!parsed || typeof parsed !== "object" || !parsed.data || typeof parsed.data !== "object") {
      throw new Error("公开更新接口返回格式无效");
    }
    return parsed;
  } finally {
    clearTimeout(timeout);
  }
}

async function readLimitedResponse(response: Response, limit: number): Promise<Buffer> {
  if (!response.body) {
    throw new Error("更新接口返回空响应");
  }
  const declared = Number(response.headers.get("content-length") || 0);
  if (Number.isFinite(declared) && declared > limit) {
    throw new Error("更新接口响应过大");
  }
  const chunks: Buffer[] = [];
  let total = 0;
  for await (const chunk of response.body as unknown as AsyncIterable<Uint8Array>) {
    const buffer = Buffer.from(chunk);
    total += buffer.length;
    if (total > limit) {
      throw new Error("更新接口响应过大");
    }
    chunks.push(buffer);
  }
  return Buffer.concat(chunks, total);
}

async function activateForcedUpdate(info: ClientUpdateInfo, knownStatus?: UpdatePolicyStatus): Promise<void> {
  latestInfo = info;
  showUpdateWindow(info);
  scheduleForcedReminder();

  const actionID = String(info.policy_revision || 0);
  if (forcedActionVersion !== actionID) {
    let plannedResume = forcedResumeNetwork;
    if (!plannedResume) {
      try {
        plannedResume = Boolean((await getPrefs()).WantRunning);
      } catch (err) {
        console.warn("ScaleTail could not read network state before a forced update:", err);
      }
    }

    // Persist the recovery intent before pausing the tunnel. A crash or disk
    // error must never leave the machine offline without enough state to
    // resume after the forced release is withdrawn.
    await saveForcedUpdateCache(info, plannedResume);
    try {
      const status = knownStatus || await applyUpdatePolicy(policyFromInfo(info));
      if (!status.active) {
        await clearForcedUpdateState(status.resume_pending || plannedResume);
        return;
      }
      forcedResumeNetwork = status.resume_pending || plannedResume || forcedResumeNetwork;
      forcedResumeNetwork = (await updateCheckerOptions.onForcedUpdate?.(info)) || forcedResumeNetwork;
      forcedActionVersion = actionID;
      clearForcedActionRetry();
    } catch (err) {
      console.warn("ScaleTail failed to suspend the network for a forced update:", err);
      scheduleForcedActionRetry(info);
    }
  }
  await saveForcedUpdateCache(info, forcedResumeNetwork);
}

async function restoreCachedForcedUpdate(): Promise<void> {
  let cached: CachedForcedUpdate | undefined;
  try {
    const cachePath = forcedUpdateCachePath();
    const info = await stat(cachePath);
    if (!info.isFile() || info.size <= 0 || info.size > maxUpdateCacheSize) {
      await clearForcedUpdateCache();
    } else {
      const value = JSON.parse(await readFile(cachePath, "utf8")) as CachedForcedUpdate;
      if (value?.schema === 1 && value.info && typeof value.info === "object") {
        cached = value;
      } else {
        await clearForcedUpdateCache();
      }
    }
  } catch (err) {
    if ((err as NodeJS.ErrnoException)?.code !== "ENOENT") {
      console.warn("ScaleTail forced update cache was rejected:", err);
      await clearForcedUpdateCache();
    }
  }

  try {
    const status = await getUpdatePolicyStatus();
    if (status.active) {
      let info = infoFromPolicyStatus(status);
      if (cached) {
        const normalized = normalizeInfo(cached.info);
        if (normalized.policy_revision === status.policy.policy_revision) {
          validatePolicyMetadata(normalized);
          info = normalized;
        }
      }
      forcedResumeNetwork = status.resume_pending || Boolean(cached?.resumeNetwork);
      await activateForcedUpdate(info, status);
      return;
    }
    await clearForcedUpdateState(status.resume_pending || Boolean(cached?.resumeNetwork));
    return;
  } catch (err) {
    console.warn("ScaleTail daemon update policy could not be restored:", err);
  }

  if (cached) {
    try {
      const normalized = normalizeInfo(cached.info);
      validatePolicyMetadata(normalized);
      if (normalized.forced && isNewerVersion(normalized.version || "", app.getVersion())) {
        forcedResumeNetwork = Boolean(cached.resumeNetwork);
        await activateForcedUpdate(normalized);
      }
    } catch (err) {
      console.warn("ScaleTail cached forced policy was rejected:", err);
      await clearForcedUpdateCache();
    }
  }
}

async function saveForcedUpdateCache(info: ClientUpdateInfo, resumeNetwork: boolean): Promise<void> {
  const value: CachedForcedUpdate = {
    schema: 1,
    resumeNetwork,
    info,
  };
  const encoded = JSON.stringify(value);
  if (Buffer.byteLength(encoded, "utf8") > maxUpdateCacheSize) {
    throw new Error("强制更新元数据过大，无法安全缓存");
  }
  const target = forcedUpdateCachePath();
  const temporary = `${target}.${process.pid}.${randomBytes(6).toString("hex")}.tmp`;
  try {
    await writeFile(temporary, encoded, { encoding: "utf8", mode: 0o600, flush: true });
    await rename(temporary, target);
  } finally {
    await rm(temporary, { force: true }).catch(() => undefined);
  }
}

async function clearForcedUpdateState(resumeNetwork = forcedResumeNetwork): Promise<void> {
  latestInfo = undefined;
  forcedResumeNetwork = false;
  forcedActionVersion = "";
  clearForcedReminder();
  clearForcedActionRetry();
  allowUpdateWindowClose = true;
  closeUpdateWindow(true);
  allowUpdateWindowClose = false;
  await clearForcedUpdateCache();
  if (resumeNetwork) {
    try {
      await updateCheckerOptions.onForcedUpdateCleared?.(true);
      await acknowledgeUpdateResume();
    } catch (err) {
      console.warn("ScaleTail failed to resume after a cleared forced update:", err);
    }
  }
}

async function clearForcedUpdateCache(): Promise<void> {
  await rm(forcedUpdateCachePath(), { force: true }).catch(() => undefined);
}

function forcedUpdateCachePath(): string {
  return path.join(app.getPath("userData"), "forced-client-update.json");
}

function hasSignedPolicy(info: ClientUpdateInfo): boolean {
  const action = String(info.update_type || "").trim().toLowerCase();
  return Number.isSafeInteger(Number(info.policy_revision))
    && Number(info.policy_revision) > 0
    && (action === "suggested" || action === "forced" || action === "clear")
    && String(info.signature || "").startsWith("v3.");
}

function policyFromInfo(info: ClientUpdateInfo): SignedUpdatePolicy {
  const action = String(info.update_type || "").toLowerCase();
  if (action !== "suggested" && action !== "forced" && action !== "clear") {
    throw new Error("更新策略动作无效");
  }
  return {
    policy_revision: Number(info.policy_revision || 0),
    update_type: action,
    version: canonicalVersion(info.version || "") || String(info.version || "").trim(),
    platform: String(info.platform || "").trim().toLowerCase(),
    sha256: String(info.sha256 || "").trim().toLowerCase(),
    file_size: Number(info.file_size || 0),
    download_url: String(info.download_url || "").trim(),
    signature: String(info.signature || "").trim(),
  };
}

function infoFromPolicyStatus(status: UpdatePolicyStatus): ClientUpdateInfo {
  const policy = status.policy;
  return normalizeInfo({
    ...policy,
    has_update: status.active && isNewerVersion(policy.version || "", app.getVersion()),
    forced: status.active,
    title: `ScaleTail ${policy.version} 强制更新`,
    description: "守护进程已验证强制更新策略，安装签名版本后才能恢复连接。",
  });
}

function normalizeInfo(info: ClientUpdateInfo): ClientUpdateInfo {
  const updateType = String(info.update_type || "suggested").toLowerCase();
  const rawVersion = String(info.version || "").trim();
  return {
    ...info,
    has_update: Boolean(info.has_update),
    policy_revision: Number(info.policy_revision || 0),
    version: canonicalVersion(rawVersion) || rawVersion,
    platform: String(info.platform || "").trim().toLowerCase(),
    download_url: String(info.download_url || "").trim(),
    sha256: String(info.sha256 || "").trim().toLowerCase(),
    signature: String(info.signature || "").trim(),
    file_size: Number(info.file_size || 0),
    update_type: updateType,
    forced: updateType === "forced",
    title: String(info.title || "").trim().slice(0, 200) || (
      updateType === "clear" ? "强制更新策略已解除" : `发现 ScaleTail ${info.version || "新版本"}`
    ),
    description: String(info.description || "").trim().slice(0, 1000) || (
      updateType === "forced"
        ? "该版本被标记为强制更新，完成更新前客户端会持续提醒。"
        : updateType === "clear"
          ? "守护进程已验证并应用新的更新放行策略。"
          : "建议安装新版本以获得最新功能和修复。"
    ),
    release_notes: String(info.release_notes || "").trim().slice(0, 20_000),
  };
}

function showUpdateWindow(info: ClientUpdateInfo): void {
  if (updateWindow && !updateWindow.isDestroyed()) {
    renderUpdateWindow(info);
    placeWindow(updateWindow, info);
    updateWindow.show();
    updateWindow.focus();
    return;
  }

  updateWindow = new BrowserWindow({
    width: 430,
    height: info.forced ? 360 : 340,
    show: false,
    frame: false,
    transparent: true,
    resizable: false,
    maximizable: false,
    minimizable: false,
    fullscreenable: false,
    alwaysOnTop: true,
    skipTaskbar: true,
    hasShadow: false,
    backgroundColor: "#00000000",
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
      webviewTag: false,
    },
  });

  updateWindow.on("close", (event) => {
    if (latestInfo?.forced && !allowUpdateWindowClose) {
      event.preventDefault();
      updateWindow?.show();
      updateWindow?.focus();
    }
  });
  updateWindow.on("closed", () => {
    updateWindow = undefined;
  });
  updateWindow.webContents.on("will-navigate", (event, url) => {
    event.preventDefault();
    void handleAction(url);
  });
  updateWindow.webContents.setWindowOpenHandler(({ url }) => {
    void handleAction(url);
    return { action: "deny" };
  });
  updateWindow.once("ready-to-show", () => {
    if (!updateWindow || updateWindow.isDestroyed()) {
      return;
    }
    placeWindow(updateWindow, info);
    updateWindow.show();
    updateWindow.focus();
  });
  renderUpdateWindow(info);
}

function renderUpdateWindow(info: ClientUpdateInfo): void {
  if (!updateWindow || updateWindow.isDestroyed()) {
    return;
  }
  updateWindow.loadURL(updateDataURL(info)).catch(() => undefined);
}

async function handleAction(url: string): Promise<void> {
  if (!url.startsWith("scaletail-update://")) {
    return;
  }
  const action = url.replace("scaletail-update://", "").replace(/\/$/, "");
  if (action === "download") {
    if (!latestInfo || isUpdating()) {
      return;
    }
    if (!hasOTAMetadata(latestInfo)) {
      setUpdateState("error", 0, "更新信息缺少完整签名元数据，已拒绝打开外部下载地址。");
      return;
    }
    await performOTAUpdate(latestInfo);
    return;
  }
  if (action === "later" && !latestInfo?.forced && !isUpdating()) {
    closeUpdateWindow();
    return;
  }
  if (action === "quit" && !isUpdating()) {
    allowUpdateWindowClose = true;
    app.quit();
  }
}

async function performOTAUpdate(info: ClientUpdateInfo): Promise<void> {
  let downloadDir = "";
  let relauncher: RelauncherHandle | undefined;
  try {
    // Verify the signed v3 policy before creating files or requesting the installer.
    const downloadURL = validateReleaseMetadata(info);
    setUpdateState("downloading", 0, "正在下载安装包...");
    const downloaded = await downloadInstaller(info, downloadURL, (progress) => {
      setUpdateState("downloading", progress, `正在下载安装包 ${progress}%`);
    });
    downloadDir = downloaded.directory;
    setUpdateState("verifying", 100, "正在校验安装包签名...");
    verifyManifest(info, downloaded.sha256, downloaded.fileSize);

    const markerID = randomBytes(16).toString("hex");
    relauncher = await spawnRelauncher(markerID);
    setUpdateState("installing", 100, "校验通过，正在静默覆盖安装...");
    await installSignedUpdate({
      installer_path: downloaded.filePath,
      policy_revision: Number(info.policy_revision || 0),
      update_type: info.update_type === "forced" ? "forced" : "suggested",
      version: info.version || "",
      platform: info.platform || platformName(),
      sha256: downloaded.sha256,
      file_size: downloaded.fileSize,
      download_url: info.download_url || "",
      signature: info.signature || "",
      marker_id: markerID,
    });
    await rm(downloadDir, { recursive: true, force: true });
    downloadDir = "";
    setUpdateState("installing", 100, "安装程序已接管，完成后将自动重新打开 ScaleTail。");
    setTimeout(() => {
      allowUpdateWindowClose = true;
      app.quit();
    }, 1200);
  } catch (err) {
    relauncher?.child.kill();
    if (relauncher?.directory) {
      await rm(relauncher.directory, { recursive: true, force: true }).catch(() => undefined);
    }
    const message = err instanceof Error ? err.message : String(err);
    setUpdateState("error", 0, `自动更新失败：${message}`);
  } finally {
    if (downloadDir) {
      await rm(downloadDir, { recursive: true, force: true }).catch(() => undefined);
    }
  }
}

async function downloadInstaller(
  info: ClientUpdateInfo,
  downloadURL: URL,
  onProgress: (progress: number) => void,
): Promise<{ directory: string; filePath: string; sha256: string; fileSize: number }> {
  const expectedSize = Number(info.file_size || 0);
  if (!Number.isSafeInteger(expectedSize) || expectedSize <= 0 || expectedSize > maxInstallerSize) {
    throw new Error("安装包大小元数据无效");
  }

  const directory = await mkdtemp(path.join(tmpdir(), "scaletail-ota-"));
  const filePath = path.join(directory, `ScaleTail-${safeFilePart(info.version || "update")}.exe`);
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), downloadTimeoutMS);
  const output = createWriteStream(filePath, { flags: "wx" });
  const hash = createHash("sha256");
  let received = 0;
  try {
    const response = await fetchHTTPSInstaller(downloadURL, controller.signal);
    if (!response.ok || !response.body) {
      throw new Error(`下载安装包失败：HTTP ${response.status}`);
    }
    for await (const chunk of response.body as unknown as AsyncIterable<Uint8Array>) {
      const buffer = Buffer.from(chunk);
      received += buffer.length;
      if (received > expectedSize || received > maxInstallerSize) {
        throw new Error("安装包大小超过发布元数据");
      }
      hash.update(buffer);
      if (!output.write(buffer)) {
        await once(output, "drain");
      }
      onProgress(Math.min(99, Math.floor((received / expectedSize) * 100)));
    }
    await new Promise<void>((resolve, reject) => {
      output.once("error", reject);
      output.end(resolve);
    });
    if (received !== expectedSize) {
      throw new Error("安装包大小与发布元数据不一致");
    }
    return { directory, filePath, sha256: hash.digest("hex"), fileSize: received };
  } catch (err) {
    output.destroy();
    await rm(directory, { recursive: true, force: true }).catch(() => undefined);
    throw err;
  } finally {
    clearTimeout(timeout);
  }
}

function verifyManifest(info: ClientUpdateInfo, actualSHA256: string, actualSize: number): void {
  validateReleaseMetadata(info);
  const expectedSHA256 = String(info.sha256 || "").toLowerCase();
  if (actualSHA256 !== expectedSHA256 || actualSize !== Number(info.file_size || 0)) {
    throw new Error("安装包完整性校验失败");
  }
}

function validateReleaseMetadata(info: ClientUpdateInfo): URL {
  const downloadURL = validatePolicyMetadata(info);
  if (!isNewerVersion(info.version || "", app.getVersion())) {
    throw new Error("更新版本号无效或不高于当前版本");
  }
  if (info.update_type !== "suggested" && info.update_type !== "forced") {
    throw new Error("更新类型无效");
  }
  if (!downloadURL) {
    throw new Error("更新策略不包含安装包");
  }
  return downloadURL.url;
}

function validatePolicyMetadata(info: ClientUpdateInfo): NormalizedDownloadURL | undefined {
  const revision = Number(info.policy_revision || 0);
  if (!Number.isSafeInteger(revision) || revision <= 0) {
    throw new Error("更新策略 revision 无效");
  }
  const action = String(info.update_type || "").toLowerCase();
  if (action !== "suggested" && action !== "forced" && action !== "clear") {
    throw new Error("更新策略动作无效");
  }
  const version = canonicalVersion(info.version || "");
  if (!version) {
    throw new Error("更新策略版本号无效");
  }
  if (!platformMatches(info.platform || "", platformName())) {
    throw new Error("更新包平台与当前客户端不匹配");
  }
  const expectedSHA256 = String(info.sha256 || "").toLowerCase();
  const fileSize = Number(info.file_size || 0);
  let downloadURL: NormalizedDownloadURL | undefined;
  if (action === "clear") {
    if (expectedSHA256 || fileSize !== 0 || String(info.download_url || "").trim()) {
      throw new Error("解除策略不能包含安装包元数据");
    }
  } else {
    downloadURL = normalizeHTTPSDownloadURL(info.download_url || "");
    if (!/^[a-f0-9]{64}$/.test(expectedSHA256)) {
      throw new Error("安装包 SHA-256 元数据无效");
    }
    if (!Number.isSafeInteger(fileSize) || fileSize <= 0 || fileSize > maxInstallerSize) {
      throw new Error("安装包大小元数据无效");
    }
  }
  const publicKeyRaw = Buffer.from(otaPublicKeyBase64, "base64");
  const spkiPrefix = Buffer.from("302a300506032b6570032100", "hex");
  const publicKey = createPublicKey({ key: Buffer.concat([spkiPrefix, publicKeyRaw]), format: "der", type: "spki" });
  const signature = parseV3Signature(String(info.signature || ""));
  const message = otaMessage(
    revision,
    action,
    version,
    info.platform || platformName(),
    expectedSHA256,
    fileSize,
    downloadURL?.canonical || "",
  );
  if (!verifySignature(null, message, publicKey, signature)) {
    throw new Error("安装包发布签名无效");
  }
  return downloadURL;
}

async function spawnRelauncher(markerID: string): Promise<RelauncherHandle> {
  const directory = await mkdtemp(path.join(tmpdir(), "scaletail-relauncher-"));
  const helperPath = path.join(directory, `ScaleTailUpdateHelper-${markerID}.exe`);
  await copyFile(bundledBinaryPath("ScaleTailUpdateHelper.exe"), helperPath);
  const child = spawn(helperPath, [
    "wait",
    `--marker-id=${markerID}`,
    `--app=${process.execPath}`,
    "--timeout=5m",
  ], {
    detached: true,
    windowsHide: true,
    stdio: "ignore",
  });
  child.unref();
  return { child, directory };
}

export function otaMessage(
  revision: number,
  action: string,
  version: string,
  platform: string,
  sha256: string,
  fileSize: number,
  downloadURL: string,
): Buffer {
  return Buffer.from(
    `scaletail-update-v3\n${revision}\n${action.trim().toLowerCase()}\n${canonicalVersion(version) || version.trim()}\n${platform.trim().toLowerCase()}\n${sha256.trim().toLowerCase()}\n${fileSize}\n${downloadURL}\n`,
    "utf8",
  );
}

function hasOTAMetadata(info: ClientUpdateInfo): boolean {
  if (!app.isPackaged) {
    return false;
  }
  try {
    validateReleaseMetadata(info);
    return true;
  } catch {
    return false;
  }
}

interface NormalizedDownloadURL {
  canonical: string;
  url: URL;
}

export function normalizeHTTPSDownloadURL(raw: string, base?: URL): NormalizedDownloadURL {
  const value = String(raw || "").trim();
  if (!value || value.length > 2048 || value.endsWith("?") || !/^[\x21-\x7e]+$/.test(value)) {
    throw new Error("安装包下载地址无效");
  }
  let parsed: URL;
  try {
    parsed = base ? new URL(value, base) : new URL(value);
  } catch {
    throw new Error("安装包下载地址无效");
  }
  const host = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (parsed.protocol !== "https:" || !validDownloadHost(host) || parsed.username || parsed.password || parsed.hash) {
    throw new Error("安装包及所有重定向地址必须使用无凭据、无片段且非本地 DNS 主机名的 HTTPS URL");
  }
  const portNumber = parsed.port ? Number.parseInt(parsed.port, 10) : 443;
  const port = parsed.port && portNumber !== 443 ? `:${portNumber}` : "";
  const pathname = normalizeURLComponent(parsed.pathname || "/");
  const query = normalizeURLComponent(parsed.search.startsWith("?") ? parsed.search.slice(1) : parsed.search);
  const canonical = `https://${host}${port}${pathname}${query ? `?${query}` : ""}`;
  return { canonical, url: new URL(canonical) };
}

function validDownloadHost(host: string): boolean {
  if (!host || host.endsWith(".") || host === "localhost" || host.endsWith(".localhost")) {
    return false;
  }
  if (isIPLiteral(host) || !/^[a-z0-9.-]+$/.test(host)) {
    return false;
  }
  return host.split(".").every((label) => label.length > 0 && label.length <= 63 && !label.startsWith("-") && !label.endsWith("-"));
}

function isIPLiteral(host: string): boolean {
  if (host.includes(":")) {
    return true;
  }
  const parts = host.split(".");
  return parts.length === 4 && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

function normalizeURLComponent(value: string): string {
  let result = "";
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    const char = value[index];
    if (code < 0x21 || code > 0x7e || char === "\\") {
      throw new Error("安装包下载地址包含不安全字符");
    }
    if (char !== "%") {
      result += char;
      continue;
    }
    const hex = value.slice(index + 1, index + 3);
    if (!/^[0-9a-fA-F]{2}$/.test(hex)) {
      throw new Error("安装包下载地址包含无效转义");
    }
    const decoded = String.fromCharCode(Number.parseInt(hex, 16));
    result += /^[A-Za-z0-9\-._~]$/.test(decoded) ? decoded : `%${hex.toUpperCase()}`;
    index += 2;
  }
  return result;
}

function parseV3Signature(value: string): Buffer {
  const parts = value.trim().split(".");
  if (parts.length !== 2 || parts[0] !== "v3" || !/^[A-Za-z0-9+/]{86}==$/.test(parts[1])) {
    throw new Error("安装包发布签名必须使用 v3 信封");
  }
  const signature = Buffer.from(parts[1], "base64");
  if (signature.length !== 64) {
    throw new Error("安装包发布签名编码无效");
  }
  return signature;
}

function validatedControlServerURL(raw: string): URL {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new Error("控制服务器地址无效");
  }
  const secure = parsed.protocol === "https:";
  if ((!secure && !(parsed.protocol === "http:" && isLoopbackHost(parsed.hostname)))
    || !parsed.hostname
    || parsed.username
    || parsed.password) {
    throw new Error("更新检查要求 HTTPS 控制服务器，本机回环地址除外");
  }
  return new URL(parsed.origin);
}

async function fetchHTTPSInstaller(url: URL, signal: AbortSignal, redirects = 0): Promise<Response> {
  const response = await fetch(url, { redirect: "manual", signal });
  if (![301, 302, 303, 307, 308].includes(response.status)) {
    return response;
  }
  if (redirects >= 3) {
    throw new Error("安装包下载重定向次数过多");
  }
  const location = response.headers.get("location");
  if (!location) {
    throw new Error("安装包下载重定向缺少目标地址");
  }
  return fetchHTTPSInstaller(normalizeHTTPSDownloadURL(location, url).url, signal, redirects + 1);
}

function safeFilePart(value: string): string {
  return value.replace(/[^0-9A-Za-z._+-]/g, "_").slice(0, 64) || "update";
}

function setUpdateState(phase: UpdatePhase, progress: number, message: string): void {
  if (updatePhase === phase && updateProgress === progress && updateMessage === message) {
    return;
  }
  updatePhase = phase;
  updateProgress = progress;
  updateMessage = message;
  if (latestInfo) {
    renderUpdateWindow(latestInfo);
  }
}

function resetUpdateState(): void {
  updatePhase = "idle";
  updateProgress = 0;
  updateMessage = "";
}

function isUpdating(): boolean {
  return updatePhase === "downloading" || updatePhase === "verifying" || updatePhase === "installing";
}

function placeWindow(win: BrowserWindow, info: ClientUpdateInfo): void {
  const bounds = win.getBounds();
  const height = info.forced ? 360 : 340;
  const { workArea } = screen.getPrimaryDisplay();
  win.setBounds({
    x: workArea.x + workArea.width - bounds.width - 18,
    y: workArea.y + workArea.height - height - 18,
    width: bounds.width,
    height,
  });
}

function scheduleForcedReminder(): void {
  if (forcedReminderTimer) {
    return;
  }
  forcedReminderTimer = setInterval(() => {
    if (latestInfo?.forced && !isUpdating()) {
      showUpdateWindow(latestInfo);
    }
  }, forcedReminderMS);
}

function clearForcedReminder(): void {
  if (forcedReminderTimer) {
    clearInterval(forcedReminderTimer);
    forcedReminderTimer = undefined;
  }
}

function scheduleForcedActionRetry(info: ClientUpdateInfo): void {
  if (forcedActionRetryTimer) {
    return;
  }
  const actionID = String(info.policy_revision || 0);
  forcedActionRetryTimer = setTimeout(() => {
    forcedActionRetryTimer = undefined;
    if (latestInfo?.forced && String(latestInfo.policy_revision || 0) === actionID && forcedActionVersion !== actionID) {
      void activateForcedUpdate(latestInfo).catch((err) => {
        console.warn("ScaleTail forced update retry failed:", err);
        scheduleForcedActionRetry(info);
      });
    }
  }, forcedActionRetryMS);
}

function clearForcedActionRetry(): void {
  if (forcedActionRetryTimer) {
    clearTimeout(forcedActionRetryTimer);
    forcedActionRetryTimer = undefined;
  }
}

function closeUpdateWindow(force = false): void {
  if (latestInfo?.forced && !force) {
    showUpdateWindow(latestInfo);
    return;
  }
  if (updateWindow && !updateWindow.isDestroyed()) {
    updateWindow.close();
  }
  updateWindow = undefined;
}

function updateDataURL(info: ClientUpdateInfo): string {
  return `data:text/html;charset=utf-8,${encodeURIComponent(updateHTML(info))}`;
}

function updateHTML(info: ClientUpdateInfo): string {
  const forced = Boolean(info.forced);
  const version = escapeHTML(info.version || "");
  const title = escapeHTML(info.title || "发现 ScaleTail 新版本");
  const description = escapeHTML(info.description || "");
  const notes = escapeHTML(info.release_notes || "");
  const active = isUpdating();
  const canOTA = hasOTAMetadata(info);
  const downloadDisabled = !canOTA || active ? " disabled" : "";
  const tag = forced ? "强制更新" : "建议更新";
  const actionText = active ? "更新处理中" : (canOTA ? "立即更新" : "更新信息无效");
  const laterButton = forced
    ? `<a class="btn ghost danger${active ? " disabled" : ""}" href="scaletail-update://quit">退出客户端</a>`
    : `<a class="btn ghost${active ? " disabled" : ""}" href="scaletail-update://later">稍后提醒</a>`;
  const closeButton = forced || active ? "" : `<a class="close" href="scaletail-update://later" title="关闭">×</a>`;
  const progress = active
    ? `<div class="progress"><span style="width:${Math.max(4, updateProgress)}%"></span></div>`
    : "";
  const statusClass = updatePhase === "error" ? "status error" : "status";
  const status = updateMessage
    ? `<div class="${statusClass}">${escapeHTML(updateMessage)}</div>${progress}`
    : "";
  return `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; frame-ancestors 'none'; form-action 'none'">
  <style>
    :root { color-scheme: light; font-family: "Microsoft YaHei UI", "Segoe UI", system-ui, sans-serif; --text:#172033; --muted:#667085; --blue:#2563eb; --blue-strong:#1d4ed8; --red:#dc2626; --amber:#d97706; --line:rgba(128,145,170,.28); }
    * { box-sizing: border-box; }
    html, body { width:100%; height:100%; margin:0; background:transparent; overflow:hidden; }
    body { padding:10px; color:var(--text); }
    .card { position:relative; width:100%; height:100%; padding:18px; border:1px solid rgba(255,255,255,.64); border-radius:22px; background:rgba(246,249,253,.94); box-shadow:0 18px 46px rgba(24,39,75,.18); backdrop-filter:blur(24px) saturate(130%); clip-path:inset(0 round 22px); }
    .head { display:flex; align-items:center; gap:12px; padding-right:32px; }
    .logo { width:42px; height:42px; border-radius:12px; display:grid; place-items:center; color:#fff; font-weight:800; background:linear-gradient(135deg,#2563eb,#10b981); box-shadow:0 10px 22px rgba(37,99,235,.22); }
    .kicker { display:flex; align-items:center; gap:8px; margin-bottom:4px; color:var(--muted); font-size:12px; font-weight:700; }
    .tag { padding:2px 8px; border-radius:999px; font-size:12px; color:${forced ? "var(--red)" : "var(--amber)"}; background:${forced ? "rgba(255,241,242,.88)" : "rgba(255,247,232,.9)"}; }
    h1 { margin:0; font-size:19px; line-height:1.25; }
    p { margin:13px 0 0; color:var(--muted); font-size:13px; line-height:1.55; }
    .notes { margin-top:10px; max-height:64px; overflow:auto; padding:9px 10px; border:1px solid var(--line); border-radius:10px; color:#344054; background:rgba(255,255,255,.68); white-space:pre-wrap; font-size:12px; line-height:1.45; }
    .status { margin-top:9px; color:#2563eb; font-size:12px; line-height:1.4; }
    .status.error { color:var(--red); }
    .progress { height:4px; margin-top:6px; overflow:hidden; border-radius:4px; background:rgba(37,99,235,.12); }
    .progress span { display:block; height:100%; border-radius:4px; background:#2563eb; transition:width .18s ease; }
    .actions { position:absolute; left:18px; right:18px; bottom:18px; display:flex; justify-content:flex-end; gap:8px; }
    .btn { display:inline-flex; align-items:center; justify-content:center; height:36px; padding:0 14px; border:1px solid var(--line); border-radius:8px; color:var(--text); background:rgba(255,255,255,.66); text-decoration:none; font-size:13px; font-weight:700; }
    .btn.primary { border-color:var(--blue); color:#fff; background:var(--blue); }
    .btn.primary:hover { background:var(--blue-strong); }
    .btn.ghost:hover { background:rgba(255,255,255,.84); }
    .btn.danger { color:var(--red); }
    .disabled { opacity:.5; pointer-events:none; }
    .close { position:absolute; right:13px; top:12px; width:30px; height:30px; border-radius:8px; display:grid; place-items:center; color:var(--muted); text-decoration:none; font-size:20px; }
    .close:hover { color:#fff; background:var(--red); }
  </style>
</head>
<body>
  <section class="card">
    ${closeButton}
    <div class="head"><div class="logo">S</div><div><div class="kicker"><span>ScaleTail 更新</span><span class="tag">${tag}</span></div><h1>${title}</h1></div></div>
    <p>${description}</p>
    <div class="notes">${notes || `当前版本：${escapeHTML(app.getVersion())}${version ? `\n最新版本：${version}` : ""}`}</div>
    ${status}
    <div class="actions">${laterButton}<a class="btn primary${downloadDisabled}" href="scaletail-update://download">${actionText}</a></div>
  </section>
</body>
</html>`;
}

function platformName(): string {
  if (process.platform === "win32") {
    return process.arch === "arm64" ? "windows-arm64" : "windows-amd64";
  }
  if (process.platform === "darwin") {
    return process.arch === "arm64" ? "macos-arm64" : "macos-amd64";
  }
  if (process.platform === "linux") {
    return process.arch === "arm64" ? "linux-arm64" : "linux-amd64";
  }
  return `${process.platform}-${process.arch}`;
}

function platformMatches(releasePlatform: string, currentPlatform: string): boolean {
  const release = releasePlatform.trim().toLowerCase();
  const current = currentPlatform.trim().toLowerCase();
  return release !== "" && release === current;
}

interface ParsedSemver {
  core: [number, number, number];
  prerelease: string[];
}

function isNewerVersion(candidate: string, current: string): boolean {
  const left = parseSemver(candidate);
  const right = parseSemver(current);
  if (!left || !right) {
    return false;
  }
  for (let index = 0; index < left.core.length; index += 1) {
    if (left.core[index] !== right.core[index]) {
      return left.core[index] > right.core[index];
    }
  }
  return comparePrerelease(left.prerelease, right.prerelease) > 0;
}

function canonicalVersion(value: string): string | undefined {
  const clean = value.trim().replace(/^[vV]/, "");
  return parseSemver(clean) ? clean : undefined;
}

function parseSemver(value: string): ParsedSemver | undefined {
  const match = value.trim().match(
    /^[vV]?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/,
  );
  if (!match) {
    return undefined;
  }
  const prerelease = match[4] ? match[4].split(".") : [];
  if (prerelease.some((part) => /^0\d+$/.test(part))) {
    return undefined;
  }
  return {
    core: [Number(match[1]), Number(match[2]), Number(match[3])],
    prerelease,
  };
}

function comparePrerelease(left: string[], right: string[]): number {
  if (left.length === 0 || right.length === 0) {
    if (left.length === right.length) return 0;
    return left.length === 0 ? 1 : -1;
  }
  for (let index = 0; index < Math.min(left.length, right.length); index += 1) {
    const leftPart = left[index];
    const rightPart = right[index];
    if (leftPart === rightPart) continue;
    const leftNumeric = /^\d+$/.test(leftPart);
    const rightNumeric = /^\d+$/.test(rightPart);
    if (leftNumeric && rightNumeric) {
      return Number(leftPart) > Number(rightPart) ? 1 : -1;
    }
    if (leftNumeric !== rightNumeric) {
      return leftNumeric ? -1 : 1;
    }
    return leftPart > rightPart ? 1 : -1;
  }
  if (left.length === right.length) return 0;
  return left.length > right.length ? 1 : -1;
}

function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "").replace(/\.$/, "");
  if (host === "localhost" || host === "::1" || host === "0:0:0:0:0:0:0:1") {
    return true;
  }
  const parts = host.split(".");
  return parts.length === 4
    && parts[0] === "127"
    && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

function escapeHTML(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}
