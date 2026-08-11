// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

import { app, BrowserWindow, ipcMain, Menu, nativeImage, Tray } from "electron";
import path from "node:path";
import {
  buildControlURL,
  getPrefs,
  getStatus,
  localRequest,
  LocalAPIError,
  logout,
  patchPrefs,
  runNetcheck,
  setUseExitNode,
  startDaemonUp,
  validateHostname,
  watchIPNBus,
} from "./localapi";
import { readClientReportConfig, saveClientReportConfig } from "./report_config";
import { getServiceOverview, startScaleTailService } from "./service";
import { resetTelemetryPolicyState, startTelemetryReporter } from "./telemetry";
import {
  assertClientUpdateAllowed,
  showRequiredClientUpdate,
  startClientUpdateChecker,
} from "./client_update";
import {
  clearAccountCredential,
  readAccountCredential,
  saveAccountCredential,
  validateAccountPassword,
} from "./credential_store";
import type { PasswordAuthErrorCode } from "./localapi";
import type { BackendState, ClientReportConfig, ConnectRequest, Status } from "../shared/types";

type Route = "dashboard" | "connect" | "nodes";

let mainWindow: BrowserWindow | undefined;
let tray: Tray | undefined;
let isQuitting = false;
let lastStatus: Status | undefined;
let stopWatch: (() => void) | undefined;
let stopTelemetry: (() => void) | undefined;
let stopUpdateChecker: (() => void) | undefined;
let refreshTimer: NodeJS.Timeout | undefined;
let accountOperationTail: Promise<void> = Promise.resolve();
let accountRestoreQueued = false;

const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on("second-instance", (_event, argv) => {
    void openRoute(routeFromArgs(argv) || "dashboard");
  });
}

app.whenReady().then(async () => {
  app.setAppUserModelId("com.scaletail.windows.client");
  createTray();
  registerIPC();
  startDaemonWatch();
  stopTelemetry = startTelemetryReporter({ getStatus });
  stopUpdateChecker = await startClientUpdateChecker({
    onForcedUpdate: suspendNetworkForForcedUpdate,
    onForcedUpdateCleared: resumeNetworkAfterForcedUpdate,
  });
  refreshTimer = setInterval(() => void refreshTrayStatus(), 8000);
  await refreshTrayStatus();
  queueAccountCredentialRestore();

  const initial = routeFromArgs(process.argv);
  if (initial) {
    setTimeout(() => void openRoute(initial), 500);
  }
});

app.on("before-quit", () => {
  isQuitting = true;
  stopWatch?.();
  stopTelemetry?.();
  stopUpdateChecker?.();
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
});

app.on("window-all-closed", () => {
  // Keep the tray process alive after the dashboard window is closed.
});

async function openDefaultWindow(): Promise<void> {
  try {
    const status = await ensureDaemonReady(false);
    const state = status.BackendState || "";
    await openRoute(needsServerConfig(state) ? "connect" : "dashboard");
  } catch {
    await openRoute("dashboard");
  }
}

async function openRoute(route: Route): Promise<void> {
  if (showRequiredClientUpdate()) {
    mainWindow?.hide();
    return;
  }
  if (!mainWindow || mainWindow.isDestroyed()) {
    mainWindow = createMainWindow(route);
    return;
  }
  mainWindow.webContents.send("navigate", route);
  if (mainWindow.isMinimized()) {
    mainWindow.restore();
  }
  mainWindow.show();
  mainWindow.focus();
}

function createMainWindow(route: Route): BrowserWindow {
  const win = new BrowserWindow({
    width: 1040,
    height: 780,
    minWidth: 920,
    minHeight: 680,
    title: route === "connect" ? "ScaleTail 服务端配置" : route === "nodes" ? "ScaleTail 节点" : "ScaleTail 仪表台",
    show: false,
    autoHideMenuBar: true,
    frame: false,
    transparent: true,
    hasShadow: false,
    resizable: false,
    maximizable: false,
    minimizable: false,
    fullscreenable: false,
    backgroundColor: "#00000000",
    icon: appIconPath(),
    webPreferences: {
      preload: path.join(__dirname, "../preload/preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
      webviewTag: false,
    },
  });

  win.on("close", (event) => {
    if (!isQuitting) {
      event.preventDefault();
      win.hide();
    }
  });
  win.once("ready-to-show", () => {
    win.show();
    win.focus();
  });
  win.webContents.on("will-navigate", (event) => {
    event.preventDefault();
  });
  win.webContents.on("will-attach-webview", (event) => {
    event.preventDefault();
  });
  win.webContents.setWindowOpenHandler(() => {
    return { action: "deny" };
  });
  win.webContents.session.setPermissionRequestHandler((_webContents, _permission, callback) => {
    callback(false);
  });
  void loadRenderer(win, route);
  return win;
}

async function loadRenderer(win: BrowserWindow, route: Route): Promise<void> {
  const devURL = process.env.ELECTRON_RENDERER_URL;
  if (devURL) {
    const rendererURL = trustedDevRendererURL(devURL);
    await win.loadURL(`${rendererURL}#/${route}`);
    return;
  }
  await win.loadFile(path.join(app.getAppPath(), "dist/renderer/index.html"), {
    hash: `/${route}`,
  });
}

function createTray(): void {
  const icon = nativeImage.createFromPath(appIconPath());
  tray = new Tray(icon);
  tray.setToolTip("ScaleTail");
  tray.on("click", () => {
    void openRoute("dashboard");
  });
  rebuildTrayMenu();
}

function rebuildTrayMenu(): void {
  if (!tray) {
    return;
  }
  const state = lastStatus?.BackendState || "状态未知";
  const menu = Menu.buildFromTemplate([
    { label: `状态：${stateLabel(state)}`, enabled: false },
    { type: "separator" },
    { label: "打开仪表盘", click: () => void openRoute("dashboard") },
    { label: "服务端设置", click: () => void openRoute("connect") },
    { label: "节点", click: () => void openRoute("nodes") },
    { type: "separator" },
    {
      label: "刷新状态",
      click: () => void refreshTrayStatus(),
    },
    {
      label: "退出托盘程序",
      click: () => {
        isQuitting = true;
        app.quit();
      },
    },
  ]);
  tray.setContextMenu(menu);
  tray.setToolTip(`ScaleTail - ${stateLabel(state)}`);
}

function registerIPC(): void {
  ipcMain.handle("api:getStatus", async (_event, peers = true) => ensureDaemonReady(Boolean(peers)));
  ipcMain.handle("api:getPrefs", async () => {
    await ensureDaemonReady(false);
    return getPrefs();
  });
  ipcMain.handle("api:connect", async (_event, req: ConnectRequest) => (
    serializeAccountOperation(() => connect(req))
  ));
  ipcMain.handle("api:disconnect", async () => serializeAccountOperation(disconnect));
  ipcMain.handle("api:reconnect", async () => serializeAccountOperation(reconnect));
  ipcMain.handle("api:logout", async () => serializeAccountOperation(async () => {
    try {
      await ensureDaemonReady(false);
      await logout();
      return { ok: true };
    } finally {
      clearAccountCredential();
      resetTelemetryPolicyState();
      await refreshTrayStatus();
    }
  }));
  ipcMain.handle("api:setExitNode", async (_event, id: string) => {
    assertClientUpdateAllowed();
    await ensureDaemonReady(false);
    const cleanID = String(id || "").trim();
    if (!cleanID) {
      await setUseExitNode(false);
    } else {
      await patchPrefs({
        Prefs: { ExitNodeID: cleanID },
        ExitNodeIDSet: true,
      });
    }
    await refreshTrayStatus();
    return { ok: true };
  });
  ipcMain.handle("api:setAdvertiseRoutes", async (_event, routes: string[]) => {
    assertClientUpdateAllowed();
    await ensureDaemonReady(false);
    const cleanRoutes = normalizeRoutes(routes);
    await patchPrefs({
      Prefs: { AdvertiseRoutes: cleanRoutes },
      AdvertiseRoutesSet: true,
    });
    await refreshTrayStatus();
    return { ok: true };
  });
  ipcMain.handle("api:netcheck", async () => {
    assertClientUpdateAllowed();
    await ensureDaemonReady(false);
    return runNetcheck();
  });
  ipcMain.handle("api:getServiceStatus", async () => getServiceOverview());
  ipcMain.handle("api:startService", async () => {
    const overview = await startScaleTailService(async () => {
      await getStatus(false);
    });
    await refreshTrayStatus();
    return overview;
  });
  ipcMain.handle("api:getReportConfig", async () => readClientReportConfig());
  ipcMain.handle("api:saveReportConfig", async (_event, config: ClientReportConfig) => {
    const saved = saveClientReportConfig(config);
    restartTelemetryReporter();
    return saved;
  });
  ipcMain.handle("window:dashboard", async () => openRoute("dashboard"));
  ipcMain.handle("window:connect", async () => openRoute("connect"));
  ipcMain.handle("window:close", async () => {
    mainWindow?.hide();
  });
}

function restartTelemetryReporter(): void {
  resetTelemetryPolicyState();
  stopTelemetry?.();
  stopTelemetry = startTelemetryReporter({ getStatus });
}

async function connect(req: ConnectRequest): Promise<{ ok: boolean; controlURL: string; message: string }> {
  assertClientUpdateAllowed();
  req = normalizeConnectRequest(req);
  const status = await ensureDaemonReady(false);
  const state = status.BackendState || "";
  if (state === "Stopped" && status.HaveNodeKey) {
    throw new Error("当前只是临时断开状态，请点击“恢复连接”。如需更换服务端，请先点击“退出当前网络”。");
  }
  if (state === "Running" || state === "Starting" || state === "NeedsMachineAuth") {
    throw new Error("当前已有连接或连接流程正在进行。请先临时断开，或退出当前网络后再修改服务端配置。");
  }

  const controlURL = buildControlURL(req);
  const hostname = validateHostname(req.hostname);
  const username = validateUsername(req.username);
  const password = validateAccountPassword(req.password);
  clearAccountCredential();
  saveAccountCredential({ controlURL, username, password });
  try {
    resetTelemetryPolicyState();
    await startDaemonUp(
      controlURL,
      hostname,
      Boolean(req.acceptRoutes),
      Boolean(req.acceptDNS),
    );

    await waitForPasswordAuthSession();
    try {
      // Local backend state can become Running before the first authenticated
      // map response arrives. Always submit account proof instead of treating
      // that local state as proof that the control server accepted the login.
      await authenticateWithPassword(controlURL, username, password);
    } catch (err) {
      throw passwordAuthenticationError(err);
    }
    const nextStatus = await waitForPasswordAuthResult();
    await refreshTrayStatus();

    const nextState = nextStatus.BackendState || "";
    let message = "已提交连接请求。";
    if (nextState === "Running") {
      message = "已连接到控制服务器。";
    } else if (nextState === "NeedsMachineAuth") {
      message = "已提交连接请求，请在服务端管理后台授权该设备。";
    } else if (nextState === "Starting") {
      message = "连接请求已提交，ScaleTail 服务正在与服务端建立连接。";
    }

    return {
      ok: true,
      controlURL,
      message,
    };
  } catch (err) {
    clearAccountCredential();
    throw err;
  }
}

async function disconnect(): Promise<{ ok: boolean; message: string }> {
  const status = await ensureDaemonReady(false);
  if (!status.HaveNodeKey && status.BackendState !== "NeedsMachineAuth") {
    throw new Error("当前没有可临时断开的已登录网络。");
  }
  await setWantRunning(false);
  resetTelemetryPolicyState();
  await refreshTrayStatus();
  return { ok: true, message: "已临时断开连接，登录状态仍保留。需要恢复时点击“恢复连接”。" };
}

async function reconnect(): Promise<{ ok: boolean; message: string }> {
  assertClientUpdateAllowed();
  const status = await ensureDaemonReady(false);
  if (!status.HaveNodeKey) {
    throw new Error("当前没有已保存的登录身份，请重新填写服务端信息和账号密码后连接。");
  }
  const credential = await readCurrentAccountCredential();
  if (!credential) {
    throw new Error("恢复连接需要账号密码，请在服务端设置中重新登录。");
  }
  resetTelemetryPolicyState();
  await setRunningPrefs(true);
  await authenticateAccountWithRetry(credential.controlURL, credential.username, credential.password, 45000);
  const nextStatus = await waitForPasswordAuthResult();
  await refreshTrayStatus();

  const nextState = nextStatus.BackendState || "";
  if (nextState === "Running") {
    return { ok: true, message: "已恢复连接。" };
  }
  if (nextState === "NeedsMachineAuth") {
    return { ok: true, message: "已提交恢复请求，当前仍在等待服务端设备授权。" };
  }
  if (nextState === "NeedsLogin" || nextStatus.AuthURL) {
    throw new Error("恢复连接需要重新认证。请退出当前网络后，使用账号密码重新连接。");
  }
  return { ok: true, message: "已提交恢复连接请求，ScaleTail 服务正在与服务端建立连接。" };
}

async function restoreAccountCredential(): Promise<void> {
  try {
    const status = await ensureDaemonReady(false);
    const state = status.BackendState || "";
    if (!status.HaveNodeKey || state === "Stopped") return;
    const credential = await readCurrentAccountCredential();
    if (!credential) return;
    await authenticateAccountWithRetry(
      credential.controlURL,
      credential.username,
      credential.password,
      15000,
    );
    await refreshTrayStatus();
  } catch (err) {
    console.warn("ScaleTail account proof restore failed:", formatError(err));
  }
}

function queueAccountCredentialRestore(): void {
  if (accountRestoreQueued) return;
  accountRestoreQueued = true;
  void serializeAccountOperation(restoreAccountCredential).finally(() => {
    accountRestoreQueued = false;
  });
}

function serializeAccountOperation<T>(operation: () => Promise<T>): Promise<T> {
  const result = accountOperationTail.then(operation, operation);
  accountOperationTail = result.then(() => undefined, () => undefined);
  return result;
}

async function suspendNetworkForForcedUpdate(): Promise<boolean> {
  return serializeAccountOperation(suspendNetworkForForcedUpdateUnlocked);
}

async function suspendNetworkForForcedUpdateUnlocked(): Promise<boolean> {
  mainWindow?.hide();
  const prefs = await getPrefs();
  const wasRunning = Boolean(prefs.WantRunning);
  if (wasRunning) {
    await setWantRunning(false);
  }
  resetTelemetryPolicyState();
  stopTelemetry?.();
  stopTelemetry = undefined;
  await refreshTrayStatus();
  return wasRunning;
}

async function resumeNetworkAfterForcedUpdate(resumeNetwork: boolean): Promise<void> {
  await serializeAccountOperation(() => resumeNetworkAfterForcedUpdateUnlocked(resumeNetwork));
}

async function resumeNetworkAfterForcedUpdateUnlocked(resumeNetwork: boolean): Promise<void> {
  if (!resumeNetwork) {
    return;
  }
  const credential = await readCurrentAccountCredential();
  if (!credential) {
    console.warn("ScaleTail cannot resume after update because no account credential is available.");
    return;
  }
  await setRunningPrefs(true);
  await authenticateAccountWithRetry(
    credential.controlURL,
    credential.username,
    credential.password,
    45_000,
  );
  restartTelemetryReporter();
  await refreshTrayStatus();
}

async function setWantRunning(wantRunning: boolean): Promise<void> {
  await patchPrefs({
    Prefs: { WantRunning: wantRunning },
    WantRunningSet: true,
  });
}

async function setRunningPrefs(wantRunning: boolean): Promise<void> {
  await patchPrefs({
    Prefs: { WantRunning: wantRunning, LoggedOut: false },
    WantRunningSet: true,
    LoggedOutSet: true,
  });
}

async function waitForPasswordAuthSession(): Promise<Status> {
  const deadline = Date.now() + 30000;
  let latest = await getStatus(false);
  while (Date.now() < deadline) {
    const state = latest.BackendState || "";
    if (state === "Running" || state === "NeedsMachineAuth" || latest.AuthURL) {
      return latest;
    }
    await delay(500);
    latest = await getStatus(false);
  }
  throw new Error("auth_session_expired: 未能从控制服务器取得有效认证会话，请检查服务端地址、端口和 HTTP/HTTPS 选择后重试。");
}

async function waitForPasswordAuthResult(): Promise<Status> {
  const deadline = Date.now() + 45000;
  let latest = await getStatus(false);
  while (Date.now() < deadline) {
    const state = latest.BackendState || "";
    if (state === "Running" || state === "NeedsMachineAuth") {
      return latest;
    }
    await delay(500);
    latest = await getStatus(false);
  }
  throw new Error("auth_session_expired: 账号认证已提交，但连接会话未在 45 秒内完成，请重新连接。");
}

async function authenticateAccountWithRetry(
  controlURL: string,
  username: string,
  password: string,
  timeoutMS: number,
): Promise<void> {
  const deadline = Date.now() + timeoutMS;
  let lastError: unknown;
  while (Date.now() < deadline) {
    try {
      await authenticateWithPassword(controlURL, username, password);
      return;
    } catch (err) {
      lastError = err;
      if (!isTransientAuthenticationError(err)) {
        throw passwordAuthenticationError(err);
      }
    }
    await delay(750);
  }
  throw passwordAuthenticationError(lastError ?? new Error("认证连接超时"));
}

async function authenticateWithPassword(controlURL: string, username: string, password: string): Promise<void> {
  await localRequest<void>(
    "POST",
    "/localapi/v0/scaletail-auth-password",
    { controlUrl: controlURL, username, password },
    204,
    30000,
  );
}

async function readCurrentAccountCredential() {
  const prefs = await getPrefs();
  return readAccountCredential(String(prefs.ControlURL || ""));
}

function isTransientAuthenticationError(err: unknown): boolean {
  if (!(err instanceof LocalAPIError)) return false;
  return err.code === "network_error"
    || err.statusCode === 502
    || err.statusCode === 503
    || err.statusCode === 504;
}

async function ensureDaemonReady(peers: boolean): Promise<Status> {
  try {
    return await getStatus(peers);
  } catch (firstError) {
    try {
      await startScaleTailService(async () => {
        await getStatus(false);
      });
      return await getStatus(peers);
    } catch (serviceError) {
      throw new Error(`无法连接 ScaleTail 服务，本地服务未运行或 LocalAPI 不可用: ${formatError(serviceError || firstError)}`);
    }
  }
}

async function refreshTrayStatus(): Promise<void> {
  try {
    lastStatus = await getStatus(false);
  } catch {
    lastStatus = undefined;
  }
  rebuildTrayMenu();
  mainWindow?.webContents.send("daemon-event", { type: "status", status: lastStatus });
}

function startDaemonWatch(): void {
  stopWatch?.();
  stopWatch = watchIPNBus(
    (notify) => {
      const n = notify as { State?: BackendState; Prefs?: unknown; BrowseToURL?: string };
      if (n.State || n.Prefs) {
        void refreshTrayStatus();
      }
      if (n.State === "NeedsLogin" || n.BrowseToURL) {
        queueAccountCredentialRestore();
      }
      mainWindow?.webContents.send("daemon-event", { type: "change" });
    },
    () => {
      // The watcher reconnects itself; avoid noisy UI for transient boot races.
    },
  );
}

function routeFromArgs(args: string[]): Route | undefined {
  if (args.includes("--open-connect")) {
    return "connect";
  }
  if (args.includes("--open-dashboard")) {
    return "dashboard";
  }
  if (args.includes("--open-nodes")) {
    return "nodes";
  }
  return undefined;
}

function normalizeRoutes(routes: string[]): string[] {
  const clean = [...new Set((routes || []).map((r) => String(r || "").trim()).filter(Boolean))];
  for (const route of clean) {
    if (!/^[0-9a-fA-F:.]+\/\d{1,3}$/.test(route)) {
      throw new Error(`路由格式不正确：${route}`);
    }
  }
  return clean;
}

function normalizeConnectRequest(req: ConnectRequest): ConnectRequest {
  if (!req || typeof req !== "object") {
    throw new Error("连接参数无效");
  }
  return {
    serverIP: String(req.serverIP || ""),
    serverPort: String(req.serverPort || ""),
    useHTTPS: Boolean(req.useHTTPS),
    hostname: String(req.hostname || ""),
    username: String(req.username || ""),
    password: String(req.password || ""),
    acceptRoutes: Boolean(req.acceptRoutes),
    acceptDNS: Boolean(req.acceptDNS),
  };
}

function validateUsername(username: string): string {
  const value = username.trim();
  if (!value || Buffer.byteLength(value, "utf8") > 254 || /[\u0000-\u001f\u007f]/.test(value)) {
    throw new Error("请输入有效账号，长度不能超过 254 字节");
  }
  return value;
}

function passwordAuthenticationError(err: unknown): Error {
  const code = err instanceof LocalAPIError ? asPasswordAuthErrorCode(err.code) : undefined;
  if (!code) {
    return new Error(`账号认证失败：${formatError(err)}`);
  }
  const messages: Record<PasswordAuthErrorCode, string> = {
    invalid_credentials: "账号或密码错误。",
    account_locked: "账号已锁定，请稍后重试或联系管理员。",
    account_disabled: "账号已被禁用，请联系管理员。",
    account_expired: "账号已过期，请联系管理员。",
    password_expired: "密码已过期，请先在管理平台修改密码。",
    network_not_assigned: "账号尚未分配到任何网络，请联系管理员。",
    node_limit_reached: "该账号已达到允许的节点数量上限，请联系管理员清理旧节点或调整上限。",
    tags_not_supported: "账号登录节点不支持身份标签，请移除宣告标签后重试。",
    auth_session_expired: "认证会话已过期，请重新连接。",
    invalid_auth_session: "认证会话无效，请重新发起连接。",
    machine_mismatch: "认证会话与当前设备不匹配，请重新发起连接。",
    auth_session_consumed: "认证会话已被使用，请重新发起连接。",
    too_many_attempts: "本次连接的认证尝试过多，请重新发起连接。",
    invalid_request: "认证参数无效，请检查账号和密码。",
    registration_failed: "服务端注册设备失败，请稍后重试或联系管理员。",
    route_approval_failed: "设备已注册，但服务端应用路由配置失败，请联系管理员。",
    internal_error: "服务端认证处理失败，请稍后重试。",
    authentication_failed: "服务端拒绝了账号认证，请稍后重试。",
    network_error: "暂时无法通过加密控制通道完成账号认证，请稍后重试。",
  };
  return new Error(`${code}: ${messages[code]}`);
}

function asPasswordAuthErrorCode(code: string | undefined): PasswordAuthErrorCode | undefined {
  switch (code) {
    case "invalid_credentials":
    case "account_locked":
    case "account_disabled":
    case "account_expired":
    case "password_expired":
    case "network_not_assigned":
    case "node_limit_reached":
    case "tags_not_supported":
    case "auth_session_expired":
    case "invalid_auth_session":
    case "machine_mismatch":
    case "auth_session_consumed":
    case "too_many_attempts":
    case "invalid_request":
    case "registration_failed":
    case "route_approval_failed":
    case "internal_error":
    case "authentication_failed":
    case "network_error":
      return code;
    default:
      return undefined;
  }
}

function trustedDevRendererURL(raw: string): string {
  const parsed = new URL(raw);
  const host = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || (host !== "localhost" && host !== "127.0.0.1" && host !== "::1")) {
    throw new Error("ELECTRON_RENDERER_URL 仅允许本机开发服务器");
  }
  parsed.hash = "";
  parsed.username = "";
  parsed.password = "";
  return parsed.toString().replace(/\/$/, "");
}

function needsServerConfig(state: string): boolean {
  return state === "NoState" || state === "NeedsLogin";
}

function resourcePath(file: string): string {
  return path.join(app.getAppPath(), "resources", file);
}

function appIconPath(): string {
  return resourcePath("app.ico");
}

function stateLabel(state: string): string {
  const labels: Record<string, string> = {
    Running: "已连接",
    Starting: "正在连接",
    NeedsLogin: "需要认证",
    NeedsMachineAuth: "等待设备授权",
    NoState: "未配置",
    Stopped: "已断开",
  };
  return labels[state] || state || "状态未知";
}

function formatError(err: unknown): string {
  return err instanceof Error ? err.message : String(err || "未知错误");
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
