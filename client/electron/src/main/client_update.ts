import { app, BrowserWindow, screen, shell } from "electron";
import { readClientReportConfig } from "./report_config";
import type { ClientUpdateInfo } from "../shared/types";

interface UpdateResponse {
  code?: number;
  data?: ClientUpdateInfo;
}

const defaultIntervalMS = 10 * 60_000;
const initialDelayMS = 12_000;
const forcedReminderMS = 60_000;
const requestTimeoutMS = 10_000;

let timer: NodeJS.Timeout | undefined;
let initialTimer: NodeJS.Timeout | undefined;
let forcedReminderTimer: NodeJS.Timeout | undefined;
let updateWindow: BrowserWindow | undefined;
let latestInfo: ClientUpdateInfo | undefined;
let checking = false;
let lastSuggestedVersion = "";

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
  if (checking) {
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
    update_type: updateType,
    forced: Boolean(info.forced || updateType === "forced"),
    title: info.title?.trim() || `发现 ScaleTail ${info.version || "新版本"}`,
    description: info.description?.trim() || (
      updateType === "forced"
        ? "该版本被标记为强制更新，安装新版本前客户端会持续提醒。"
        : "建议安装新版本以获得最新功能和修复。"
    ),
  };
}

function showUpdateWindow(info: ClientUpdateInfo): void {
  if (updateWindow && !updateWindow.isDestroyed()) {
    updateWindow.loadURL(updateDataURL(info)).catch(() => undefined);
    placeWindow(updateWindow, info);
    updateWindow.show();
    updateWindow.focus();
    return;
  }

  updateWindow = new BrowserWindow({
    width: 430,
    height: info.forced ? 330 : 310,
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
    handleAction(url);
  });
  updateWindow.webContents.setWindowOpenHandler(({ url }) => {
    handleAction(url);
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
  updateWindow.loadURL(updateDataURL(info)).catch(() => undefined);
}

function handleAction(url: string): void {
  if (!url.startsWith("scaletail-update://")) {
    if (/^https?:\/\//i.test(url)) {
      void shell.openExternal(url);
    }
    return;
  }
  const action = url.replace("scaletail-update://", "").replace(/\/$/, "");
  if (action === "download") {
    const downloadURL = latestInfo?.download_url || "";
    if (/^https?:\/\//i.test(downloadURL)) {
      void shell.openExternal(downloadURL);
    }
    if (!latestInfo?.forced) {
      closeUpdateWindow();
    }
    return;
  }
  if (action === "later") {
    if (!latestInfo?.forced) {
      closeUpdateWindow();
    }
    return;
  }
  if (action === "quit") {
    app.quit();
  }
}

function placeWindow(win: BrowserWindow, info: ClientUpdateInfo): void {
  const bounds = win.getBounds();
  const height = info.forced ? 330 : 310;
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
    if (latestInfo?.forced) {
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
  const downloadDisabled = info.download_url ? "" : " disabled";
  const tag = forced ? "强制更新" : "建议更新";
  const laterButton = forced
    ? `<a class="btn ghost danger" href="scaletail-update://quit">退出客户端</a>`
    : `<a class="btn ghost" href="scaletail-update://later">稍后提醒</a>`;
  const closeButton = forced ? "" : `<a class="close" href="scaletail-update://later" title="关闭">×</a>`;
  return `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <style>
    :root {
      color-scheme: light;
      font-family: "Microsoft YaHei UI", "Segoe UI", system-ui, sans-serif;
      --text: #172033;
      --muted: #667085;
      --blue: #2563eb;
      --blue-strong: #1d4ed8;
      --red: #dc2626;
      --amber: #d97706;
      --line: rgba(128, 145, 170, 0.28);
    }
    * { box-sizing: border-box; }
    html, body { width: 100%; height: 100%; margin: 0; background: transparent; overflow: hidden; }
    body { padding: 10px; color: var(--text); }
    .card {
      position: relative;
      width: 100%;
      height: 100%;
      padding: 18px;
      border: 1px solid rgba(255,255,255,0.64);
      border-radius: 22px;
      background: rgba(246, 249, 253, 0.92);
      box-shadow: 0 18px 46px rgba(24, 39, 75, 0.18);
      backdrop-filter: blur(24px) saturate(130%);
      -webkit-backdrop-filter: blur(24px) saturate(130%);
      clip-path: inset(0 round 22px);
    }
    .head { display: flex; align-items: center; gap: 12px; padding-right: 32px; }
    .logo {
      width: 42px; height: 42px; border-radius: 12px;
      display: grid; place-items: center;
      color: #fff; font-weight: 800;
      background: linear-gradient(135deg, #2563eb, #10b981);
      box-shadow: 0 10px 22px rgba(37, 99, 235, 0.22);
    }
    .kicker { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; color: var(--muted); font-size: 12px; font-weight: 700; }
    .tag {
      padding: 2px 8px; border-radius: 999px; font-size: 12px;
      color: ${forced ? "var(--red)" : "var(--amber)"};
      background: ${forced ? "rgba(255,241,242,0.88)" : "rgba(255,247,232,0.9)"};
    }
    h1 { margin: 0; font-size: 19px; line-height: 1.25; }
    p { margin: 14px 0 0; color: var(--muted); font-size: 13px; line-height: 1.55; }
    .notes {
      margin-top: 12px;
      max-height: 76px;
      overflow: auto;
      padding: 10px;
      border: 1px solid var(--line);
      border-radius: 10px;
      color: #344054;
      background: rgba(255, 255, 255, 0.68);
      white-space: pre-wrap;
      font-size: 12px;
      line-height: 1.45;
    }
    .actions {
      position: absolute;
      left: 18px; right: 18px; bottom: 18px;
      display: flex; justify-content: flex-end; gap: 8px;
    }
    .btn {
      display: inline-flex; align-items: center; justify-content: center;
      height: 36px; padding: 0 14px;
      border: 1px solid var(--line); border-radius: 8px;
      color: var(--text); background: rgba(255,255,255,0.66);
      text-decoration: none; font-size: 13px; font-weight: 700;
    }
    .btn.primary { border-color: var(--blue); color: #fff; background: var(--blue); }
    .btn.primary:hover { background: var(--blue-strong); }
    .btn.ghost:hover { background: rgba(255,255,255,0.84); }
    .btn.danger { color: var(--red); }
    .btn.disabled { opacity: 0.5; pointer-events: none; }
    .close {
      position: absolute; right: 13px; top: 12px;
      width: 30px; height: 30px; border-radius: 8px;
      display: grid; place-items: center;
      color: var(--muted); text-decoration: none; font-size: 20px;
    }
    .close:hover { color: #fff; background: var(--red); }
  </style>
</head>
<body>
  <section class="card">
    ${closeButton}
    <div class="head">
      <div class="logo">S</div>
      <div>
        <div class="kicker"><span>ScaleTail 更新</span><span class="tag">${tag}</span></div>
        <h1>${title}</h1>
      </div>
    </div>
    <p>${description}</p>
    <div class="notes">${notes || `当前版本：${escapeHTML(app.getVersion())}${version ? `\n最新版本：${version}` : ""}`}</div>
    <div class="actions">
      ${laterButton}
      <a class="btn primary${downloadDisabled}" href="scaletail-update://download">立即下载</a>
    </div>
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
