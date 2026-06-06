import { contextBridge, ipcRenderer } from "electron";
import { ConnectRequest, TailscaleAPI } from "../shared/types";

const api: TailscaleAPI = {
  getStatus: (peers = true) => ipcRenderer.invoke("api:getStatus", peers),
  getPrefs: () => ipcRenderer.invoke("api:getPrefs"),
  connect: (req: ConnectRequest) => ipcRenderer.invoke("api:connect", req),
  disconnect: () => ipcRenderer.invoke("api:disconnect"),
  reconnect: () => ipcRenderer.invoke("api:reconnect"),
  logout: () => ipcRenderer.invoke("api:logout"),
  setExitNode: (id: string) => ipcRenderer.invoke("api:setExitNode", id),
  setAdvertiseRoutes: (routes: string[]) => ipcRenderer.invoke("api:setAdvertiseRoutes", routes),
  runNetcheck: () => ipcRenderer.invoke("api:netcheck"),
  getServiceStatus: () => ipcRenderer.invoke("api:getServiceStatus"),
  startService: () => ipcRenderer.invoke("api:startService"),
  openDashboard: () => ipcRenderer.invoke("window:dashboard"),
  openConnect: () => ipcRenderer.invoke("window:connect"),
  closeWindow: () => ipcRenderer.invoke("window:close"),
  onNavigate: (cb) => {
    const listener = (_event: Electron.IpcRendererEvent, route: "dashboard" | "connect" | "nodes") => cb(route);
    ipcRenderer.on("navigate", listener);
    return () => ipcRenderer.removeListener("navigate", listener);
  },
  onDaemonEvent: (cb) => {
    const listener = (_event: Electron.IpcRendererEvent, payload: unknown) => cb(payload);
    ipcRenderer.on("daemon-event", listener);
    return () => ipcRenderer.removeListener("daemon-event", listener);
  },
};

contextBridge.exposeInMainWorld("tailscale", api);
