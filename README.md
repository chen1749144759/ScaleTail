# ScaleTail

[![Release](https://img.shields.io/github/v/release/chen1749144759/ScaleTail?sort=semver&label=release)](https://github.com/chen1749144759/ScaleTail/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20amd64-0078D4?logo=windows&logoColor=white)](https://github.com/chen1749144759/ScaleTail/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-38-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![Upstream](https://img.shields.io/badge/Tailscale-v1.98.5%20fixes-4D7CFE)](https://github.com/tailscale/tailscale)

ScaleTail 是基于 Tailscale 源码裂变的自建网络客户端，主要面向 Headscale/ScaleForge 私有控制服务器。项目目标是把新手原本需要在 CMD 里执行的连接、状态、netcheck、出口节点、宣告路由等操作移动到桌面可视化窗口中。

仓库地址：[chen1749144759/ScaleTail](https://github.com/chen1749144759/ScaleTail)

## 版本定位

| 项目项 | 当前说明 |
|---|---|
| 裂变来源 | 基于 `tailscale/tailscale` 主线源码改造，项目 module 已改为 `scaletail.com` |
| 当前产品版本 | `v0.0.1` |
| 对标官方版本 | 已按官方 Tailscale `v1.98.5` 的关键客户端修复做定向审计和回补 |
| Go 版本 | `go.mod` 使用 Go `1.26.2` |
| 桌面端 | Electron 38 + Vue 3 + TypeScript |
| 配套服务端 | 推荐配套 `ScaleForge` + `Headscale-Admin-AE` |

注意：ScaleTail 不是对官方 Tailscale 的整仓无损同步版本，而是保留自定义命名、Windows 服务、LocalAPI helper、Electron 桌面端和安装包逻辑后，对上游关键稳定性修复进行选择性回补。

## 自实现功能

- 产品命名统一为 ScaleTail，核心命令为 `scaletail`，核心服务为 `scaletaild`，LocalAPI helper 为 `scaletail-localapi`。
- Windows 桌面端使用 Electron 38 + Vue 3 + TypeScript 实现，安装后由托盘常驻。
- 服务端连接页支持控制服务器 IP/域名、端口、HTTP/HTTPS、设备名、预认证密钥、接受路由。
- 连接页展示等价命令，便于核对实际执行逻辑，例如 `scaletail up --login-server=http://host:port --auth-key=...`。
- 已修正预认证密钥连接逻辑：填写 key 时不再触发浏览器交互式认证。
- 仪表盘展示连接状态、本机名称、本机 ScaleTail IP、节点数量、节点列表和节点流量。
- 节点页支持 `netcheck`、出口节点选择、宣告子网路由。
- `netcheck` 通过 LocalAPI 调用，不再依赖外露 CMD 窗口。
- 托盘左键直接唤起已有窗口，不重复打开多个仪表盘。
- Windows 安装包包含 `ScaleTailUI.exe`、`scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe`、`wintun.dll`。
- 安装、覆盖安装和卸载会尝试关闭相关进程、停止服务、清理旧服务和残留文件。
- 新增平台上报能力：客户端可定时向 ScaleForge 上报流量、请求连接摘要、策略应用状态。
- 新增策略领取能力：客户端可领取 ScaleForge 下发的限速/配额策略。
- Windows 上传限速通过系统 QoS 尝试应用；下载限速字段已预留，但暂未做 TUN/内核级强制，后续进入 `scaletaild` 核心流量路径实现。

## 部署难度

| 场景 | 难度 | 说明 |
|---|---:|---|
| 直接安装 Windows exe 安装包 | 低 | 推荐方式。需要管理员权限安装 Windows 服务和 Wintun。 |
| 从源码构建 Windows 安装包 | 中 | 需要 Go、Node.js、npm、Inno Setup 6。构建脚本已自动处理 Electron 和 Wintun。 |
| Linux 客户端命令行包 | 中 | 可按原项目 Linux 编译链路打包，图形窗口可选。 |
| macOS 未签名包 | 中高 | 可以通过 GitHub Actions macOS runner 构建，未签名包需要用户手动信任。 |
| 自行深度改核心网络 | 高 | 涉及 `scaletaild`、LocalAPI、Wintun/TUN、路由、DNS 和控制面协议。 |

## Windows 构建

前置要求：

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

输出文件：

```text
dist\installer\ScaleTail-0.0.1-windows-amd64-setup-custom.exe
```

## Electron 开发

```powershell
cd client\electron
npm ci
npm run typecheck
npm run build
```

完整联调建议使用构建脚本生成 `dist\electron\win-unpacked\ScaleTailUI.exe`，或直接安装生成的 Windows 安装包。

## 与服务端关系

```text
ScaleTail 客户端
  |
  | Tailscale/headscale 控制协议 + LocalAPI
  v
Headscale-Admin-AE 控制服务
  |
  | 共享数据库/API
  v
ScaleForge 管理平台
```

ScaleTail 负责客户端连接和桌面体验；Headscale-Admin-AE 负责控制面和节点注册；ScaleForge 负责可视化管理、策略、流量统计、安全审计。

## 当前已验证

- Electron `npm run typecheck` 通过。
- Electron `npm run build` 通过。
- Windows 安装包脚本完整通过。
- Inno Setup 6 编译通过，安装包内确认包含 `scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe` 和 `wintun.dll`。

## TODO

- 下载限速进入 `scaletaild` 核心流量路径实现，避免只做页面字段但无强制效果。
- 继续减少上游残留命名和注释中的 Tailscale 字样。
- macOS 未签名安装包通过 GitHub Actions macOS runner 自动构建。
- Linux 图形端作为可选包输出，服务器场景默认只安装命令行和服务。

## 打赏

如果这个项目帮你节省了部署和维护时间，可以请作者喝杯咖啡。打赏二维码维护在 ScaleForge 仓库中：

![打赏](https://raw.githubusercontent.com/chen1749144759/ScaleForge/main/docs/screenshots/donate.jpg)

感谢支持，项目会继续围绕自建 Headscale/ScaleTail 网络的易用性、稳定性和安全可视化迭代。

## 致谢

- [tailscale/tailscale](https://github.com/tailscale/tailscale)
- [juanfont/headscale](https://github.com/juanfont/headscale)
- [Electron](https://www.electronjs.org/)
- [Vue](https://vuejs.org/)
