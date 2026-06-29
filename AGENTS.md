# ScaleTail 项目协作规则

本文件是 ScaleTail 客户端仓库的项目级约束。处理本仓库时，优先级高于通用偏好；如果与系统安全规则冲突，以更高层规则为准。

## 项目识别

- 项目名称：ScaleTail。
- 本地路径：`D:\workspace-qoder\tailscale-main`。
- 项目类型：Windows 客户端、系统服务、Electron 桌面 UI、安装包工程。
- 核心产物：`scaletail.exe`、`scaletaild.exe`、`scaletail-localapi.exe`、`ScaleTailUI.exe`、Inno Setup 安装包。
- 前端：Electron + Vue 3 + TypeScript。
- 后端/核心：Go，基于 Tailscale 上游裂变并改名为 ScaleTail。
- 安装包：Inno Setup 6，脚本位于 `installer\scaletail.iss`。

## 协作边界

- 用户目标是减少 CMD 依赖：连接、状态、断开、退出网络、出口节点、netcheck、DNS 采用等能力应优先通过窗口完成。
- Electron 是 Windows 用户主入口；托盘左键应唤起已有窗口到最前，不要重复创建闪现窗口。
- `scaletaild` 是后台服务和核心网络进程；Electron 主进程优先通过 LocalAPI/命名管道与 `scaletaild` 通信。
- `scaletail.exe` 是 CLI 辅助程序，不是产品交互入口。若短期内部调用 CLI，必须隐藏窗口，并在后续优先替换为 LocalAPI/daemon API。
- Windows 服务名、二进制名、安装目录、快捷方式、托盘、窗口、卸载清理应统一使用 ScaleTail 命名。

## 修改安全规则

- 修改已有文件前，先备份到 `D:\codex_backups\tailscale-main`，允许覆盖上一次备份。
- 不要回滚用户未明确要求回滚的改动。
- 不要把生产 token、验证码 secret、数据库密码、GitHub token 写入仓库或最终回复。
- 构建临时生成的上报配置只允许作为打包临时文件存在，构建后必须清理。

## 验证规则

- Go 代码改动优先运行相关包测试或最小可行 `go test`。
- Electron UI 改动优先运行 `npm run typecheck`，必要时运行构建。
- 安装包相关改动至少检查 Inno Setup 脚本语法、安装产物包含 `wintun.dll` 和所有 ScaleTail 二进制。
- 连接流程改动需要核对服务启动、LocalAPI、预认证密钥、断开连接和退出网络两条路径。

## 提交和推送规则

- 每次改动完成后，默认必须执行：最小验证 -> `git add` -> `git commit` -> `git push`。
- 除非用户明确说“先别提交”“先别 push”“只本地改”，否则不要停在未提交或只本地提交状态。
- 如果同一轮涉及 `ScaleTail`、`ScaleForge`、`Headscale-Admin-AE` 多仓库，必须分别提交并分别 push。
- 如果发现已有未提交改动，先区分本次改动和用户已有改动，向用户说明后再决定是否纳入提交；不要混入未确认的无关改动。
- 提交后确认 `git status --short --branch` 干净，或只剩用户明确要求保留的改动。
