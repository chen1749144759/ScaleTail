#define AppVersion "1.97.0"

[Setup]
AppName=Tailscale
AppVersion={#AppVersion}
DefaultDirName={autopf}\Tailscale
DefaultGroupName=Tailscale
PrivilegesRequired=admin
OutputDir=..\dist\installer
OutputBaseFilename=tailscale-{#AppVersion}-windows-amd64-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
DefaultDialogFontName=Microsoft YaHei UI

[Languages]
Name: "chinesesimp"; MessagesFile: ".\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加图标："; Flags: unchecked

[Files]
Source: "..\dist\windows-amd64\tailscale.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\windows-amd64\tailscaled.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\windows-amd64\tailscale-systray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\windows-amd64\wintun.dll"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Tailscale"; Filename: "{app}\tailscale-systray.exe"; Parameters: "-open-dashboard"
Name: "{autodesktop}\Tailscale"; Filename: "{app}\tailscale-systray.exe"; Parameters: "-open-dashboard"; Tasks: desktopicon
Name: "{commonstartup}\Tailscale"; Filename: "{app}\tailscale-systray.exe"

[Run]
Filename: "{cmd}"; Parameters: "/C taskkill /F /IM tailscale-systray.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧托盘程序..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{app}\tailscaled.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{app}\tailscaled.exe"; Parameters: "install-system-daemon"; Flags: runhidden waituntilterminated; StatusMsg: "正在安装 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" start Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在启动 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C timeout /T 3 /NOBREAK >NUL"; Flags: runhidden waituntilterminated; StatusMsg: "正在等待 Tailscale 服务就绪..."
Filename: "{app}\tailscale-systray.exe"; Description: "启动 Tailscale 托盘程序"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{cmd}"; Parameters: "/C taskkill /F /IM tailscale-systray.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 Tailscale 托盘程序..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /IM tailscale.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 Tailscale 命令行进程..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{app}\tailscaled.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在卸载 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 Tailscale 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 WinTun 驱动..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 WinTun 驱动..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 Wintun 驱动..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 Wintun 驱动..."

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
