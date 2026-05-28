# ScaleTail

[![Release](https://img.shields.io/github/v/release/chen1749144759/ScaleTail?sort=semver&label=release)](https://github.com/chen1749144759/ScaleTail/releases)
[![Download](https://img.shields.io/github/downloads/chen1749144759/ScaleTail/total?label=downloads)](https://github.com/chen1749144759/ScaleTail/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20amd64-0078D4?logo=windows&logoColor=white)](https://github.com/chen1749144759/ScaleTail/releases)
[![Go](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-38-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

ScaleTail 是基于 [tailscale/tailscale](https://github.com/tailscale/tailscale) 定制的 Windows 客户端，主要面向自建 Headscale/Tailscale 控制服务器场景。

本项目保留 Tailscale 的核心网络能力，同时提供独立的 Electron 图形客户端，让连接、状态查看、网络检测、出口节点和宣告路由等操作尽量不再依赖命令行。

> 当前仓库：<https://github.com/chen1749144759/ScaleTail>

## 标签

`tailscale` `headscale` `wireguard` `vpn` `windows` `windows-client` `electron` `vue3` `typescript` `go` `localapi` `wintun`

## 当前发布

最新版本：[`v0.0.1`](https://github.com/chen1749144759/ScaleTail/releases/tag/v0.0.1)

Windows amd64 安装包：

```text
scaletail-0.0.1-windows-amd64-setup-custom.exe
```

SHA256：

```text
5c748480a6395a40d217336f53c03596b2aa40c7ba6e04b2b723d6bd16f78b81
```

## 功能

- 图形化连接 Headscale/Tailscale 控制服务器，支持服务端 IP/域名、端口、HTTP/HTTPS、设备名、预认证密钥和接受路由选项。
- 仪表盘查看连接状态、本机名称、本机 IP、节点数量、节点列表和节点流量。
- 节点工具支持出口节点选择、`netcheck` 网络检测、宣告本地子网路由。
- 托盘菜单支持打开仪表盘、服务端设置、节点工具、刷新状态和退出托盘程序。
- Electron 主进程通过隐藏的 Go LocalAPI helper 访问本地 `tailscaled`，避免 Windows named pipe impersonation 兼容问题。
- 新增 `/localapi/v0/netcheck`，网络检测直接走 `tailscaled` LocalAPI，不再依赖命令行 `tailscale.exe netcheck`。
- 安装器集成 `ScaleTail.exe`、`tailscaled.exe`、`tailscale-localapi.exe` 和 `wintun.dll`。

## 架构

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

Electron 主进程默认会从打包后的路径加载资源；完整联调建议直接使用安装包，或使用 `scripts\build-windows-installer.ps1` 生成的 `dist\electron\win-unpacked\ScaleTail.exe`。

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

实际操作由 GUI 调用 LocalAPI 完成，不需要用户手动打开 CMD。

如果使用宣告路由，Headscale 服务端仍需要批准该节点发布的路由，例如：

```bash
headscale nodes list-routes
headscale nodes approve-routes --identifier <node-id> --routes 192.168.10.0/24
```

## Linux 包说明

Linux amd64 包可以通过 WSL 构建。默认会同时生成核心包和可选 GUI 包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-linux-packages-wsl.ps1 -Distro Ubuntu-24.04 -Version 0.0.1
```

如果只给服务器使用，不需要桌面托盘和可视化入口，可以跳过 GUI 包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-linux-packages-wsl.ps1 -Distro Ubuntu-24.04 -Version 0.0.1 -SkipGui
```

输出目录：

```text
dist\linux-v0.0.1\
  scaletail_0.0.1_amd64.deb
  scaletail_0.0.1_x86_64.rpm
  scaletail_0.0.1_amd64.tgz
  scaletail-gui_0.0.1_all.deb
  scaletail-gui_0.0.1_noarch.rpm
  SHA256SUMS-linux-amd64.txt
```

Linux 核心包仍复用上游 Linux 运行方式：命令为 `/usr/bin/tailscale`，服务为 `tailscaled.service`。包名改为 `scaletail`，并声明与官方 `tailscale` 包冲突，避免同一台机器同时安装导致文件和 systemd 单元冲突。

Debian/Ubuntu 服务器只安装核心包：

```bash
sudo apt install ./scaletail_0.0.1_amd64.deb
sudo systemctl enable --now tailscaled
```

有桌面环境时再额外安装 GUI 包：

```bash
sudo apt install ./scaletail-gui_0.0.1_all.deb
systemctl --user enable --now scaletail-systray.service
```

RPM 系发行版同理，服务器只装 `scaletail_0.0.1_x86_64.rpm`，桌面环境再装 `scaletail-gui_0.0.1_noarch.rpm`。

## macOS 包说明

macOS 未签名安装包通过 GitHub Actions 的 macOS runner 构建。该 workflow 只支持手动触发，不会在 push 或 PR 时自动运行：

```text
Actions -> Build macOS package -> Run workflow
```

输入版本号后会分别生成：

```text
ScaleTail-0.0.1-darwin-amd64.pkg
ScaleTail-0.0.1-darwin-arm64.pkg
scaletail_0.0.1_darwin_amd64.tar.gz
scaletail_0.0.1_darwin_arm64.tar.gz
```

当前 macOS 包是核心 daemon/CLI 安装包，不包含 Electron GUI。包内安装路径复用上游 macOS 运行方式：

- `/usr/local/bin/tailscale`
- `/usr/local/bin/tailscaled`
- `/Library/LaunchDaemons/com.tailscale.tailscaled.plist`
- `/Library/Tailscale`

未签名 `.pkg` 适合内部测试。正式公开分发时，还需要 Apple Developer ID 证书签名和 notarization。

## 与上游的关系

ScaleTail 基于 Tailscale 源码定制，核心网络功能仍来自上游项目。许可证保持与上游一致。

上游项目：

- <https://github.com/tailscale/tailscale>
