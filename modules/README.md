# ScaleTail 模块边界

这个目录定义项目的第一阶段模块化边界。当前不直接搬动 Go 包目录，因为现有代码仍是一个 `scaletail.com` Go module，很多包通过 build tag 在同一个目录内区分平台；贸然物理拆目录会导致大量 import 和构建规则同时变化。

本阶段采用“清单式模块化”：

- `common`：所有平台共用的核心协议、网络、LocalAPI、daemon/CLI 主体、共享工具和根构建文件。
- `win`：Windows 桌面客户端、Electron 仪表盘、Inno 安装包、Windows 服务/托盘/命名管道/策略等平台代码。
- `linux`：Linux daemon/CLI、deb/rpm/tgz 包、systemd/sysv 辅助脚本、Linux 路由/DNS/防火墙等平台代码。
- `mac`：macOS daemon/CLI、未签名 pkg 构建脚本、Darwin 网络/DNS/安装等平台代码。

每个模块目录下的 `module.json` 是机器可读清单，供 `scripts/module.ps1` 使用。后续拆成多个 Git 仓库时，可以先用清单导出目标模块组合，例如：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\module.ps1 export -Module win -OutDir D:\exports\ScaleTail-Win -Clean
```

上面的命令会自动带上 `common`，导出 `common + win` 的源文件集合。Linux/mac 同理。

## 拆仓库建议

建议最终拆为：

- `ScaleTail-Core`：承载 `common`。
- `ScaleTail-Win`：承载 `common + win`，或以后通过子模块/依赖引用 Core。
- `ScaleTail-Linux`：承载 `common + linux`。
- `ScaleTail-Mac`：承载 `common + mac`。

在真正拆仓库前，不建议先改 Go module path。当前清单先解决“边界”和“可导出”，等各平台构建稳定后，再决定是否把 Core 做成独立 Go module。
