# ScaleTail

[![Release](https://img.shields.io/github/v/release/chen1749144759/ScaleTail?sort=semver&label=release)](https://github.com/chen1749144759/ScaleTail/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20amd64-0078D4?logo=windows&logoColor=white)](https://github.com/chen1749144759/ScaleTail/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-38-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![Upstream](https://img.shields.io/badge/Tailscale-v1.98.9%20fixes-4D7CFE)](https://github.com/tailscale/tailscale/releases/tag/v1.98.9)

ScaleTail 是基于 Tailscale 源码裂变的自建网络客户端，主要面向 Headscale/ScaleForge 私有控制服务器。项目目标是把新手原本需要在 CMD 里执行的连接、状态、netcheck、出口节点、宣告路由等操作移动到桌面可视化窗口中。

仓库地址：[chen1749144759/ScaleTail](https://github.com/chen1749144759/ScaleTail)

## 版本定位

| 项目项 | 当前说明 |
|---|---|
| 裂变来源 | 基于 `tailscale/tailscale` 主线源码改造，项目 module 已改为 `scaletail.com` |
| 当前产品版本 | `v0.0.7` |
| 对标官方版本 | 已按官方 Tailscale `v1.98.9` 的关键客户端、安全和稳定性修复做定向审计和回补 |
| Go 版本 | `go.mod` 使用 Go `1.26.5` |
| 桌面端 | Electron 38 + Vue 3 + TypeScript |
| 配套服务端 | 推荐配套 `ScaleForge` + `Headscale-Admin-AE` |

注意：ScaleTail 不是对官方 Tailscale 的整仓无损同步版本，而是保留自定义命名、Windows 服务、LocalAPI helper、Electron 桌面端和安装包逻辑后，对上游关键稳定性修复进行选择性回补。

## 最近更新

### v0.0.7 官方修复、签名 OTA 与 TUN 双向限速

- 定向回补 Tailscale `v1.98.8-v1.98.9` 的 SSH 用户名/环境变量安全校验、Serve/Funnel 路径防护、服务 VIP 端口校验、休眠恢复和握手抑制等关键修复。
- Windows 客户端支持完整 OTA：后台下载、SHA-256 校验、Ed25519 发布签名校验、`scaletaild` 二次验签、系统权限静默覆盖安装和安装后自动拉起桌面端。
- OTA 不信任平台返回的下载地址本身；只有内置公钥认可的安装包才能进入静默安装流程。
- 构建脚本会生成与安装包配套的 `.ota.json`，可直接导入 ScaleForge 客户端版本发布页。
- `v0.0.7` 是 OTA 引导版本：更早版本需要手动覆盖安装一次；安装 `v0.0.7` 后，后续签名版本可由客户端静默覆盖升级。
- 上传和下载限速进入 `scaletaild` TUN 数据路径，按整台机器分别控制双向总带宽，策略支持 LocalAPI 热更新。
- 限速只处理经过 ScaleTail 虚拟网卡的覆盖网络流量，不影响物理局域网直连、普通公网流量和控制连接。
- 修正客户端采样语义：未连接网络时不把缺少流量采样误判为故障，平台依据心跳和连接状态区分离线、空闲与异常缺失。

### v0.0.6

- Electron 连接流程改为直接调用 `scaletaild` LocalAPI 新增的 `/localapi/v0/scaletail-up`，执行等价 `up` 的连接逻辑，不再走隐藏 CLI 作为连接主路径。
- `scaletail-up` 支持控制服务器地址、设备名、预认证密钥、接受路由和 DNS 偏好，并在 daemon 内完成参数校验、Prefs 更新、启动和必要的交互式登录触发。
- 修复平台上报配置保存时 Electron IPC 出现 `An object could not be cloned` 的问题，保存前会转为可克隆的纯对象。
- 支持读取打包内置的 `resources/report-config.json`，便于安装包携带 ScaleForge 上报地址和客户端 token；用户配置仍优先覆盖内置配置。
- 安装包版本升级到 `0.0.6`，覆盖安装和卸载时会清理旧版 `tailscale.exe`、`tailscaled.exe`、`tailscale-localapi.exe` 进程和旧服务残留。

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
- Windows 安装包包含 `ScaleTailUI.exe`、`scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe`、`ScaleTailUpdateHelper.exe`、`wintun.dll`。
- 安装、覆盖安装和卸载会尝试关闭相关进程、停止服务、清理旧服务和残留文件。
- 新增平台上报能力：客户端可定时向 ScaleForge 上报流量、请求连接摘要、策略应用状态。
- 新增策略领取能力：客户端可领取 ScaleForge 下发的限速/配额策略。
- 客户端版本发布支持建议更新/强制更新；Windows 桌面端可在不手动卸载、不重复操作安装向导的情况下完成签名 OTA 覆盖升级。
- Windows 双向限速已进入 `scaletaild` TUN 核心流量路径：上传、下载分别按整台机器的总带宽整形，所有进程和连接共享总额度，不按进程或单连接分配。
- 数据面继续使用 Wintun，不引入 WinDivert 或额外进程级拦截驱动。
- 限速策略支持 LocalAPI 热更新；相同策略重复轮询不会重置令牌桶，断开连接、退出网络、服务关闭和配额阻断都会清除运行态，不在系统中遗留限速状态。
- 限速只作用于穿过 ScaleTail 虚拟网卡的覆盖网络流量，不影响物理局域网直连、普通公网访问和控制连接。
- Windows 连接明细按实际路由出口过滤，只有确认经当前 ScaleTail 虚拟网卡转发的目标才会上报；物理网直连和未走 ScaleTail 的公网连接不会进入平台排行。
- 月度配额的提醒、降速和阻断由 `scaletaild` 执行；降速同时约束双向总带宽，阻断会停止当前网络连接。

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
dist\installer\ScaleTail-0.0.7-windows-amd64-setup-custom.exe
dist\installer\ScaleTail-0.0.7-windows-amd64.ota.json
```

构建脚本默认从 `D:\workspace-qoder\deps\scaletail-ota\ed25519-private.key` 读取 OTA 私钥，也可通过 `SCALETAIL_UPDATE_SIGNING_KEY` 指定。私钥只能保存在构建机并单独备份，禁止提交到 Git。将安装包上传到下载地址后，在 ScaleForge 的“客户端版本”页面导入 `.ota.json`，再填写下载地址即可发布。

## Windows 安装与升级

1. 从 [GitHub Releases](https://github.com/chen1749144759/ScaleTail/releases) 下载 `ScaleTail-0.0.7-windows-amd64-setup-custom.exe` 和 `SHA256SUMS.txt`。
2. 使用 PowerShell 执行 `Get-FileHash .\ScaleTail-0.0.7-windows-amd64-setup-custom.exe -Algorithm SHA256`，确认结果与校验文件一致。
3. 右键安装包并选择“以管理员身份运行”。首次安装会安装 `ScaleTail` Windows 服务和 Wintun 驱动。
4. 已安装旧版本时直接运行新安装包覆盖升级，不需要先断开网络或手动卸载；节点身份、服务端配置和上报配置会继续保留。
5. `v0.0.6` 及更早版本需要手动覆盖安装一次 `v0.0.7`，之后才能使用签名 OTA 静默升级。

公开 Release 安装包不内置任何生产环境的 `SCALETAIL_CLIENT_TOKEN`。新安装后请在客户端页面配置 ScaleForge 地址和上报密钥；覆盖升级不会清除已经保存的本地配置。

## Linux 构建与安装

在已安装 Ubuntu 24.04 WSL 的 Windows 构建机上执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build-linux-packages-wsl.ps1 `
  -Distro Ubuntu-24.04 `
  -Version 0.0.7 `
  -DependencyRoot D:\workspace-qoder\deps
```

默认输出到 `dist\linux-v0.0.7`，包含 amd64 的 `.tgz`、`.deb`、`.rpm`、可选 GUI 包和 `SHA256SUMS-linux-amd64.txt`。服务器建议只安装核心包；桌面 Linux 再选择 `scaletail-gui` 包。

Debian/Ubuntu 示例：

```bash
sudo dpkg -i scaletail_0.0.7_amd64.deb
sudo systemctl enable --now scaletaild
sudo scaletail up --login-server=http://HEADSCALE_IP:PORT --auth-key=hskey-auth-REPLACE_ME --accept-routes
```

安装前请按实际协议替换控制服务器地址和预认证密钥；生产环境应优先使用 HTTPS，禁止把真实密钥写入镜像、ISO 公共仓库或安装日志。

## v0.0.7 发布产物

- `ScaleTail-0.0.7-windows-amd64-setup-custom.exe`：Windows amd64 图形客户端、服务、更新助手和 Wintun 的完整安装包。
- `ScaleTail-0.0.7-windows-amd64.ota.json`：供 ScaleForge 客户端版本页导入的签名 OTA 元数据。
- `scaletail_0.0.7_amd64.deb`、`.rpm`、`.tgz`：Linux amd64 核心客户端包。
- `scaletail-gui_0.0.7_all.deb`、`scaletail-gui_0.0.7_noarch.rpm`：Linux 架构无关的可选图形集成包。
- `SHA256SUMS.txt`、`SHA256SUMS-linux-amd64.txt`：下载完整性校验文件。

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
- Linux amd64 的 tgz、deb、rpm 和可选 GUI 包使用锁定 revision 的 Tailscale Go 工具链在 Linux 容器中构建并完成包内容核验。

## TODO

- 继续减少上游残留命名和注释中的 Tailscale 字样。
- macOS 未签名安装包通过 GitHub Actions macOS runner 自动构建。
- Linux 图形端作为可选包输出，服务器场景默认只安装命令行和服务。

## 交流学习

欢迎加入 ScaleForge 交流群，一起交流自建 Headscale、ScaleTail、ScaleForge 的部署、使用和二次开发经验。

群号：`1041671099`

<img src="docs/images/scaleforge-qq-group.jpg" alt="ScaleForge 交流群" width="360">

## 打赏

如果这个项目帮你节省了部署和维护时间，可以请作者喝杯咖啡。打赏二维码维护在 ScaleForge 仓库中：

![打赏](https://raw.githubusercontent.com/chen1749144759/ScaleForge/main/docs/screenshots/donate.jpg)

感谢支持，项目会继续围绕自建 Headscale/ScaleTail 网络的易用性、稳定性和安全可视化迭代。

## 致谢

- [tailscale/tailscale](https://github.com/tailscale/tailscale)
- [juanfont/headscale](https://github.com/juanfont/headscale)
- [Electron](https://www.electronjs.org/)
- [Vue](https://vuejs.org/)
