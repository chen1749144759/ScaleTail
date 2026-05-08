# tailscale 定制版

基于 [tailscale/tailscale](https://github.com/tailscale/tailscale) 分支自主演进的定制版本，
主要适配 **headscale** 自建控制服务器场景。

## 与上游的主要差异

### 1. 控制协议端口修复

`control/ts2021/client.go` — 修复非标准端口场景下的 HTTPS fallback 错误。

上游代码在 `http://IP:PORT` 且 PORT≠80 时，会将 `httpsPort` 硬编码为 `"443"`。
重连时（2 分钟内再次拨号），`forceNoise443()` 触发后直接跳到 443 端口，而此时
headscale 运行在非标准端口（如 `:60098`），导致连接失败。

本修改将非标准端口场景下的 `httpsPort` 设为 `controlhttp.NoPort`，彻底禁用
HTTPS fallback，确保重连始终使用正确的 HTTP 端口。

```go
// 修改前 (control/ts2021/client.go:142)
httpsPort = "443"

// 修改后
if port == "80" {
    httpsPort = "443"          // 标准端口保留 fallback
} else {
    httpsPort = controlhttp.NoPort  // 非标准端口禁用 fallback
}
```

同时修复了私有主机（`isPrivateHost`）场景从空字符串改为 `NoPort`，避免污染默认逻辑。

### 2. 系统托盘仪表盘

在 Windows 系统托盘（systray）中新增"Dashboard"菜单项和配套功能：

| 文件 | 说明 |
|------|------|
| `client/systray/dashboard.go` | 嵌入式 HTTP 仪表盘服务，提供流量图、节点列表、路由信息面板 |
| `client/systray/dashboard_open.go` | 跨平台浏览器启动器，自动打开仪表盘页面 |
| `client/systray/systray.go` | 新增 Dashboard 菜单项；tailscaled 进入 Running 状态时自动打开仪表盘 |

仪表盘运行在 `localhost:5252`，界面包含：
- 实时流量折线图
- 已连接节点表格（含 IP、状态、出口节点标记）
- 子网路由列表

### 3. Web 客户端配置界面

`client/web/` — 将上游的 Tailscale OAuth 登录界面替换为适配 headscale 的配置表单。

| 文件 | 变更 |
|------|------|
| `client/web/src/components/views/login-view.tsx` | 重写为 `ServerConfigView`：提供 IP、端口、HTTPS 开关、预认证密钥输入框 |
| `client/web/src/components/views/disconnected-view.tsx` | 注销后提供"返回配置"按钮，而非上游的无操作页面 |
| `client/web/build/index.html` | 构建产物更新 |

功能特性：
- GUI 配置控制服务器 URL（无需命令行 `tailscale up --login-server=...`）
- 实时 URL 预览
- 支持预认证密钥（免浏览器认证流程）
- 支持"重新认证"操作
- 全中文界面

## 构建

```bash
# 需要 Go 1.26+
go install tailscale.com/cmd/tailscale{,d}

# 或使用构建脚本（含版本信息）
./build_dist.sh tailscale.com/cmd/tailscale
./build_dist.sh tailscale.com/cmd/tailscaled
```

## 使用场景

本版本专为 **自建 headscale 控制服务器** 场景优化，典型部署：

```
┌─────────────┐     HTTP (非标准端口)     ┌──────────────┐
│  tailscaled │ ◄──────────────────────► │  headscale    │
│  (本客户端)  │    http://IP:60098       │  (控制服务器)  │
└─────────────┘                          └──────────────┘
```

首次使用时通过 Web 客户端（`http://100.100.100.100`）填写服务器信息，
或通过 CLI：

```bash
tailscale up --login-server=http://YOUR_IP:PORT
```

## 开源协议

BSD 3-Clause License — 与上游保持一致。

原始项目：[tailscale/tailscale](https://github.com/tailscale/tailscale)
