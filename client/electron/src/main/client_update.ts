import { createHash, createPublicKey, randomBytes, verify as verifySignature } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { createWriteStream } from "node:fs";
import { copyFile, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { once } from "node:events";
import { app, BrowserWindow, screen, shell } from "electron";
import { bundledBinaryPath, installSignedUpdate } from "./localapi";
import { readClientReportConfig } from "./report_config";
import type { ClientUpdateInfo } from "../shared/types";

interface UpdateResponse {
  code?: number;
  data?: ClientUpdateInfo;
}

interface RelauncherHandle {
  child: ChildProcess;
  directory: string;
}

type UpdatePhase = "idle" | "downloading" | "verifying" | "installing" | "error";

const otaPublicKeyBase64 = "vLGmMjFWFdcyPurQt1EZ1cDZgY4FcroH4aRMfDpEP2o=";
const defaultIntervalMS = 10 * 60_000;
const initialDelayMS = 12_000;
const forcedReminderMS = 60_000;
const requestTimeoutMS = 10_000;
const downloadTimeoutMS = 15 * 60_000;
const maxInstallerSize = 1024 * 1024 * 1024;

let timer: NodeJS.Timeout | undefined;
let initialTimer: NodeJS.Timeout | undefined;
let forcedReminderTimer: NodeJS.Timeout | undefined;
let updateWindow: BrowserWindow | undefined;
let latestInfo: ClientUpdateInfo | undefined;
let checking = false;
let lastSuggestedVersion = "";
let updatePhase: UpdatePhase = "idle";
let updateProgress = 0;
let updateMessage = "";

export function startClientUpdateChecker(intervalMS = defaultIntervalMS): () => void {
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
    closeUpdateWindow();
  };
}

async function checkClientUpdate(): Promise<void> {
  if (checking || isUpdating()) {
    return;
  }
  const config = readClientReportConfig();
  const baseURL = config.baseURL.trim();
  const token = config.token.trim();
  if (!config.enabled || !baseURL || !token) {
    return;
  }

  checking = true;
  try {
    const params = new URLSearchParams({
      current_version: app.getVersion(),
      platform: platformName(),
    });
    const url = `${endpoint(baseURL, "/client-update")}?${params.toString()}`;
    const response = await fetchJSON(url, token);
    const info = response.data;
    if (!info?.has_update) {
      clearForcedReminder();
      closeUpdateWindow();
      latestInfo = undefined;
      return;
    }
    latestInfo = normalizeInfo(info);
    resetUpdateState();
    if (latestInfo.forced) {
      showUpdateWindow(latestInfo);
      scheduleForcedReminder();
      return;
    }
    clearForcedReminder();
    const version = latestInfo.version || "";
    if (version && version !== lastSuggestedVersion) {
      lastSuggestedVersion = version;
      showUpdateWindow(latestInfo);
    }
  } catch (err) {
    console.warn("ScaleTail client update check failed:", err);
  } finally {
    checking = false;
  }
}

async function fetchJSON(url: string, token: string): Promise<UpdateResponse> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), requestTimeoutMS);
  try {
    const res = await fetch(url, {
      headers: { "X-ScaleTail-Token": token },
      signal: controller.signal,
    });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    return await res.json() as UpdateResponse;
  } finally {
    clearTimeout(timeout);
  }
}

function normalizeInfo(info: ClientUpdateInfo): ClientUpdateInfo {
  const updateType = String(info.update_type || "suggested").toLowerCase();
  return {
    ...info,
    version: String(info.version || "").trim(),
    platform: String(info.platform || "").trim().toLowerCase(),
    sha256: String(info.sha256 || "").trim().toLowerCase(),
    signature: String(info.signature || "").trim(),
    file_size: Number(info.file_size || 0),
    update_type: updateType,
    forced: Boolean(info.forced || updateType === "forced"),
    title: info.title?.trim() || `发现 ScaleTail ${info.version || "新版本"}`,
    description: info.description?.trim() || (
      updateType === "forced"
        ? "该版本被标记为强制更新，完成更新前客户端会持续提醒。"
        : "建议安装新版本以获得最新功能和修复。"
    ),
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
    },
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
    if (/^https?:\/\//i.test(url)) {
      await shell.openExternal(url);
    }
    return;
  }
  const action = url.replace("scaletail-update://", "").replace(/\/$/, "");
  if (action === "download") {
    if (!latestInfo || isUpdating()) {
      return;
    }
    if (!hasOTAMetadata(latestInfo)) {
      const downloadURL = latestInfo.download_url || "";
      if (/^https?:\/\//i.test(downloadURL)) {
        await shell.openExternal(downloadURL);
      }
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
    app.quit();
  }
}

async function performOTAUpdate(info: ClientUpdateInfo): Promise<void> {
  let downloadDir = "";
  let relauncher: RelauncherHandle | undefined;
  try {
    setUpdateState("downloading", 0, "正在下载安装包...");
    const downloaded = await downloadInstaller(info, (progress) => {
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
      version: info.version || "",
      platform: info.platform || platformName(),
      sha256: downloaded.sha256,
      file_size: downloaded.fileSize,
      signature: info.signature || "",
      marker_id: markerID,
    });
    await rm(downloadDir, { recursive: true, force: true });
    downloadDir = "";
    setUpdateState("installing", 100, "安装程序已接管，完成后将自动重新打开 ScaleTail。");
    setTimeout(() => app.quit(), 1200);
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
  onProgress: (progress: number) => void,
): Promise<{ directory: string; filePath: string; sha256: string; fileSize: number }> {
  const downloadURL = new URL(info.download_url || "");
  if (downloadURL.protocol !== "http:" && downloadURL.protocol !== "https:") {
    throw new Error("下载地址必须使用 HTTP 或 HTTPS");
  }
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
    const response = await fetch(downloadURL, { redirect: "follow", signal: controller.signal });
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
  const expectedSHA256 = String(info.sha256 || "").toLowerCase();
  if (actualSHA256 !== expectedSHA256 || actualSize !== Number(info.file_size || 0)) {
    throw new Error("安装包完整性校验失败");
  }
  const publicKeyRaw = Buffer.from(otaPublicKeyBase64, "base64");
  const spkiPrefix = Buffer.from("302a300506032b6570032100", "hex");
  const publicKey = createPublicKey({ key: Buffer.concat([spkiPrefix, publicKeyRaw]), format: "der", type: "spki" });
  const signature = Buffer.from(String(info.signature || ""), "base64");
  const message = otaMessage(info.version || "", info.platform || platformName(), expectedSHA256, actualSize);
  if (signature.length !== 64 || !verifySignature(null, message, publicKey, signature)) {
    throw new Error("安装包发布签名无效");
  }
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

function otaMessage(version: string, platform: string, sha256: string, fileSize: number): Buffer {
  return Buffer.from(
    `scaletail-update-v1\n${version.trim()}\n${platform.trim().toLowerCase()}\n${sha256.trim().toLowerCase()}\n${fileSize}\n`,
    "utf8",
  );
}

function hasOTAMetadata(info: ClientUpdateInfo): boolean {
  return app.isPackaged
    && /^https?:\/\//i.test(String(info.download_url || ""))
    && /^[a-f0-9]{64}$/i.test(String(info.sha256 || ""))
    && Number.isSafeInteger(Number(info.file_size || 0))
    && Number(info.file_size || 0) > 0
    && String(info.signature || "").length >= 80;
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

function closeUpdateWindow(): void {
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
  const downloadDisabled = !info.download_url || active ? " disabled" : "";
  const tag = forced ? "强制更新" : "建议更新";
  const actionText = active ? "更新处理中" : (canOTA ? "立即更新" : "浏览器下载");
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

function endpoint(baseURL: string, pathName: string): string {
  const cleanBase = baseURL.endsWith("/") ? baseURL.slice(0, -1) : baseURL;
  const cleanPath = pathName.startsWith("/") ? pathName : `/${pathName}`;
  if (cleanBase.endsWith("/api/client-reports")) {
    return `${cleanBase}${cleanPath}`;
  }
  return `${cleanBase}/api/client-reports${cleanPath}`;
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

function escapeHTML(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}
