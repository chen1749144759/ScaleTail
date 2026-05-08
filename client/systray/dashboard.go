// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package systray

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
)

// DashboardServer serves the management dashboard.
type DashboardServer struct {
	lc   *local.Client
	http *http.Server
	port int
	mu   sync.Mutex
}

// StartDashboard creates and starts a dashboard server, returning the URL.
func StartDashboard(lc *local.Client) (string, error) {
	if lc == nil {
		lc = &local.Client{}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("dashboard listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	ds := &DashboardServer{
		lc:   lc,
		port: port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ds.serveHTML)
	mux.HandleFunc("/api/status", ds.serveStatus)

	ds.http = &http.Server{Handler: mux}
	go func() {
		if err := ds.http.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard server: %v", err)
		}
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	return url, nil
}

// Shutdown stops the dashboard server.
func (ds *DashboardServer) Shutdown() {
	if ds.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ds.http.Shutdown(ctx)
	}
}

func (ds *DashboardServer) serveStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := ds.lc.Status(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(status)
}

func (ds *DashboardServer) serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, dashboardHTML)
}

// dashboardHTML is a self-contained single-page management dashboard.
// It fetches /api/status every 2 seconds and renders:
//   - Connection status
//   - Self node info (hostname, IPs)
//   - Peer table (name, user, IPs, received routes, traffic, online status)
//   - Real-time traffic graph
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Tailscale Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Segoe UI',system-ui,sans-serif;background:#0d1117;color:#c9d1d9;min-height:100vh}
.header{background:#161b22;border-bottom:1px solid #30363d;padding:16px 24px;display:flex;align-items:center;gap:12px}
.status-dot{width:12px;height:12px;border-radius:50%;flex-shrink:0}
.status-dot.running{background:#3fb950;box-shadow:0 0 6px #3fb950}
.status-dot.offline{background:#f85149}
.status-dot.starting{background:#d29922;animation:pulse 1s infinite}
@keyframes pulse{50%{opacity:0.4}}
.tailnet-name{font-size:13px;color:#8b949e}
.main{padding:24px;max-width:1400px;margin:0 auto}
.self-panel{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:20px;margin-bottom:24px}
.self-panel h2{font-size:15px;color:#58a6ff;margin-bottom:12px}
.self-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px}
.self-item{font-size:13px}
.self-item .label{color:#8b949e;display:block;margin-bottom:2px}
.self-item .value{color:#c9d1d9;font-family:'Cascadia Code',Consolas,monospace}
.traffic-chart{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:20px;margin-bottom:24px}
.traffic-chart h2{font-size:15px;color:#58a6ff;margin-bottom:12px}
.traffic-legend{display:flex;gap:20px;margin-bottom:8px;font-size:13px}
.traffic-legend .in{color:#58a6ff}
.traffic-legend .out{color:#3fb950}
canvas{width:100%;height:200px;border-radius:4px;background:#0d1117}
.peer-panel{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:20px}
.peer-panel h2{font-size:15px;color:#58a6ff;margin-bottom:16px}
table{width:100%;border-collapse:collapse;font-size:13px}
th{text-align:left;padding:10px 12px;border-bottom:1px solid #30363d;color:#8b949e;font-weight:600;white-space:nowrap;cursor:pointer;user-select:none}
th:hover{color:#c9d1d9}
th .sort-arrow{margin-left:4px;font-size:10px}
td{padding:8px 12px;border-bottom:1px solid #21262d}
tr:hover td{background:#1c2129}
.peer-online{color:#3fb950}
.peer-offline{color:#8b949e}
.peer-traffic{font-family:'Cascadia Code',Consolas,monospace;font-size:12px}
.peer-routes{max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:11px;color:#8b949e}
.peer-name{font-weight:500}
.peer-user{color:#8b949e;font-size:12px}
.empty-state{text-align:center;padding:40px;color:#484f58;font-size:14px}
.summary-bar{display:flex;gap:24px;margin-bottom:16px;font-size:13px}
.summary-item{color:#8b949e}
.summary-item strong{color:#c9d1d9}
</style>
</head>
<body>

<div class="header">
  <div id="statusDot" class="status-dot offline"></div>
  <div>
    <div id="statusText" style="font-size:16px;font-weight:600">Disconnected</div>
    <div id="tailnetName" class="tailnet-name"></div>
  </div>
</div>

<div class="main">
  <div class="self-panel" id="selfPanel">
    <h2>This Device</h2>
    <div class="self-grid" id="selfGrid"></div>
  </div>

  <div class="traffic-chart">
    <h2>Traffic</h2>
    <div class="traffic-legend">
      <span class="in">▼ In: <strong id="trafficIn">0 B/s</strong></span>
      <span class="out">▲ Out: <strong id="trafficOut">0 B/s</strong></span>
    </div>
    <canvas id="trafficCanvas"></canvas>
  </div>

  <div class="peer-panel">
    <h2>Peers</h2>
    <div class="summary-bar">
      <span class="summary-item">Total: <strong id="peerCount">0</strong></span>
      <span class="summary-item">Online: <strong id="peerOnline">0</strong></span>
      <span class="summary-item">Exit Node: <strong id="exitNodeName">none</strong></span>
    </div>
    <table>
      <thead>
        <tr>
          <th>Status</th>
          <th onclick="sortTable('name')">Name <span class="sort-arrow">⇅</span></th>
          <th onclick="sortTable('user')">User <span class="sort-arrow">⇅</span></th>
          <th>Tailscale IPs</th>
          <th>Routes</th>
          <th onclick="sortTable('rx')">Rx <span class="sort-arrow">⇅</span></th>
          <th onclick="sortTable('tx')">Tx <span class="sort-arrow">⇅</span></th>
          <th>Connection</th>
        </tr>
      </thead>
      <tbody id="peerBody"></tbody>
    </table>
    <div class="empty-state" id="peerEmpty">No peers connected</div>
  </div>
</div>

<script>
let status = null;
let trafficHistory = [];
const MAX_HISTORY = 60;
const POLL_INTERVAL = 2000;
let sortCol = 'name';
let sortAsc = true;

function formatBytes(n) {
  if (typeof n !== 'number' || !isFinite(n)) n = 0;
  if (n < 0) n = 0;
  const units = ['B','KB','MB','GB','TB'];
  let i = 0;
  while (n >= 1024 && i < units.length-1) { n/=1024; i++; }
  return n.toFixed(i===0?0:1)+' '+units[i];
}

function formatRate(bps) {
  return formatBytes(bps)+'/s';
}

function formatIPs(ips) {
  if (!ips||!ips.length) return '—';
  return ips.join(', ');
}

function formatRoutes(routes) {
  if (!routes||!routes.length) return '—';
  return routes.join(', ');
}

function ago(t) {
  if (!t) return 'never';
  var d = (Date.now() - new Date(t).getTime())/1000;
  if (d<60) return Math.round(d)+'s ago';
  if (d<3600) return Math.round(d/60)+'m ago';
  if (d<86400) return Math.round(d/3600)+'h ago';
  return Math.round(d/86400)+'d ago';
}

async function fetchStatus() {
  try {
    var r = await fetch('/api/status');
    if (!r.ok) throw new Error(r.statusText);
    status = await r.json();
    updateUI();
  } catch(e) {
    console.error('fetch status:', e);
  }
}

function updateUI() {
  updateHeader();
  updateSelf();
  updatePeers();
  updateTraffic();
}

function updateHeader() {
  var dot = document.getElementById('statusDot');
  var text = document.getElementById('statusText');
  var tailnet = document.getElementById('tailnetName');
  dot.className = 'status-dot';
  switch(status.BackendState) {
    case 'Running':
      dot.classList.add('running');
      text.textContent = 'Connected';
      break;
    case 'Starting':
      dot.classList.add('starting');
      text.textContent = 'Starting...';
      break;
    default:
      dot.classList.add('offline');
      text.textContent = 'Disconnected';
  }
  if (status.CurrentTailnet) {
    tailnet.textContent = status.CurrentTailnet.Name + (status.CurrentTailnet.MagicDNSEnabled ? ' (MagicDNS on)' : '');
  } else {
    tailnet.textContent = '';
  }
}

function updateSelf() {
  var grid = document.getElementById('selfGrid');
  if (!status.Self) { grid.innerHTML = '<div class="self-item"><span class="value">Not connected</span></div>'; return; }
  var s = status.Self;
  grid.innerHTML =
    item('Hostname', s.HostName) +
    item('DNS Name', s.DNSName || '—') +
    item('OS', s.OS || '—') +
    item('Tailscale IPs', formatIPs(s.TailscaleIPs)) +
    item('Node ID', (s.ID||'—')) +
    item('Online', s.Online ? '<span class="peer-online">Yes</span>' : '<span class="peer-offline">No</span>') +
    item('Last Handshake', s.LastHandshake ? new Date(s.LastHandshake).toLocaleString() : 'never');
}

function item(label, value) {
  return '<div class="self-item"><span class="label">'+label+'</span><span class="value">'+value+'</span></div>';
}

var peerData = [];
function updatePeers() {
  var body = document.getElementById('peerBody');
  var empty = document.getElementById('peerEmpty');
  peerData = [];
  if (!status.Peer) {
    body.innerHTML = '';
    empty.style.display = 'block';
    return;
  }

  var users = status.User || {};
  for (var k in status.Peer) {
    var p = status.Peer[k];
    var u = users[p.UserID] || {};
    peerData.push({
      key: k,
      online: p.Online,
      name: p.HostName || p.DNSName || k.substring(0,8),
      user: u.DisplayName || u.LoginName || '',
      ips: formatIPs(p.TailscaleIPs),
      routes: formatRoutes(p.PrimaryRoutes),
      rx: p.RxBytes||0,
      tx: p.TxBytes||0,
      curAddr: p.CurAddr || '',
      relay: p.Relay || '',
      exitNode: p.ExitNode || false,
      exitOption: p.ExitNodeOption || false,
      lastSeen: p.LastSeen
    });
  }

  peerData.sort(function(a,b) {
    var va = a[sortCol], vb = b[sortCol];
    if (typeof va === 'number') return sortAsc ? va-vb : vb-va;
    return sortAsc ? (''+va).localeCompare(''+vb) : (''+vb).localeCompare(''+va);
  });

  if (!peerData.length) {
    body.innerHTML = '';
    empty.style.display = 'block';
  } else {
    empty.style.display = 'none';
    body.innerHTML = peerData.map(function(p) {
      var statusClass = p.online ? 'peer-online' : 'peer-offline';
      var statusText = p.online ? '●' : '○';
      var conn = p.curAddr || (p.relay ? 'relay:'+p.relay : '—');
      var tags = [];
      if (p.exitNode) tags.push('[exit]');
      if (p.exitOption) tags.push('[exit-opt]');
      return '<tr>'+
        '<td class="'+statusClass+'">'+statusText+'</td>'+
        '<td class="peer-name">'+esc(p.name)+' '+tags.join(' ')+'</td>'+
        '<td class="peer-user">'+esc(p.user)+'</td>'+
        '<td class="peer-traffic">'+esc(p.ips)+'</td>'+
        '<td class="peer-routes" title="'+esc(p.routes)+'">'+esc(p.routes)+'</td>'+
        '<td class="peer-traffic">'+formatBytes(p.rx)+'</td>'+
        '<td class="peer-traffic">'+formatBytes(p.tx)+'</td>'+
        '<td>'+esc(conn)+'</td>'+
        '</tr>';
    }).join('');
  }

  document.getElementById('peerCount').textContent = peerData.length;
  document.getElementById('peerOnline').textContent = peerData.filter(function(p){return p.online}).length;

  var exitName = 'none';
  if (status.ExitNodeStatus && status.ExitNodeStatus.ID) {
    for (var k in status.Peer) {
      if (status.Peer[k].ExitNode) {
        exitName = status.Peer[k].HostName || status.ExitNodeStatus.ID;
        break;
      }
    }
    if (exitName === 'none') exitName = status.ExitNodeStatus.ID;
  }
  document.getElementById('exitNodeName').textContent = exitName;
}

function esc(s) { return (''+s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }

function sortTable(col) {
  if (sortCol === col) sortAsc = !sortAsc;
  else { sortCol = col; sortAsc = true; }
  updatePeers();
}

var prevRx = 0, prevTx = 0, prevTime = 0;
function updateTraffic() {
  var now = Date.now();
  var totalRx = 0, totalTx = 0;
  if (status.Peer) {
    for (var k in status.Peer) {
      totalRx += status.Peer[k].RxBytes||0;
      totalTx += status.Peer[k].TxBytes||0;
    }
  }

  if (prevTime > 0) {
    var dt = (now - prevTime) / 1000;
    var rxRate = dt>0 ? (totalRx - prevRx) / dt : 0;
    var txRate = dt>0 ? (totalTx - prevTx) / dt : 0;
    document.getElementById('trafficIn').textContent = formatRate(Math.max(0, rxRate));
    document.getElementById('trafficOut').textContent = formatRate(Math.max(0, txRate));
  }
  prevRx = totalRx; prevTx = totalTx; prevTime = now;

  trafficHistory.push({time: now, rx: totalRx, tx: totalTx});
  while (trafficHistory.length > MAX_HISTORY) trafficHistory.shift();
  drawTrafficChart();
}

function drawTrafficChart() {
  var canvas = document.getElementById('trafficCanvas');
  var ctx = canvas.getContext('2d');
  var W = canvas.parentElement.clientWidth - 40;
  var H = 200;
  canvas.width = W * (window.devicePixelRatio||1);
  canvas.height = H * (window.devicePixelRatio||1);
  canvas.style.width = W+'px';
  canvas.style.height = H+'px';
  ctx.scale(window.devicePixelRatio||1, window.devicePixelRatio||1);

  ctx.fillStyle = '#0d1117';
  ctx.fillRect(0, 0, W, H);

  if (trafficHistory.length < 2) {
    ctx.fillStyle = '#484f58';
    ctx.font = '13px system-ui';
    ctx.textAlign = 'center';
    ctx.fillText('Collecting data...', W/2, H/2);
    return;
  }

  // Grid lines
  var pad = {top:10, right:20, bottom:30, left:60};
  var pw = W - pad.left - pad.right;
  var ph = H - pad.top - pad.bottom;

  var allRx = trafficHistory.map(function(d){return d.rx});
  var allTx = trafficHistory.map(function(d){return d.tx});
  var minVal = 0;
  var maxVal = Math.max.apply(null, allRx.concat(allTx)) || 1;

  ctx.strokeStyle = '#21262d';
  ctx.lineWidth = 1;
  for (var i=1; i<4; i++) {
    var y = pad.top + (ph*i/4);
    ctx.beginPath();
    ctx.moveTo(pad.left, y);
    ctx.lineTo(W-pad.right, y);
    ctx.stroke();
  }

  // Y-axis labels
  ctx.fillStyle = '#8b949e';
  ctx.font = '10px system-ui';
  ctx.textAlign = 'right';
  for (var i=0; i<=4; i++) {
    var val = minVal + (maxVal-minVal)*(4-i)/4;
    ctx.fillText(formatBytes(val), pad.left-4, pad.top + (ph*i/4) + 4);
  }

  // X-axis labels (time)
  ctx.textAlign = 'center';
  var showEvery = Math.max(1, Math.floor(trafficHistory.length/6));
  for (var i=0; i<trafficHistory.length; i+=showEvery) {
    var t = new Date(trafficHistory[i].time);
    var x = pad.left + (pw*i/(trafficHistory.length-1||1));
    ctx.fillText(t.getHours()+':'+('0'+t.getMinutes()).slice(-2)+':'+('0'+t.getSeconds()).slice(-2), x, H-pad.bottom+16);
  }

  function x(i) { return pad.left + (pw*i/(trafficHistory.length-1||1)); }
  function y(v) { return pad.top + ph - (ph*(v-minVal)/(maxVal-minVal||1)); }

  // Rx line (blue)
  ctx.strokeStyle = '#58a6ff';
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(x(0), y(allRx[0]));
  for (var i=1; i<trafficHistory.length; i++) ctx.lineTo(x(i), y(allRx[i]));
  ctx.stroke();

  // Tx line (green)
  ctx.strokeStyle = '#3fb950';
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(x(0), y(allTx[0]));
  for (var i=1; i<trafficHistory.length; i++) ctx.lineTo(x(i), y(allTx[i]));
  ctx.stroke();
}

document.addEventListener('DOMContentLoaded', function() {
  fetchStatus();
  setInterval(fetchStatus, POLL_INTERVAL);
  window.addEventListener('resize', drawTrafficChart);
});
</script>
</body>
</html>`

// Ensure ipnstate types used in the template are referenced so the compiler
// knows we depend on the package.
var _ = (*ipnstate.Status)(nil)
