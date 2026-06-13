#define AppVersion "0.0.1"

[Setup]
AppName=ScaleTail
AppVersion={#AppVersion}
DefaultDirName={autopf}\ScaleTail
DefaultGroupName=ScaleTail
PrivilegesRequired=admin
OutputDir=..\dist\installer
OutputBaseFilename=scaletail-{#AppVersion}-windows-amd64-setup-custom
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
DefaultDialogFontName=Microsoft YaHei UI
SetupIconFile=..\client\electron\resources\app.ico
UninstallDisplayIcon={app}\resources\app\resources\app.ico
CloseApplications=force
RestartApplications=no

[Languages]
Name: "chinesesimp"; MessagesFile: ".\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加图标："; Flags: unchecked

[InstallDelete]
Type: files; Name: "{app}\Tailscale.exe"
Type: files; Name: "{app}\ScaleTail.exe"
Type: files; Name: "{app}\tailscaled.exe"
Type: files; Name: "{app}\scaletaild.exe"
Type: files; Name: "{app}\tailscale.exe"
Type: files; Name: "{app}\tailscale-cli.exe"
Type: files; Name: "{app}\tailscale-systray.exe"
Type: files; Name: "{app}\tailscale-localapi.exe"
Type: files; Name: "{app}\scaletail-localapi.exe"
Type: files; Name: "{app}\wintun.dll"
Type: filesandordirs; Name: "{app}\resources"
Type: filesandordirs; Name: "{app}\locales"
Type: files; Name: "{autoprograms}\Tailscale.lnk"
Type: files; Name: "{autodesktop}\Tailscale.lnk"
Type: files; Name: "{commonstartup}\Tailscale.lnk"

[Files]
Source: "..\dist\electron\win-unpacked\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs restartreplace uninsrestartdelete

[Icons]
Name: "{autoprograms}\ScaleTail"; Filename: "{app}\ScaleTail.exe"; Parameters: "--open-dashboard"; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{autodesktop}\ScaleTail"; Filename: "{app}\ScaleTail.exe"; Parameters: "--open-dashboard"; Tasks: desktopicon; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{commonstartup}\ScaleTail"; Filename: "{app}\ScaleTail.exe"; IconFilename: "{app}\resources\app\resources\app.ico"

[Run]
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM Tailscale.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧客户端..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale-systray.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧托盘程序..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止旧服务..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscaled.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletaild.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."
Filename: "{cmd}"; Parameters: "/C call ""{app}\scaletaild.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{cmd}"; Parameters: "/C call ""{app}\tailscaled.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{app}\scaletaild.exe"; Parameters: "install-system-daemon"; Flags: runhidden waituntilterminated; StatusMsg: "正在安装 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" start ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在启动 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C timeout /T 3 /NOBREAK >NUL"; Flags: runhidden waituntilterminated; StatusMsg: "正在等待 ScaleTail 服务就绪..."
Filename: "{app}\ScaleTail.exe"; Parameters: "--open-dashboard"; Description: "启动 ScaleTail 客户端"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM Tailscale.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧客户端..."; RunOnceId: "CloseElectron"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."; RunOnceId: "CloseScaleTailElectron"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."; RunOnceId: "CloseLocalAPIHelper"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."; RunOnceId: "CloseScaleTailLocalAPIHelper"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale-systray.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧托盘程序..."; RunOnceId: "CloseOldSystray"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale-cli.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭辅助进程..."; RunOnceId: "CloseCLI"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscale.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭旧命令行进程..."; RunOnceId: "CloseOldCLI"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 ScaleTail 服务..."; RunOnceId: "StopScaleTailService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止旧服务..."; RunOnceId: "StopLegacyService"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM tailscaled.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."; RunOnceId: "CloseDaemon"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletaild.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."; RunOnceId: "CloseScaleTailDaemon"
Filename: "{cmd}"; Parameters: "/C call ""{app}\scaletaild.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在卸载 ScaleTail 服务..."; RunOnceId: "UninstallScaleTailService"
Filename: "{cmd}"; Parameters: "/C call ""{app}\tailscaled.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在卸载旧服务..."; RunOnceId: "UninstallLegacyService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 ScaleTail 服务..."; RunOnceId: "DeleteScaleTailService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Tailscale >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除旧服务..."; RunOnceId: "DeleteLegacyService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 WinTun 驱动..."; RunOnceId: "StopWinTun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 WinTun 驱动..."; RunOnceId: "DeleteWinTun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 Wintun 驱动..."; RunOnceId: "StopWintun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 Wintun 驱动..."; RunOnceId: "DeleteWintun"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
Type: filesandordirs; Name: "{commonappdata}\ScaleTail"
Type: filesandordirs; Name: "{commonappdata}\Tailscale"

[Code]
procedure RunHidden(CommandLine: String);
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{cmd}'), '/C ' + CommandLine, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure StopScaleTailBeforeFileOps();
begin
  RunHidden('taskkill /F /T /IM Tailscale.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM tailscale-localapi.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM tailscale-systray.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM tailscale-cli.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM tailscale.exe >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" stop ScaleTail >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" stop Tailscale >NUL 2>NUL');
  RunHidden('timeout /T 2 /NOBREAK >NUL');
  RunHidden('taskkill /F /T /IM tailscaled.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletaild.exe >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{app}\scaletaild.exe') + '" uninstall-system-daemon >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{app}\tailscaled.exe') + '" uninstall-system-daemon >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" delete ScaleTail >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" delete Tailscale >NUL 2>NUL');
end;

procedure CleanupCurrentUserScaleTailData();
begin
  RunHidden('if exist "%APPDATA%\ScaleTail" rmdir /S /Q "%APPDATA%\ScaleTail"');
  RunHidden('if exist "%LOCALAPPDATA%\ScaleTail" rmdir /S /Q "%LOCALAPPDATA%\ScaleTail"');
  RunHidden('if exist "%APPDATA%\Tailscale" rmdir /S /Q "%APPDATA%\Tailscale"');
  RunHidden('if exist "%LOCALAPPDATA%\Tailscale" rmdir /S /Q "%LOCALAPPDATA%\Tailscale"');
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopScaleTailBeforeFileOps();
  Result := '';
end;

function InitializeUninstall(): Boolean;
begin
  StopScaleTailBeforeFileOps();
  CleanupCurrentUserScaleTailData();
  Result := True;
end;
