# ScaleTail

[![Release](https://img.shields.io/github/v/release/chen1749144759/ScaleTail?sort=semver&label=release)](https://github.com/chen1749144759/ScaleTail/releases)
[![Download](https://img.shields.io/github/downloads/chen1749144759/ScaleTail/total?label=downloads)](https://github.com/chen1749144759/ScaleTail/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20amd64-0078D4?logo=windows&logoColor=white)](https://github.com/chen1749144759/ScaleTail/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-38-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Upstream Core](https://img.shields.io/badge/Tailscale_Core-v1.98.5-4D7CFE)](https://github.com/tailscale/tailscale/releases/tag/v1.98.5)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

ScaleTail 是基于 [tailscale/tailscale](https://github.com/tailscale/tailscale) 定制的图形化客户端，主要面向自建 Headscale/ScaleTail 控制服务器场景。项目目标是让新手不再依赖 CMD：连接、状态查看、网络检测、出口节点、宣告路由等常用操作都可以在内置窗口里完成。

仓库地址：https://github.com/chen1749144759/ScaleTail

## 当前对标版本

- ScaleTail 产品版本按自身节奏发布，当前稳定包仍以 `v0.0.1` 为基线。
- 核心网络代码对标官方 Tailscale `v1.98.5`，保留 ScaleTail 自身的命名、服务、安装包和 Electron 图形界面改造。
- 当前维护方式是“手工审计 + 定向补丁”，不直接整仓合并上游，避免破坏 `scaletail`/`scaletaild`、Windows 服务、LocalAPI 桥接和安装包结构。
- 已核对并纳入 v1.98.x 关键客户端修复，包括 controlclient map session 队列关闭防死锁、注册 429 退避、disco key 防旧值覆盖、macOS resolver 路径防护、Windows NRPT 多 DNS 分隔符、Linux nftables/iptables 空指针保护、homeDERP 缓存与 ReSTUN 行为。

## 功能

- Electron 38 + Vue 3 + TypeScript 图形客户端。
- 服务端连接页支持 IP/域名、端口、HTTP/HTTPS、设备名、预认证密钥、接受路由。
- 连接页展示等价命令，例如 `scaletail up --login-server=http://host:port --accept-routes=true --auth-key=...`。
- 仪表盘展示连接状态、本机名称、本机 IP、节点数量、节点列表和节点流量。
- 节点工具支持出口节点选择、`netcheck` 网络检测、宣告本地子网路由。
- `netcheck` 已切换为 LocalAPI 能力，不再依赖隐藏命令行窗口。
- 与 Headscale/ScaleForge 预认证密钥流程对齐，支持退出当前网络后重新输入新 key 发起注册。
- 托盘左键直接唤起已有窗口，不重复打开多个仪表盘。
- Windows 服务名、安装目录、快捷方式、主程序名统一为 ScaleTail。
- 核心命令统一为 `scaletail`，核心服务进程统一为 `scaletaild`。
- LocalAPI helper 统一为 `scaletail-localapi`。
- Windows 安装包包含 `ScaleTailUI.exe`、`scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe`、`wintun.dll`。

## 架构

```text
ScaleTailUI.exe
  Electron GUI
  |
  | spawn
  v
scaletail-localapi.exe
  Go LocalAPI helper
  |
  | client/local
  v
scaletaild.exe
  Windows service / core networking
  |
  v
Headscale or ScaleTail control server
```

Electron 主进程直接调用 `scaletaild` LocalAPI。`netcheck` 已经提供 `/localapi/v0/netcheck`，不再依赖隐藏命令行执行。

## 模块化

项目已开始按 `common`、`win`、`linux`、`mac` 划分模块边界，清单位于 `modules/`。

当前阶段采用清单式模块化，不直接搬动 Go 包目录，避免破坏 `scaletail.com/...` 包路径和平台 build tag。后续拆成多个 Git 仓库时，可以用模块脚本导出目标平台需要的文件集合：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\module.ps1 list
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\module.ps1 export -Module win -OutDir D:\exports\ScaleTail-Win -Clean
```

`win`、`linux`、`mac` 会自动带上 `common`。详细说明见 `modules/README.md`。

## Windows 构建

推荐在 Windows 上构建：

- Go 1.26+
- Node.js + npm
- Inno Setup 6
- PowerShell

默认依赖缓存目录：

```text
D:\workspace-qoder\deps
```

如果 Inno Setup 不在 `D:\Inno Setup 6\ISCC.exe`，可以设置 `ISCC` 环境变量指向 `ISCC.exe`。

在项目根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-windows-installer.ps1
```

脚本会完成：

- 编译 `scaletail.exe`
- 编译 `scaletaild.exe`
- 编译 `scaletail-localapi.exe`
- 准备 `wintun.dll`
- 安装并构建 Electron GUI
- 生成 Inno Setup 安装包

当前输出文件：

```text
dist\installer\ScaleTail-0.0.1-windows-amd64-setup-custom.exe
```

当前 SHA256：

```text
569C08FD5F6B8F64A6BB06F7DD7A36C491356E64D8F937C39703C59135A0B610
```

## Electron 开发

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

完整联调建议直接运行 Windows 安装包，或者使用构建脚本生成 `dist\electron\win-unpacked\ScaleTailUI.exe`。

## 安装与卸载

安装包会：

- 安装 `ScaleTailUI.exe`
- 安装 `scaletail.exe`
- 安装并启动 `ScaleTail` Windows 服务，服务进程为 `scaletaild.exe`
- 打包 `scaletail-localapi.exe`
- 打包 `wintun.dll`
- 可选创建桌面快捷方式
- 创建开始菜单和开机启动项

卸载和覆盖安装前会强制关闭：

- `ScaleTail.exe`
- `ScaleTailUI.exe`
- `scaletail.exe`
- `scaletaild.exe`
- `scaletail-localapi.exe`

卸载时会清理安装目录、`ScaleTail` 数据目录和当前用户下的 ScaleTail 配置目录。

## Headscale 使用提示

连接页等价于执行：

```powershell
scaletail up --login-server=http://YOUR_SERVER:PORT --accept-routes=true --auth-key=YOUR_KEY
```

实际操作由 GUI 通过 LocalAPI 完成，不需要用户手动打开 CMD。

如果使用宣告路由，Headscale 服务端仍需要批准该节点发布的路由，例如：

```bash
headscale nodes list-routes
headscale nodes approve-routes --identifier <node-id> --routes 192.168.10.0/24
```

## Linux 包

Linux amd64 包可以通过 WSL 构建。默认生成核心包和可选 GUI 包：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-linux-packages-wsl.ps1 -Distro Ubuntu-24.04 -Version 0.0.1
```

服务器只需要核心包时可以跳过 GUI：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-linux-packages-wsl.ps1 -Distro Ubuntu-24.04 -Version 0.0.1 -SkipGui
```

核心命令为 `/usr/bin/scaletail`，服务为 `scaletaild.service`。

## macOS 包

macOS 未签名安装包通过 GitHub Actions 的 macOS runner 构建，当前为核心 daemon/CLI 安装包，不包含 Electron GUI。

安装路径：

```text
/usr/local/bin/scaletail
/usr/local/bin/scaletaild
/Library/LaunchDaemons/com.scaletail.scaletaild.plist
/Library/ScaleTail
```

未签名 `.pkg` 适合内部测试。正式公开分发仍需要 Apple Developer ID 签名和 notarization。

## 与上游关系

ScaleTail 基于 Tailscale 源码定制，核心网络能力仍来自上游项目。当前核心代码按官方 Tailscale `v1.98.5` 的关键修复进行审计和定向回补，但产品名、命令名、服务名、Electron 桌面端、Windows 安装包和平台模块化清单属于 ScaleTail 自身维护内容。

客户端与服务端通过标准 Tailscale/headscale 控制协议对接；对自建 Headscale/ScaleForge 场景，推荐配套使用已回补 headscale `v0.29.1` 注册稳定性修复的服务端版本。

许可证保持与上游一致。

上游项目：

- https://github.com/tailscale/tailscale
