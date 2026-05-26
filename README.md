# ScaleTail

ScaleTail 是基于 [tailscale/tailscale](https://github.com/tailscale/tailscale) 定制的 Windows 客户端，主要面向自建 Headscale/Tailscale 控制服务器场景。

本项目保留 Tailscale 的核心网络能力，同时为 Windows 用户提供独立的 Electron 可视化客户端，让连接、状态查看、网络检测、出口节点和宣告路由等操作尽量不再依赖命令行。

> 当前 GitHub 仓库地址：<https://github.com/chen1749144759/ScaleTail>

## 项目定位

- 面向 Windows 桌面客户端。
- 适配自建 Headscale 控制服务器，支持填写服务端 IP/域名、端口、HTTP/HTTPS 和预认证密钥。
- 提供中文图形界面，降低新手使用门槛。
- 使用官方 `tailscaled` 作为核心后台服务，GUI 通过本地 LocalAPI 控制连接和状态。
- 安装包集成 `ScaleTail.exe`、`tailscaled.exe`、`tailscale-localapi.exe` 和 `wintun.dll`。

## 主要功能

### ScaleTail 图形客户端

Windows 桌面入口为 `ScaleTail.exe`，使用 Electron 38 + Vue 3 + TypeScript 实现。

界面包含：

- 仪表盘：查看连接状态、本机名称、自己 IP、节点数量、节点总流量和节点列表。
- 服务端设置：填写控制服务器地址、端口、HTTPS、设备名、预认证密钥和接受路由选项。
- 节点工具：设置出口节点、运行 `netcheck`、宣告本地子网路由。
- 托盘菜单：打开仪表盘、服务端设置、节点工具、刷新状态和退出托盘程序。

### 本地 LocalAPI Helper

Electron 主进程不直接访问 Windows named pipe，而是调用隐藏的 `tailscale-localapi.exe`。

这个 helper 使用 Go 官方 `client/local` 访问 LocalAPI，避免 Electron/Node 在 Windows named pipe impersonation 上遇到认证问题。

### netcheck LocalAPI

项目新增了 `/localapi/v0/netcheck`，GUI 的网络检测直接走 `tailscaled` LocalAPI，不再依赖打包 `tailscale.exe netcheck`。

### 宣告路由

节点页面支持填写 CIDR 路由，例如：

```text
192.168.10.0/24
10.10.0.0/16
```

提交后会通过 LocalAPI 写入 `AdvertiseRoutes`。如果使用 Headscale，仍需要在服务端管理后台批准该节点发布的路由。

## 架构说明

```text
ScaleTail.exe
  Electron GUI
  |
  | spawn
  v
tailscale-localapi.exe
  Go LocalAPI helper
  |
  | client/local
  v
tailscaled.exe
  Windows service / core networking
  |
  v
Headscale or Tailscale control server
```

Windows 服务名、LocalAPI 命名管道和 Electron 服务检测逻辑已经统一为 `ScaleTail`。安装器仍会清理旧版 `Tailscale` 服务和旧数据目录，避免覆盖安装或卸载时留下旧服务。

本仓库默认关闭上游 GitHub Actions 工作流，原 workflow 文件已移动到 `.github/workflows-disabled/`，避免每次提交都跑上游完整 CI。

## 构建环境

推荐在 Windows 上构建：

- Go
- Node.js + npm
- Inno Setup 6
- PowerShell

默认脚本会把 npm/Electron 依赖缓存放到：

```text
D:\workspace-qoder\deps
```

如果 Inno Setup 不在 `D:\Inno Setup 6\ISCC.exe`，可以设置 `ISCC` 环境变量指向 `ISCC.exe`。

## 构建 Windows 安装包

在项目根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows-installer.ps1
```

脚本会完成：

- 编译 `tailscaled.exe`
- 编译 `tailscale-localapi.exe`
- 准备 `wintun.dll`
- 安装并构建 Electron GUI
- 生成 Inno Setup 安装包

输出文件：

```text
dist\installer\scaletail-0.0.1-windows-amd64-setup-custom.exe
```

## 开发 Electron 客户端

```powershell
cd client\electron
npm ci
npm run typecheck
npm run build
```

本地调试渲染层：

```powershell
npm run dev
```

Electron 主进程默认会从打包后的路径加载资源；完整联调建议直接使用安装包或 `scripts\build-windows-installer.ps1` 生成的 `dist\electron\win-unpacked\ScaleTail.exe`。

## 安装和卸载

安装包会：

- 安装 `ScaleTail.exe`
- 安装并启动 `ScaleTail` 系统服务，服务进程仍由 `tailscaled.exe` 提供核心网络能力
- 打包 `tailscale-localapi.exe`
- 打包 `wintun.dll`
- 可选创建桌面快捷方式
- 创建开始菜单和开机启动项

卸载和覆盖安装前会强制关闭旧进程，包括：

- `ScaleTail.exe`
- `Tailscale.exe`
- `tailscaled.exe`
- `tailscale-localapi.exe`
- 旧版 `tailscale-systray.exe`
- 旧版 `tailscale.exe` / `tailscale-cli.exe`

卸载时会清理安装目录、`ScaleTail` 数据目录，以及旧版 `Tailscale` 数据目录。

## Headscale 使用提示

连接页面等价于执行类似命令：

```powershell
tailscale up --login-server=http://YOUR_SERVER:PORT --accept-routes=true --auth-key=YOUR_KEY
```

但实际操作由 GUI 调用 LocalAPI 完成，不需要用户手动打开 CMD。

如果使用宣告路由，Headscale 服务端需要批准路由，例如：

```bash
headscale nodes list-routes
headscale nodes approve-routes --identifier <node-id> --routes 192.168.10.0/24
```

## 与上游的关系

ScaleTail 基于 Tailscale 源码定制，核心网络功能仍来自上游项目。许可证保持与上游一致。

上游项目：

- <https://github.com/tailscale/tailscale>
