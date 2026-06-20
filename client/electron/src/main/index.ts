import { app, BrowserWindow, ipcMain, Menu, nativeImage, shell, Tray } from "electron";
import path from "node:path";
import {
  buildControlURL,
  getPrefs,
  getStatus,
  logout,
  patchPrefs,
  runNetcheck,
  setUseExitNode,
  startLoginInteractive,
  startWithPrefs,
  validateHostname,
  watchIPNBus,
} from "./localapi";
import { readClientReportConfig, saveClientReportConfig } from "./report_config";
import { getServiceOverview, startScaleTailService } from "./service";
import { startTelemetryReporter } from "./telemetry";
import { BackendState, ClientReportConfig, ConnectRequest, Status } from "../shared/types";

type Route = "dashboard" | "connect" | "nodes";

let mainWindow: BrowserWindow | undefined;
let tray: Tray | undefined;
let isQuitting = false;
let lastStatus: Status | undefined;
let stopWatch: (() => void) | undefined;
let stopTelemetry: (() => void) | undefined;
let refreshTimer: NodeJS.Timeout | undefined;
let authBrowserAllowedUntil = 0;
let authBrowserSuppressedUntil = 0;
let lastAuthURL = "";
let lastAuthURLOpenedAt = 0;

const AUTH_BROWSER_WINDOW_MS = 2 * 60 * 1000;
const AUTH_URL_DEDUPE_MS = 60 * 1000;

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
  stopTelemetry = startTelemetryReporter({ getStatus, runNetcheck, setWantRunning });
  refreshTimer = setInterval(() => void refreshTrayStatus(), 8000);
  await refreshTrayStatus();

  const initial = routeFromArgs(process.argv);
  if (initial) {
    setTimeout(() => void openRoute(initial), 500);
  }
});

app.on("before-quit", () => {
  isQuitting = true;
  stopWatch?.();
  stopTelemetry?.();
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
      sandbox: false,
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
  win.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });
  void loadRenderer(win, route);
  return win;
}

async function loadRenderer(win: BrowserWindow, route: Route): Promise<void> {
  const devURL = process.env.ELECTRON_RENDERER_URL;
  if (devURL) {
    await win.loadURL(`${devURL}#/${route}`);
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
  ipcMain.handle("api:connect", async (_event, req: ConnectRequest) => connect(req));
  ipcMain.handle("api:disconnect", async () => disconnect());
  ipcMain.handle("api:reconnect", async () => reconnect());
  ipcMain.handle("api:logout", async () => {
    await ensureDaemonReady(false);
    await logout();
    await refreshTrayStatus();
    return { ok: true };
  });
  ipcMain.handle("api:setExitNode", async (_event, id: string) => {
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
  stopTelemetry?.();
  stopTelemetry = startTelemetryReporter({ getStatus, runNetcheck, setWantRunning });
}

async function connect(req: ConnectRequest): Promise<{ ok: boolean; controlURL: string; message: string }> {
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
  const prefs = await getPrefs();
  prefs.ControlURL = controlURL;
  prefs.Hostname = hostname;
  prefs.WantRunning = true;
  prefs.LoggedOut = false;
  prefs.RouteAll = Boolean(req.acceptRoutes);

  const authKey = req.authKey.trim();
  if (authKey) {
    suppressAuthBrowser();
  } else {
    allowAuthBrowser();
  }
  await startWithPrefs(prefs, authKey);
  if (!authKey && (state === "NeedsLogin" || !status.HaveNodeKey)) {
    await startLoginInteractive();
  }
  const nextStatus = await waitForConnectionProgress(authKey);
  if (!authKey && nextStatus.AuthURL) {
    openAuthURLIfAllowed(nextStatus.AuthURL);
  }
  await refreshTrayStatus();

  const nextState = nextStatus.BackendState || "";
  let message = "已提交连接请求。";
  if (nextState === "Running") {
    message = "已连接到控制服务器。";
  } else if (nextState === "NeedsMachineAuth") {
    message = "已提交连接请求，请在服务端管理后台授权该设备。";
  } else if (nextState === "NeedsLogin") {
    message = authKey
      ? "连接仍需要认证。请确认预认证密钥属于当前服务端、未过期且未被一次性使用；也可以不填 key 改用浏览器认证。"
      : "已打开认证页面，请在浏览器中完成登录。";
  } else if (nextState === "Starting") {
    message = "连接请求已提交，ScaleTail 服务正在与服务端建立连接。";
  }

  return {
    ok: true,
    controlURL,
    message,
  };
}

async function disconnect(): Promise<{ ok: boolean; message: string }> {
  const status = await ensureDaemonReady(false);
  if (!status.HaveNodeKey && status.BackendState !== "NeedsMachineAuth") {
    throw new Error("当前没有可临时断开的已登录网络。");
  }
  await setWantRunning(false);
  await refreshTrayStatus();
  return { ok: true, message: "已临时断开连接，登录状态仍保留。需要恢复时点击“恢复连接”。" };
}

async function reconnect(): Promise<{ ok: boolean; message: string }> {
  const status = await ensureDaemonReady(false);
  if (!status.HaveNodeKey) {
    throw new Error("当前没有已保存的登录身份，请重新填写服务端信息和预认证密钥后连接。");
  }
  suppressAuthBrowser();
  await setRunningPrefs(true);
  const nextStatus = await waitForConnectionProgress("");
  await refreshTrayStatus();

  const nextState = nextStatus.BackendState || "";
  if (nextState === "Running") {
    return { ok: true, message: "已恢复连接。" };
  }
  if (nextState === "NeedsMachineAuth") {
    return { ok: true, message: "已提交恢复请求，当前仍在等待服务端设备授权。" };
  }
  if (nextState === "NeedsLogin" || nextStatus.AuthURL) {
    throw new Error("恢复连接需要重新认证。请退出当前网络后，使用有效预认证密钥重新连接。");
  }
  return { ok: true, message: "已提交恢复连接请求，ScaleTail 服务正在与服务端建立连接。" };
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

async function waitForConnectionProgress(authKey: string): Promise<Status> {
  const started = Date.now();
  const deadline = Date.now() + 45000;
  let latest = await getStatus(false);
  while (Date.now() < deadline) {
    const state = latest.BackendState || "";
    if (state === "Running" || state === "NeedsMachineAuth" || latest.AuthURL) {
      return latest;
    }
    if (state === "NeedsLogin" && !authKey && Date.now() - started > 8000) {
      return latest;
    }
    await delay(1000);
    latest = await getStatus(false);
  }
  if (authKey) {
    throw new Error("连接请求已提交，但 45 秒内未进入已连接或等待授权状态。请检查服务端地址、端口、HTTP/HTTPS 选择，以及预认证密钥是否属于当前服务端、未过期且未被一次性使用。");
  }
  return latest;
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
      const n = notify as { BrowseToURL?: string; State?: BackendState; Prefs?: unknown };
      if (n.BrowseToURL) {
        openAuthURLIfAllowed(n.BrowseToURL);
      }
      if (n.State || n.Prefs) {
        void refreshTrayStatus();
      }
      mainWindow?.webContents.send("daemon-event", notify);
    },
    () => {
      // The watcher reconnects itself; avoid noisy UI for transient boot races.
    },
  );
}

function allowAuthBrowser(): void {
  authBrowserAllowedUntil = Date.now() + AUTH_BROWSER_WINDOW_MS;
  authBrowserSuppressedUntil = 0;
}

function suppressAuthBrowser(): void {
  authBrowserAllowedUntil = 0;
  authBrowserSuppressedUntil = Date.now() + AUTH_BROWSER_WINDOW_MS;
}

function openAuthURLIfAllowed(url: string): boolean {
  const now = Date.now();
  if (!url || now < authBrowserSuppressedUntil || now > authBrowserAllowedUntil) {
    return false;
  }
  if (url === lastAuthURL && now - lastAuthURLOpenedAt < AUTH_URL_DEDUPE_MS) {
    return true;
  }
  lastAuthURL = url;
  lastAuthURLOpenedAt = now;
  void shell.openExternal(url);
  return true;
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
