# ScaleTail

[![Release](https://img.shields.io/github/v/release/chen1749144759/ScaleTail?sort=semver&label=release)](https://github.com/chen1749144759/ScaleTail/releases)
[![Windows](https://img.shields.io/badge/Windows-amd64-0078D4?logo=windows&logoColor=white)](https://github.com/chen1749144759/ScaleTail/releases)
[![Linux](https://img.shields.io/badge/Linux-amd64-FCC624?logo=linux&logoColor=black)](https://github.com/chen1749144759/ScaleTail/releases)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Electron](https://img.shields.io/badge/Electron-43.3.0-47848F?logo=electron&logoColor=white)](https://www.electronjs.org/)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](https://vuejs.org/)
[![Upstream](https://img.shields.io/badge/Tailscale-v1.98.9%20fixes-4D7CFE)](https://github.com/tailscale/tailscale/releases/tag/v1.98.9)

ScaleTail 是面向 Headscale-Admin-AE 与 ScaleForge 私有控制面的网络客户端。项目基于 `tailscale/tailscale` 源码裂变，统一使用 ScaleTail 产品名，并提供 Windows 桌面端、系统服务、Linux CLI、签名 OTA、DNS 策略、流量采样和 TUN 数据路径限速。

仓库地址：[chen1749144759/ScaleTail](https://github.com/chen1749144759/ScaleTail)

## 版本定位

| 项目 | 当前说明 |
|---|---|
| 裂变来源 | `tailscale/tailscale` 主线源码，Go module 已改为 `scaletail.com` |
| 当前产品版本 | `v0.0.9`，支持纯 HTTP 控制地址下的 Noise 加密与 TOFU 公钥锁定，继续沿用签名 OTA v3 |
| 对标版本 | 已定向审计并回补 Tailscale `v1.98.9` 的关键安全和稳定性修复 |
| Windows 桌面端 | Electron 43.3.0 + Vue 3 + TypeScript |
| 配套服务端 | Headscale-Admin-AE + ScaleForge |

ScaleTail 不是上游的无损镜像。上游修复会先按当前控制协议、服务命名、LocalAPI、桌面端和安装包结构评估，再定向实现。

## 核心架构

```text
ScaleTail UI / scaletail CLI
          |
          | LocalAPI / Windows named pipe / Unix socket
          v
      scaletaild
          |
          | Noise control connection + account proof
          v
 Headscale-Admin-AE
          |
          | private Unix socket gateway
          v
      ScaleForge
```

- `scaletaild` 是网络核心和长期运行的系统服务。
- Windows UI 和 Linux CLI 都通过 LocalAPI 调用 `scaletaild`，不会要求用户打开 CMD 完成连接。
- 账号密码通过本机 LocalAPI 进入现有 Noise 控制连接，不由浏览器跳转，也不使用预认证 Key。
- 机器密钥、节点密钥仍用于 WireGuard/Noise 加密身份，但不是另一套用户登录凭证。
- 遥测、策略和更新请求由 Headscale 验证节点身份后通过私有 UDS 转发，不再维护额外的客户端上报令牌。

## 账户认证

新设备和重新认证统一使用 ScaleForge 账号密码。服务端要求每个新的控制会话完成账户证明，并将节点有效期限制在密码有效期内。

- 密码最长 72 字节，由服务端使用强密码哈希保存。
- 密码每 90 天必须由用户更新；过期后不能建立新的认证会话。
- 控制地址支持严格的 origin-only `http://` 和 `https://`；两者都拒绝 URL 凭据、业务路径、查询参数和片段。
- 账号密码始终只在现有 Noise 控制通道中传输。HTTP 不会把密码作为明文 HTTP 请求体或请求头发送。
- HTTP 首次连接采用 TOFU 固定该控制 origin 返回的 Noise 公钥；同 origin 后续公钥变化会立即阻断连接。确认服务端确实重建或轮换密钥后，需要先退出当前网络，再重新连接以建立新信任。
- HTTPS 使用正常的证书信任建立初始服务端身份，不依赖 HTTP TOFU pin。
- CLI 不提供明文 `--password` 参数，避免密码进入 shell 历史和进程列表。
- 旧预认证 Key、浏览器认证、OIDC 登录和手工注册码不是当前产品登录路径。

Linux 安装包提供账户配置工具。它会隐藏密码输入，验证登录成功后才以 `0600` 权限保存凭据，并启用守护进程重启后的自动重认证：

```bash
sudo scaletail-configure-account \
  --server http://control.example.com:60090 \
  --username alice \
  --accept-routes true \
  --accept-dns true
```

无人值守或 ISO 预制场景应由安全的配置系统生成 `/etc/scaletail/account.conf` 和仅 root 可读的 `/etc/scaletail/account-password`，再启用自动登录单元：

```bash
sudo install -o root -g root -m 0600 /secure/provision/account-password \
  /etc/scaletail/account-password

sudo install -o root -g root -m 0600 /secure/provision/account.conf \
  /etc/scaletail/account.conf

sudo systemctl enable --now scaletaild.service
sudo systemctl enable --now scaletail-account-login.service
```

配置模板位于 `/usr/share/doc/scaletail/account.conf.example`。不要把账号密码写入镜像公共层、内核命令行或 shell 历史。

登录后再单独配置路由，避免身份参数和网络参数混在同一条命令中：

```bash
sudo scaletail set --accept-routes=true
sudo scaletail set --advertise-routes=192.168.10.0/24
```

## 自实现能力

- Windows 原生产品入口：Electron 仪表盘、托盘单实例、连接/恢复/断开/退出网络、节点、出口节点、路由、DNS 和 netcheck。
- Windows 服务、二进制、安装目录、快捷方式和卸载逻辑统一为 `ScaleTail`、`scaletail.exe`、`scaletaild.exe`。
- Windows 安装包包含 `ScaleTailUI.exe`、`scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe`、更新助手和 `wintun.dll`。
- 安装包支持直接覆盖升级；卸载会停止服务并清理相关进程、服务和历史残留。
- Windows 签名 OTA：下载、SHA-256 校验、Ed25519 发布签名校验、daemon 二次验签和静默覆盖安装。
- ScaleForge DNS 策略下发；客户端可显式选择是否采用服务端 DNS。
- 流量、连接摘要、在线状态、策略应用结果和安全数据定时上报。
- 上传/下载限速位于 `scaletaild` TUN 数据路径，按整台机器控制经过 ScaleTail 覆盖网络的总带宽。
- 限速策略可经 LocalAPI 热更新；断开、退出、关闭和配额阻断会清理运行态。
- 物理局域网直连和未经过 ScaleTail 虚拟网卡的普通公网流量不统计、不限速。

## Windows 构建

前置依赖：

- Go 1.26+
- Node.js + npm
- Inno Setup 6
- PowerShell

默认依赖缓存位于 `D:\workspace-qoder\deps`。Inno Setup 不在 `D:\Inno Setup 6\ISCC.exe` 时，可设置 `ISCC` 环境变量。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\build-windows-installer.ps1 `
  -GoBuildParallelism 4
```

主要输出：

```text
dist\installer\ScaleTail-0.0.9-windows-amd64-setup-custom.exe
dist\installer\ScaleTail-0.0.9-windows-amd64.ota.json
```

OTA 私钥默认从 `D:\workspace-qoder\deps\scaletail-ota\ed25519-private.key` 读取，也可通过 `SCALETAIL_UPDATE_SIGNING_KEY` 指定。私钥只能保存在构建机并单独备份，禁止提交到 Git。

`GoBuildParallelism` 默认是 `4`，用于限制大型 Windows 构建的并发编译器数量；稳定的高性能构建机可显式调高。

## OTA v3 发布协议

v3 将策略版本、建议/强制/撤销动作和安装包元数据放在同一个签名中。每次发布或撤销必须使用严格递增的 `revision`：

```powershell
go run ./cmd/scaletail-update-sign `
  -private-key D:\secure\ed25519-private.key `
  -file .\dist\installer\ScaleTail-0.0.9-windows-amd64-setup-custom.exe `
  -version 0.0.9 `
  -platform windows-amd64 `
  -action suggested `
  -revision 202608090001 `
  -download-url https://downloads.example.com/releases/ScaleTail-0.0.9-windows-amd64-setup.exe `
  -json-out .\dist\installer\ScaleTail-0.0.9-windows-amd64.ota.json
```

签名消息按顺序包含 `scaletail-update-v3`、`revision`、动作、版本、平台、小写 SHA-256、文件大小和规范化 `download_url`。`signature` 必须使用 `v3.<Ed25519 Base64>`；v1/v2 元数据会被拒绝。

- `suggested` 允许用户延后安装，`forced` 会由 daemon 持久化强制策略、暂停网络并要求升级。
- `clear` 是带签名的撤销策略，不含安装包摘要、大小和 URL；撤销后按升级前状态恢复网络。
- `scaletaild` 持久化已接受的最高 revision，拒绝回滚和同 revision 内容替换；Electron 只是展示和触发入口。
- 安装包在 Electron 下载前和 daemon 执行前分别验签、核对大小及 SHA-256，静默覆盖升级不要求先卸载或手工断开。

客户端会在下载安装包前完成签名和元数据校验，并且每个重定向都只能是无凭据、无片段的 HTTPS DNS 主机名。它拒绝 `localhost`、回环/私网/链路本地 IP literal 及全部 IP literal。当前没有预解析域名并拒绝私网 DNS 结果，以避免把一次 DNS 查询误当成连接时保证；受信任下载域必须在发布运维层控制，DNS 重绑定或恶意 DNS 解析到私网仍是该层的残余风险。

Electron 单独验证：

```powershell
cd client\electron
npm ci
npm run typecheck
npm run build
```

## Linux 构建

在安装了 Rocky Linux 9 WSL 的 Windows 构建机上执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\build-linux-packages-wsl.ps1 `
  -Distro Rocky-9.4 `
  -Version 0.0.9 `
  -DependencyRoot D:\DevDeps `
  -GoProxy https://goproxy.cn,direct
```

输出目录 `dist\linux-v0.0.9` 包含 amd64 的 `.tgz`、`.deb`、`.rpm`、校验文件以及可选 GUI 包。无桌面的服务器只安装核心包：

```bash
sudo dpkg -i scaletail_0.0.9_amd64.deb
sudo scaletail-configure-account --server http://control.example.com:60090 --username alice
```

## 部署难度

| 场景 | 难度 | 说明 |
|---|---:|---|
| Windows 安装包 | 低 | 管理员运行安装包，自动安装服务和 Wintun |
| Windows 源码构建 | 中 | 需要 Go、Node.js、npm 和 Inno Setup 6 |
| Linux CLI | 中 | 安装核心包、启用服务并完成账户登录 |
| Linux 无人值守 | 中 | 需要安全注入 0600 密码文件并规划 90 天轮换 |
| macOS 未签名包 | 中高 | 通过 GitHub Actions macOS runner 构建，用户需手动信任 |
| 核心网络开发 | 高 | 涉及 LocalAPI、TUN、路由、DNS、Noise 和控制协议 |

## 安全边界

- 不要把账号密码、OTA 私钥或生产配置提交到仓库、镜像、ISO 公共层或安装日志。
- 不要把密码直接写在命令行参数中；无人值守只使用权限为 `0600` 的文件或标准输入。
- HTTP 控制面的安全边界是 Noise 加密与按 origin 持久化的 TOFU 公钥；公钥异常变化不得自动接受。
- 强制 OTA 只接受内置公钥验证通过且摘要匹配的安装包。
- 自定义 DERP 不是必需组件；当前优先使用服务端嵌入 DERP，并启用私有节点验证。
- 客户端升级不会部署或迁移 ScaleForge/Headscale 数据库，服务端升级需按各自仓库文档执行。

## 验证

开发改动至少运行：

```powershell
go test ./cmd/scaletail/cli ./client/local ./ipn/localapi ./control/controlclient
cd client\electron
npm run typecheck
npm run build
```

安装包发布前还需确认 Inno Setup 编译成功，并检查安装包包含全部 ScaleTail 二进制、更新助手和 `wintun.dll`。

## 交流学习

欢迎加入 ScaleForge 交流学习群，讨论 Headscale、ScaleTail、ScaleForge 的部署、使用和二次开发。

群号：`1041671099`

<img src="docs/images/scaleforge-qq-group.jpg" alt="ScaleForge 交流学习群" width="360">

## 打赏

如果项目帮助你节省了部署和维护时间，可以请作者喝杯咖啡：

![打赏](https://raw.githubusercontent.com/chen1749144759/ScaleForge/main/docs/screenshots/donate.jpg)

## 致谢

- [tailscale/tailscale](https://github.com/tailscale/tailscale)
- [juanfont/headscale](https://github.com/juanfont/headscale)
- [Electron](https://www.electronjs.org/)
- [Vue](https://vuejs.org/)
