// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package systray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"scaletail.com/client/local"
	"scaletail.com/ipn"
	"scaletail.com/net/netcheck"
	"scaletail.com/net/netmon"
	"scaletail.com/tailcfg"
	"scaletail.com/types/logger"
	"scaletail.com/util/dnsname"
	"scaletail.com/util/eventbus"
)

var (
	panelMu     sync.Mutex
	panelServer *DashboardServer
)

// DashboardServer serves the Windows-friendly control panel used by systray.
type DashboardServer struct {
	lc   *local.Client
	http *http.Server
	url  string
}

// StartDashboard creates or reuses the local control panel and returns the
// dashboard URL.
func StartDashboard(lc *local.Client) (string, error) {
	return startPanelURL(lc, "/dashboard")
}

// StartConnectWindow creates or reuses the local control panel and returns the
// server configuration URL.
func StartConnectWindow(lc *local.Client) (string, error) {
	return startPanelURL(lc, "/connect")
}

func startPanelURL(lc *local.Client, path string) (string, error) {
	ds, err := startPanel(lc)
	if err != nil {
		return "", err
	}
	return ds.url + path, nil
}

func startPanel(lc *local.Client) (*DashboardServer, error) {
	panelMu.Lock()
	defer panelMu.Unlock()

	if panelServer != nil {
		return panelServer, nil
	}
	if lc == nil {
		lc = &local.Client{}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("control panel listen: %w", err)
	}

	ds := &DashboardServer{
		lc:  lc,
		url: "http://" + listener.Addr().String(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.serveRoot)
	mux.HandleFunc("/dashboard", ds.serveDashboard)
	mux.HandleFunc("/connect", ds.serveConnect)
	mux.HandleFunc("/api/status", ds.serveStatus)
	mux.HandleFunc("/api/prefs", ds.servePrefs)
	mux.HandleFunc("/api/ping", ds.servePing)
	mux.HandleFunc("/api/connect", ds.serveConnectAPI)
	mux.HandleFunc("/api/disconnect", ds.serveDisconnectAPI)
	mux.HandleFunc("/api/logout", ds.serveLogoutAPI)
	mux.HandleFunc("/api/exit-node", ds.serveExitNodeAPI)
	mux.HandleFunc("/api/netcheck", ds.serveNetcheck)

	ds.http = &http.Server{Handler: mux}
	panelServer = ds

	go func() {
		if err := ds.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("control panel server: %v", err)
		}
	}()

	return ds, nil
}

// Shutdown stops the dashboard server.
func (ds *DashboardServer) Shutdown() {
	if ds.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ds.http.Shutdown(ctx)
	}
}

func (ds *DashboardServer) serveRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (ds *DashboardServer) serveDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, dashboardHTML)
}

func (ds *DashboardServer) serveConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, connectHTML)
}

func (ds *DashboardServer) serveStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	status, err := ds.lc.Status(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, status)
}

func (ds *DashboardServer) servePrefs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	prefs, err := ds.lc.GetPrefs(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, struct {
		ControlURL  string
		Hostname    string
		WantRunning bool
		RouteAll    bool
		ExitNodeID  string
	}{
		ControlURL:  prefs.ControlURL,
		Hostname:    prefs.Hostname,
		WantRunning: prefs.WantRunning,
		RouteAll:    prefs.RouteAll,
		ExitNodeID:  string(prefs.ExitNodeID),
	})
}

func (ds *DashboardServer) servePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct{ OK bool }{OK: true})
}

func (ds *DashboardServer) serveConnectAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("需要使用 POST 请求"))
		return
	}

	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	controlURL, err := req.controlURL()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hostname, err := req.hostname()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if st, err := ds.lc.StatusWithoutPeers(ctx); err == nil && st.BackendState == ipn.Running.String() {
		writeError(w, http.StatusConflict, errors.New("当前已连接。请先退出当前网络，再修改服务端配置"))
		return
	}
	prefs, err := ds.lc.GetPrefs(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	prefs.ControlURL = controlURL
	prefs.Hostname = hostname
	prefs.WantRunning = true
	prefs.RouteAll = req.AcceptRoutes

	authKey := strings.TrimSpace(req.AuthKey)
	if err := ds.lc.Start(ctx, ipn.Options{
		UpdatePrefs: prefs,
		AuthKey:     authKey,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	message := "已提交连接请求。"
	if authKey == "" {
		if err := ds.lc.StartLoginInteractive(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		message = "已提交连接请求，请在随后打开的浏览器中完成认证。"
	}

	writeJSON(w, struct {
		OK         bool
		ControlURL string
		Message    string
	}{
		OK:         true,
		ControlURL: controlURL,
		Message:    message,
	})
}

func (ds *DashboardServer) serveDisconnectAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("需要使用 POST 请求"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	_, err := ds.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			WantRunning: false,
		},
		WantRunningSet: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, struct{ OK bool }{OK: true})
}

func (ds *DashboardServer) serveLogoutAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("需要使用 POST 请求"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if err := ds.lc.Logout(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, struct{ OK bool }{OK: true})
}

func (ds *DashboardServer) serveExitNodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("需要使用 POST 请求"))
		return
	}

	var req struct {
		ID string
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		if err := ds.lc.SetUseExitNode(ctx, false); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, struct{ OK bool }{OK: true})
		return
	}
	_, err := ds.lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			ExitNodeID: tailcfg.StableNodeID(id),
		},
		ExitNodeIDSet: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, struct{ OK bool }{OK: true})
}

func (ds *DashboardServer) serveNetcheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("需要使用 GET 或 POST 请求"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := ds.ensureBackend(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	report, err := runNetcheck(ctx, ds.lc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, report)
}

func (ds *DashboardServer) ensureBackend(ctx context.Context) error {
	if _, err := ds.lc.StatusWithoutPeers(ctx); err != nil {
		return fmt.Errorf("无法连接 scaletaild，本地服务未运行或 LocalAPI 不可用。请确认 ScaleTail 服务已安装并启动: %w", err)
	}
	return nil
}

func runNetcheck(ctx context.Context, lc *local.Client) (*netcheck.Report, error) {
	bus := eventbus.New()
	defer bus.Close()

	netMon, err := netmon.New(bus, logger.Discard)
	if err != nil {
		return nil, err
	}
	dm, err := lc.CurrentDERPMap(ctx)
	if err != nil {
		return nil, err
	}
	if dm == nil || len(dm.Regions) == 0 {
		return nil, errors.New("DERP 映射为空，请先连接控制服务器")
	}

	c := &netcheck.Client{
		NetMon:      netMon,
		UseDNSCache: false,
		Logf:        logger.Discard,
	}
	if err := c.Standalone(ctx, ""); err != nil {
		log.Printf("netcheck UDP setup: %v", err)
	}
	return c.GetReport(ctx, dm, nil)
}

type connectRequest struct {
	ServerIP     string
	ServerPort   string
	UseHTTPS     bool
	Hostname     string
	AuthKey      string
	AcceptRoutes bool
}

func (r connectRequest) controlURL() (string, error) {
	host := strings.TrimSpace(r.ServerIP)
	port := strings.TrimSpace(r.ServerPort)
	useHTTPS := r.UseHTTPS

	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil {
			return "", fmt.Errorf("服务端 URL 无效: %w", err)
		}
		if u.Scheme == "https" {
			useHTTPS = true
		} else if u.Scheme == "http" {
			useHTTPS = false
		} else {
			return "", errors.New("服务端 URL 协议必须是 http 或 https")
		}
		host = u.Hostname()
		if port == "" {
			port = u.Port()
		}
	}

	host = strings.Trim(host, "[]")
	if host == "" {
		return "", errors.New("请输入服务端地址")
	}
	if port == "" {
		if useHTTPS {
			port = "443"
		} else {
			port = "80"
		}
	}

	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", errors.New("服务端端口必须在 1 到 65535 之间")
	}

	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, port),
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (r connectRequest) hostname() (string, error) {
	hostname := strings.TrimSpace(r.Hostname)
	if hostname == "" {
		return "", nil
	}
	if err := dnsname.ValidHostname(hostname); err != nil {
		return "", fmt.Errorf("设备名称无效，只能使用字母、数字、短横线和点号: %w", err)
	}
	return hostname, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string
	}{
		Error: err.Error(),
	})
}

const connectHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ScaleTail 服务端配置</title>
<style>
:root{color-scheme:light;--bg:#f6f8fb;--panel:#fff;--text:#172033;--muted:#667085;--line:#d8dee9;--blue:#2563eb;--blue2:#1d4ed8;--red:#dc2626;--green:#15803d}
*{box-sizing:border-box}body{margin:0;background:var(--bg);font-family:"Segoe UI",system-ui,sans-serif;color:var(--text)}
.wrap{max-width:620px;margin:0 auto;padding:18px}.top{display:flex;align-items:center;justify-content:space-between;margin-bottom:12px}.brand{font-size:18px;font-weight:700}.state{font-size:12px;color:var(--muted)}
.panel{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:18px;box-shadow:0 8px 24px rgba(24,39,75,.06)}
h1{font-size:18px;margin:0 0 4px}p{margin:0;color:var(--muted);line-height:1.45;font-size:13px}.grid{display:grid;grid-template-columns:1fr 128px;gap:10px;margin-top:16px}.field{margin-top:12px}label{display:block;font-size:12px;font-weight:600;margin-bottom:6px}
input[type=text],input[type=password]{width:100%;height:36px;border:1px solid var(--line);border-radius:6px;padding:0 10px;font-size:13px;background:#fff;color:var(--text)}
input:focus{outline:2px solid rgba(37,99,235,.16);border-color:var(--blue)}.checks{display:flex;gap:18px;flex-wrap:wrap;margin-top:12px}.check{display:flex;align-items:center;gap:8px;height:28px;font-size:13px;font-weight:600}.check input{width:16px;height:16px}
.url{margin-top:12px;background:#f8fafc;border:1px solid var(--line);border-radius:6px;padding:9px 10px;font-family:Consolas,monospace;font-size:12px;color:#344054;word-break:break-all}.cmd-head{display:flex;align-items:center;justify-content:space-between;margin-top:14px}.cmd-title{font-size:12px;font-weight:700}.cmd{margin-top:6px;width:100%;min-height:58px;resize:vertical;border:1px solid var(--line);border-radius:6px;background:#101828;color:#e5e7eb;padding:9px 10px;font-family:Consolas,monospace;font-size:12px;line-height:1.45}
.actions{display:flex;gap:8px;margin-top:16px;flex-wrap:wrap}button,a.btn{border:1px solid var(--line);background:#fff;color:var(--text);height:36px;padding:0 12px;border-radius:6px;font-size:13px;cursor:pointer;text-decoration:none;display:inline-flex;align-items:center}
button.primary{border-color:var(--blue);background:var(--blue);color:#fff}button.primary:hover{background:var(--blue2)}button:disabled{opacity:.6;cursor:not-allowed}.msg{margin-top:12px;font-size:12px;line-height:1.45}.msg.err{color:var(--red)}.msg.ok{color:var(--green)}
button.danger{border-color:#fecaca;color:var(--red);background:#fff}
@media(max-width:640px){.wrap{padding:12px}.grid{grid-template-columns:1fr}.top{align-items:flex-start;gap:6px;flex-direction:column}}
</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div class="brand">ScaleTail</div>
    <div class="state" id="state">读取状态中...</div>
  </div>
  <div class="panel">
    <h1>服务端连接</h1>
    <p>填写控制服务器信息。下方会同步生成等价命令，方便你核对参数。</p>
    <div class="grid">
      <div>
        <label for="serverIP">服务端 IP 或域名</label>
        <input id="serverIP" type="text" placeholder="192.168.1.10 或 headscale.example.com">
      </div>
      <div>
        <label for="serverPort">端口</label>
        <input id="serverPort" type="text" inputmode="numeric" placeholder="80">
      </div>
    </div>
    <div class="field">
      <label for="hostname">本机设备名称，可选</label>
      <input id="hostname" type="text" placeholder="留空使用系统主机名，例如 office-pc">
    </div>
    <div class="checks">
      <label class="check"><input id="useHTTPS" type="checkbox"> 使用 HTTPS</label>
      <label class="check"><input id="acceptRoutes" type="checkbox" checked> 接受路由</label>
    </div>
    <div class="url" id="preview">http://:80</div>
    <div class="field">
      <label for="authKey">预认证密钥，可选</label>
      <input id="authKey" type="password" placeholder="tskey-auth-...">
    </div>
    <div class="cmd-head">
      <div class="cmd-title">等价命令</div>
      <button id="copyBtn">复制命令</button>
    </div>
    <textarea class="cmd" id="commandLine" readonly></textarea>
    <div class="actions">
      <button class="primary" id="connectBtn">连接</button>
      <button class="danger" id="logoutBtn">退出当前网络并修改</button>
      <a class="btn" href="/dashboard">仪表台</a>
      <button id="closeBtn">关闭窗口</button>
    </div>
    <div class="msg" id="msg"></div>
  </div>
</div>
<script>
const $ = id => document.getElementById(id);
if(!initPanelWindow()){ throw new Error('panel already open'); }
let connected = false;
function initPanelWindow(){
  const idKey = 'scaletail-panel-id';
  const activeKey = 'scaletail-panel-active';
  const commandKey = 'scaletail-panel-command';
  let id = sessionStorage.getItem(idKey);
  if(!id){ id = Date.now() + '-' + Math.random(); sessionStorage.setItem(idKey, id); }
  try{
    const active = JSON.parse(localStorage.getItem(activeKey) || '{}');
    if(active.id && active.id !== id && Date.now() - active.ts < 5000){
      localStorage.setItem(commandKey, JSON.stringify({id, target:location.pathname, ts:Date.now()}));
      document.body.innerHTML = '<div class="wrap"><div class="panel"><h1>仪表台已打开</h1><p>已有一个 ScaleTail 窗口正在运行，本窗口会自动关闭。</p></div></div>';
      setTimeout(() => window.close(), 120);
      return false;
    }
  }catch(e){}
  const beat = () => localStorage.setItem(activeKey, JSON.stringify({id, ts: Date.now()}));
  beat();
  setInterval(beat, 1000);
  window.addEventListener('beforeunload', () => {
    try{
      const active = JSON.parse(localStorage.getItem(activeKey) || '{}');
      if(active.id === id) localStorage.removeItem(activeKey);
    }catch(e){}
  });
  window.addEventListener('storage', ev => {
    if(ev.key !== commandKey || !ev.newValue) return;
    try{
      const cmd = JSON.parse(ev.newValue);
      if(cmd.id === id || Date.now() - cmd.ts > 5000) return;
      if(cmd.target && location.pathname !== cmd.target) location.href = cmd.target;
      else window.focus();
    }catch(e){}
  });
  setInterval(async () => {
    try{ await fetch('/api/ping', {cache:'no-store'}); }
    catch(e){
      document.body.innerHTML = '<div class="wrap"><div class="panel"><h1>托盘程序已退出</h1><p>本地控制面板已停止，本窗口会自动关闭。</p></div></div>';
      setTimeout(() => window.close(), 300);
    }
  }, 2500);
  return true;
}
function stateLabel(state){
  const labels = {Running:'已连接',Starting:'正在连接',NeedsLogin:'需要认证',NeedsMachineAuth:'等待设备授权',NoState:'未配置',Stopped:'已断开'};
  return labels[state] || state || '-';
}
async function getJSON(path, opts){
  const res = await fetch(path, opts);
  let data = {};
  try{ data = await res.json(); }catch(e){}
  if(!res.ok) throw new Error(data.Error || res.statusText);
  return data;
}
function parseControlURL(raw){
  if(!raw) return;
  try{
    const u = new URL(raw);
    $('serverIP').value = u.hostname || '';
    $('serverPort').value = u.port || (u.protocol === 'https:' ? '443' : '80');
    $('useHTTPS').checked = u.protocol === 'https:';
  }catch(e){}
}
function currentURL(){
  const scheme = $('useHTTPS').checked ? 'https' : 'http';
  const host = $('serverIP').value.trim();
  const port = $('serverPort').value.trim() || ($('useHTTPS').checked ? '443' : '80');
  return scheme + '://' + host + ':' + port;
}
function quoteArg(v){
  if(!v) return '""';
  return /[\s"&|<>^]/.test(v) ? '"' + v.replace(/"/g, '\\"') + '"' : v;
}
function refreshPreview(){
  const url = currentURL();
  const key = $('authKey').value.trim();
  const hostname = $('hostname').value.trim();
  const args = ['scaletail','up','--login-server=' + quoteArg(url),'--accept-routes=' + ($('acceptRoutes').checked ? 'true' : 'false')];
  if(hostname) args.push('--hostname=' + quoteArg(hostname));
  if(key) args.push('--auth-key=' + quoteArg(key));
  $('preview').textContent = url;
  $('commandLine').value = args.join(' ');
}
function setLocked(locked){
  connected = locked;
  ['serverIP','serverPort','useHTTPS','hostname','acceptRoutes','authKey'].forEach(id => { $(id).disabled = locked; });
  $('connectBtn').disabled = locked;
  $('copyBtn').disabled = locked;
  $('logoutBtn').style.display = locked ? 'inline-flex' : 'none';
  if(locked){
    $('msg').className = 'msg';
    $('msg').textContent = '当前已连接，服务端配置已锁定。需要连接其他服务端时，请先退出当前网络。';
  }
}
async function load(){
  try{
    const st = await getJSON('/api/status');
    $('state').textContent = st.BackendState ? ('当前状态：' + stateLabel(st.BackendState)) : '状态未知';
    connected = st.BackendState === 'Running';
  }catch(e){ $('state').textContent = '无法读取状态'; }
  try{
    const prefs = await getJSON('/api/prefs');
    parseControlURL(prefs.ControlURL);
    $('hostname').value = prefs.Hostname || '';
    if(typeof prefs.RouteAll === 'boolean') $('acceptRoutes').checked = prefs.RouteAll;
  }catch(e){}
  if(!$('serverPort').value) $('serverPort').value = $('useHTTPS').checked ? '443' : '80';
  refreshPreview();
  setLocked(connected);
}
async function connect(){
  if(connected) return;
  $('msg').className = 'msg';
  $('msg').textContent = '正在提交连接请求...';
  $('connectBtn').disabled = true;
  try{
    const data = await getJSON('/api/connect', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({
        ServerIP:$('serverIP').value,
        ServerPort:$('serverPort').value,
        UseHTTPS:$('useHTTPS').checked,
        Hostname:$('hostname').value,
        AuthKey:$('authKey').value,
        AcceptRoutes:$('acceptRoutes').checked
      })
    });
    $('msg').className = 'msg ok';
    $('msg').textContent = data.Message || '已提交连接请求。';
  }catch(e){
    $('msg').className = 'msg err';
    $('msg').textContent = e.message || String(e);
  }finally{
    $('connectBtn').disabled = false;
  }
}
async function logoutCurrent(){
  if(!confirm('退出当前网络后可以修改服务端配置，确定继续吗？')) return;
  $('logoutBtn').disabled = true;
  $('msg').className = 'msg';
  $('msg').textContent = '正在退出当前网络...';
  try{
    await getJSON('/api/logout', {method:'POST'});
    connected = false;
    setLocked(false);
    $('state').textContent = '当前状态：未配置';
    $('msg').className = 'msg ok';
    $('msg').textContent = '已退出当前网络，现在可以修改服务端配置。';
  }catch(e){
    $('msg').className = 'msg err';
    $('msg').textContent = e.message || String(e);
  }finally{
    $('logoutBtn').disabled = false;
  }
}
async function copyCommand(){
  refreshPreview();
  try{
    await navigator.clipboard.writeText($('commandLine').value);
    $('msg').className = 'msg ok';
    $('msg').textContent = '命令已复制。';
  }catch(e){
    $('commandLine').select();
    document.execCommand('copy');
  }
}
['serverIP','serverPort','useHTTPS','hostname','acceptRoutes','authKey'].forEach(id => $(id).addEventListener('input', refreshPreview));
$('connectBtn').addEventListener('click', connect);
$('logoutBtn').addEventListener('click', logoutCurrent);
$('copyBtn').addEventListener('click', copyCommand);
$('closeBtn').addEventListener('click', () => window.close());
load();
</script>
</body>
</html>`

const dashboardHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ScaleTail 仪表台</title>
<style>
:root{color-scheme:light;--bg:#f5f7fb;--panel:#fff;--text:#172033;--muted:#667085;--line:#d8dee9;--blue:#2563eb;--green:#16a34a;--red:#dc2626;--amber:#d97706}
*{box-sizing:border-box}body{margin:0;background:var(--bg);font-family:"Segoe UI",system-ui,sans-serif;color:var(--text)}.wrap{max-width:1180px;margin:0 auto;padding:24px}
.bar{display:flex;align-items:center;justify-content:space-between;gap:14px;margin-bottom:18px}.title{display:flex;align-items:center;gap:10px}.dot{width:12px;height:12px;border-radius:50%;background:#98a2b3}.dot.Running{background:var(--green)}.dot.Starting{background:var(--amber)}.dot.NeedsLogin,.dot.NoState,.dot.Stopped{background:var(--red)}
h1{font-size:22px;margin:0}.sub{color:var(--muted);font-size:13px;margin-top:3px}.actions{display:flex;gap:8px;flex-wrap:wrap}.btn{border:1px solid var(--line);background:#fff;color:var(--text);height:36px;padding:0 12px;border-radius:6px;font-size:13px;cursor:pointer;text-decoration:none;display:inline-flex;align-items:center}.btn.primary{border-color:var(--blue);background:var(--blue);color:#fff}.btn.danger{border-color:#fecaca;color:var(--red)}
.cards{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px;margin-bottom:12px}.card{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:16px}.label{font-size:12px;color:var(--muted);margin-bottom:6px}.value{font-size:18px;font-weight:700;word-break:break-word}.mono{font-family:Consolas,monospace}.grid{display:grid;grid-template-columns:1fr 1fr;gap:12px}.panel{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:16px;margin-bottom:12px}
h2{font-size:15px;margin:0 0 12px}.rows{display:grid;grid-template-columns:150px 1fr;gap:8px;font-size:13px}.muted{color:var(--muted)}table{width:100%;border-collapse:collapse;font-size:13px}th,td{text-align:left;border-bottom:1px solid #eef2f7;padding:9px 8px;vertical-align:top}th{color:var(--muted);font-weight:600}.err{color:var(--red);font-size:13px;margin-top:8px}.ok{color:var(--green)}.mini{font-size:12px;color:var(--muted);margin-top:8px}.exit-form{display:flex;gap:8px;align-items:center}select{height:36px;border:1px solid var(--line);border-radius:6px;padding:0 10px;background:#fff;color:var(--text);font-size:13px;min-width:260px;max-width:100%}
@media(max-width:900px){.cards,.grid{grid-template-columns:1fr}.bar{align-items:flex-start;flex-direction:column}}
</style>
</head>
<body>
<div class="wrap">
  <div class="bar">
    <div class="title">
      <div class="dot" id="dot"></div>
      <div>
        <h1>ScaleTail 仪表台</h1>
        <div class="sub" id="summary">读取中...</div>
      </div>
    </div>
    <div class="actions">
      <a class="btn primary" href="/connect">服务端设置</a>
      <button class="btn" id="netcheckBtn">运行网络检测</button>
      <button class="btn" id="refreshBtn">刷新</button>
      <button class="btn danger" id="disconnectBtn">断开</button>
    </div>
  </div>
  <div class="cards">
    <div class="card"><div class="label">状态</div><div class="value" id="state">-</div></div>
    <div class="card"><div class="label">IPv4</div><div class="value mono" id="ipv4">-</div></div>
    <div class="card"><div class="label">总接收</div><div class="value" id="rx">0 B</div></div>
    <div class="card"><div class="label">总发送</div><div class="value" id="tx">0 B</div></div>
    <div class="card"><div class="label">出口节点</div><div class="value" id="exitNode">未使用</div></div>
  </div>
  <div class="grid">
    <div class="panel">
      <h2>本机</h2>
      <div class="rows" id="selfRows"></div>
    </div>
    <div class="panel">
      <h2>连接配置</h2>
      <div class="rows" id="prefsRows"></div>
    </div>
  </div>
  <div class="panel">
    <h2>出口节点</h2>
    <div class="exit-form">
      <select id="exitNodeSelect"></select>
      <button class="btn primary" id="applyExitNodeBtn">应用</button>
    </div>
    <div class="mini" id="exitNodeMsg">选择后点击应用。</div>
  </div>
  <div class="panel">
    <h2>网络检测 (netcheck)</h2>
    <div class="rows" id="netcheckRows"><div class="muted">状态</div><div>尚未运行</div></div>
    <div class="err" id="netcheckErr"></div>
  </div>
  <div class="panel">
    <h2>节点</h2>
    <table>
      <thead><tr><th>状态</th><th>名称</th><th>IP</th><th>流量</th><th>连接</th></tr></thead>
      <tbody id="peers"></tbody>
    </table>
  </div>
</div>
<script>
const $ = id => document.getElementById(id);
if(!initPanelWindow()){ throw new Error('panel already open'); }
let exitNodeTouched = false;
let lastExitNodeSignature = '';
function initPanelWindow(){
  const idKey = 'scaletail-panel-id';
  const activeKey = 'scaletail-panel-active';
  const commandKey = 'scaletail-panel-command';
  let id = sessionStorage.getItem(idKey);
  if(!id){ id = Date.now() + '-' + Math.random(); sessionStorage.setItem(idKey, id); }
  try{
    const active = JSON.parse(localStorage.getItem(activeKey) || '{}');
    if(active.id && active.id !== id && Date.now() - active.ts < 5000){
      localStorage.setItem(commandKey, JSON.stringify({id, target:location.pathname, ts:Date.now()}));
      document.body.innerHTML = '<div class="wrap"><div class="panel"><h1>仪表台已打开</h1><p>已有一个 ScaleTail 窗口正在运行，本窗口会自动关闭。</p></div></div>';
      setTimeout(() => window.close(), 120);
      return false;
    }
  }catch(e){}
  const beat = () => localStorage.setItem(activeKey, JSON.stringify({id, ts: Date.now()}));
  beat();
  setInterval(beat, 1000);
  window.addEventListener('beforeunload', () => {
    try{
      const active = JSON.parse(localStorage.getItem(activeKey) || '{}');
      if(active.id === id) localStorage.removeItem(activeKey);
    }catch(e){}
  });
  window.addEventListener('storage', ev => {
    if(ev.key !== commandKey || !ev.newValue) return;
    try{
      const cmd = JSON.parse(ev.newValue);
      if(cmd.id === id || Date.now() - cmd.ts > 5000) return;
      if(cmd.target && location.pathname !== cmd.target) location.href = cmd.target;
      else window.focus();
    }catch(e){}
  });
  setInterval(async () => {
    try{ await fetch('/api/ping', {cache:'no-store'}); }
    catch(e){
      document.body.innerHTML = '<div class="wrap"><div class="panel"><h1>托盘程序已退出</h1><p>本地控制面板已停止，本窗口会自动关闭。</p></div></div>';
      setTimeout(() => window.close(), 300);
    }
  }, 2500);
  return true;
}
function stateLabel(state){
  const labels = {
    Running: '已连接',
    Starting: '正在连接',
    NeedsLogin: '需要认证',
    NeedsMachineAuth: '等待设备授权',
    NoState: '未配置',
    Stopped: '已断开'
  };
  return labels[state] || state || '-';
}
function fmtBytes(n){
  n = Number(n || 0);
  const units = ['B','KB','MB','GB','TB'];
  let i = 0;
  while(n >= 1024 && i < units.length - 1){ n = n / 1024; i++; }
  return n.toFixed(i === 0 || n >= 10 ? 0 : 1) + ' ' + units[i];
}
function fmtLatency(v){
  if(v === undefined || v === null) return '-';
  if(typeof v === 'number') return Math.round(v / 1000000) + ' ms';
  return String(v);
}
function rows(el, items){
  el.innerHTML = items.map(x => '<div class="muted">' + x[0] + '</div><div>' + (x[1] || '-') + '</div>').join('');
}
function nodeName(p){
  return p.HostName || (p.DNSName ? p.DNSName.split('.')[0] : '') || p.ID || '-';
}
function exitNodeLabel(st, peers){
  const cur = st.ExitNodeStatus;
  if(!cur || !cur.ID) return '未使用';
  const p = peers.find(x => x.ID === cur.ID);
  const name = p ? nodeName(p) : cur.ID;
  return cur.Online === false ? name + ' (离线)' : name;
}
function renderExitNodeOptions(st, peers){
  const options = [{id:'', label:'不使用出口节点', disabled:false}];
  peers.filter(p => p.ExitNodeOption).sort((a,b) => nodeName(a).localeCompare(nodeName(b))).forEach(p => {
    options.push({id:String(p.ID || ''), label:nodeName(p) + (p.Online ? '' : ' (离线)'), disabled:!p.Online});
  });
  const current = st.ExitNodeStatus && st.ExitNodeStatus.ID ? String(st.ExitNodeStatus.ID) : '';
  const sig = current + '|' + options.map(o => o.id + ':' + o.label + ':' + o.disabled).join('|');
  if(exitNodeTouched && sig === lastExitNodeSignature) return;
  lastExitNodeSignature = sig;
  $('exitNodeSelect').innerHTML = options.map(o => '<option value="' + o.id + '"' + (o.disabled ? ' disabled' : '') + (o.id === current ? ' selected' : '') + '>' + o.label + '</option>').join('');
  $('applyExitNodeBtn').disabled = options.length <= 1;
  if(options.length <= 1) $('exitNodeMsg').textContent = '当前没有可用出口节点。';
}
async function getJSON(path, opts){
  const res = await fetch(path, opts);
  const data = await res.json();
  if(!res.ok) throw new Error(data.Error || res.statusText);
  return data;
}
async function refresh(manual){
  const btn = $('refreshBtn');
  const oldText = btn.textContent;
  if(manual){
    btn.disabled = true;
    btn.textContent = '刷新中...';
  }
  try{
    const st = await getJSON('/api/status');
    const prefs = await getJSON('/api/prefs').catch(() => ({}));
    const state = st.BackendState || '-';
    const stateText = stateLabel(state);
    $('dot').className = 'dot ' + state;
    $('state').textContent = stateText;
    $('summary').textContent = (st.CurrentTailnet && st.CurrentTailnet.Name ? st.CurrentTailnet.Name : '未加入网络') + ' / ' + stateText;
    const self = st.Self || {};
    const ips = self.ScaleTailIPs || [];
    const peers = Object.values(st.Peer || {});
    const total = peers.reduce((acc, p) => {
      acc.rx += Number(p.RxBytes || 0);
      acc.tx += Number(p.TxBytes || 0);
      return acc;
    }, {rx:0, tx:0});
    $('ipv4').textContent = ips[0] || '-';
    $('rx').textContent = fmtBytes(total.rx);
    $('tx').textContent = fmtBytes(total.tx);
    $('exitNode').textContent = exitNodeLabel(st, peers);
    rows($('selfRows'), [
      ['主机名', self.HostName],
      ['DNS 名称', self.DNSName],
      ['ScaleTail IP', ips.join(', ')],
      ['在线', self.Online === false ? '否' : '是']
    ]);
    rows($('prefsRows'), [
      ['控制服务器', prefs.ControlURL || '-'],
      ['自定义设备名', prefs.Hostname || '使用系统主机名'],
      ['期望连接', prefs.WantRunning ? '是' : '否'],
      ['接受路由', prefs.RouteAll ? '是' : '否']
    ]);
    renderExitNodeOptions(st, peers);
    $('peers').innerHTML = peers.length ? peers.map(p => {
      const name = nodeName(p);
      const ip = (p.ScaleTailIPs || []).join(', ');
      const traffic = '接收 ' + fmtBytes(p.RxBytes) + ' / 发送 ' + fmtBytes(p.TxBytes);
      const conn = p.Relay ? ('DERP ' + p.Relay) : (p.CurAddr || '-');
      return '<tr><td>' + (p.Online ? '<span class="ok">在线</span>' : '离线') + '</td><td>' + name + '</td><td class="mono">' + ip + '</td><td>' + traffic + '</td><td>' + conn + '</td></tr>';
    }).join('') : '<tr><td colspan="5" class="muted">暂无节点</td></tr>';
  }catch(e){
    $('summary').textContent = '无法连接 scaletaild：' + e.message;
  }finally{
    if(manual){
      btn.disabled = false;
      btn.textContent = '已刷新 ' + new Date().toLocaleTimeString();
      setTimeout(() => { if(btn.textContent.startsWith('已刷新')) btn.textContent = oldText; }, 1200);
    }
  }
}
async function applyExitNode(){
  $('applyExitNodeBtn').disabled = true;
  $('exitNodeMsg').textContent = '正在应用出口节点...';
  try{
    await getJSON('/api/exit-node', {
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ID:$('exitNodeSelect').value})
    });
    exitNodeTouched = false;
    $('exitNodeMsg').textContent = '出口节点设置已提交。';
    await refresh(false);
  }catch(e){
    $('exitNodeMsg').textContent = e.message || String(e);
  }finally{
    $('applyExitNodeBtn').disabled = false;
  }
}
async function runNetcheck(){
  $('netcheckBtn').disabled = true;
  $('netcheckErr').textContent = '';
  rows($('netcheckRows'), [['状态','运行中...']]);
  try{
    const r = await getJSON('/api/netcheck', {method:'POST'});
    const lat = r.RegionLatency || {};
    const regions = Object.keys(lat).slice(0, 8).map(k => 'DERP ' + k + ': ' + fmtLatency(lat[k])).join('<br>');
    rows($('netcheckRows'), [
      ['时间', r.Now || '-'],
      ['UDP', r.UDP ? '可用' : '不可用'],
      ['IPv4', r.GlobalV4 || '-'],
      ['IPv6', r.GlobalV6 || (r.IPv6 ? '可用' : '不可用')],
      ['NAT 映射变化', r.MappingVariesByDestIP ? '是' : '否'],
      ['首选 DERP', r.PreferredDERP || '-'],
      ['延迟', regions || '-'],
      ['强制门户', r.CaptivePortal || '-']
    ]);
  }catch(e){
    rows($('netcheckRows'), [['状态','失败']]);
    $('netcheckErr').textContent = e.message;
  }finally{
    $('netcheckBtn').disabled = false;
  }
}
async function disconnect(){
  if(!confirm('确定要断开 ScaleTail 吗？')) return;
  await getJSON('/api/disconnect', {method:'POST'}).catch(e => alert(e.message));
  refresh(false);
}
$('refreshBtn').addEventListener('click', () => refresh(true));
$('netcheckBtn').addEventListener('click', runNetcheck);
$('disconnectBtn').addEventListener('click', disconnect);
$('exitNodeSelect').addEventListener('change', () => { exitNodeTouched = true; });
$('applyExitNodeBtn').addEventListener('click', applyExitNode);
refresh(false);
setInterval(() => refresh(false), 2500);
</script>
</body>
</html>`
