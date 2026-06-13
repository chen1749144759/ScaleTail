#define AppVersion "0.0.1"

[Setup]
AppName=ScaleTail
AppVersion={#AppVersion}
DefaultDirName={autopf}\ScaleTail
DefaultGroupName=ScaleTail
PrivilegesRequired=admin
OutputDir=..\dist\installer
OutputBaseFilename=ScaleTail-{#AppVersion}-windows-amd64-setup-custom
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
Type: files; Name: "{app}\ScaleTail.exe"
Type: files; Name: "{app}\ScaleTailUI.exe"
Type: files; Name: "{app}\scaletail.exe"
Type: files; Name: "{app}\scaletaild.exe"
Type: files; Name: "{app}\scaletail-localapi.exe"
Type: files; Name: "{app}\wintun.dll"
Type: filesandordirs; Name: "{app}\resources"
Type: filesandordirs; Name: "{app}\locales"

[Files]
Source: "..\dist\electron\win-unpacked\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs restartreplace uninsrestartdelete

[Icons]
Name: "{autoprograms}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{autodesktop}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; Tasks: desktopicon; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{commonstartup}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; IconFilename: "{app}\resources\app\resources\app.ico"

[Run]
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTailUI.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 命令行..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletaild.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."
Filename: "{cmd}"; Parameters: "/C call ""{app}\scaletaild.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在清理旧服务..."
Filename: "{app}\scaletaild.exe"; Parameters: "install-system-daemon"; Flags: runhidden waituntilterminated; StatusMsg: "正在安装 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" start ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在启动 ScaleTail 服务..."
Filename: "{cmd}"; Parameters: "/C timeout /T 3 /NOBREAK >NUL"; Flags: runhidden waituntilterminated; StatusMsg: "正在等待 ScaleTail 服务就绪..."
Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; Description: "启动 ScaleTail 客户端"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."; RunOnceId: "CloseScaleTailElectron"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM ScaleTailUI.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 客户端..."; RunOnceId: "CloseScaleTailUI"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 命令行..."; RunOnceId: "CloseScaleTailCLI"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭本地接口辅助进程..."; RunOnceId: "CloseScaleTailLocalAPIHelper"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 ScaleTail 服务..."; RunOnceId: "StopScaleTailService"
Filename: "{cmd}"; Parameters: "/C taskkill /F /T /IM scaletaild.exe >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在关闭 ScaleTail 服务进程..."; RunOnceId: "CloseScaleTailDaemon"
Filename: "{cmd}"; Parameters: "/C call ""{app}\scaletaild.exe"" uninstall-system-daemon >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在卸载 ScaleTail 服务..."; RunOnceId: "UninstallScaleTailService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete ScaleTail >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 ScaleTail 服务..."; RunOnceId: "DeleteScaleTailService"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 WinTun 驱动..."; RunOnceId: "StopWinTun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete WinTun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 WinTun 驱动..."; RunOnceId: "DeleteWinTun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" stop Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在停止 Wintun 驱动..."; RunOnceId: "StopWintun"
Filename: "{cmd}"; Parameters: "/C call ""{sys}\sc.exe"" delete Wintun >NUL 2>NUL || exit /B 0"; Flags: runhidden waituntilterminated; StatusMsg: "正在删除 Wintun 驱动..."; RunOnceId: "DeleteWintun"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
Type: filesandordirs; Name: "{commonappdata}\ScaleTail"

[Code]
procedure RunHidden(CommandLine: String);
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{cmd}'), '/C ' + CommandLine, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure StopScaleTailBeforeFileOps();
begin
  RunHidden('taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM ScaleTailUI.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletail.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" stop ScaleTail >NUL 2>NUL');
  RunHidden('timeout /T 2 /NOBREAK >NUL');
  RunHidden('taskkill /F /T /IM scaletaild.exe >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{app}\scaletaild.exe') + '" uninstall-system-daemon >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" delete ScaleTail >NUL 2>NUL');
end;

procedure CleanupCurrentUserScaleTailData();
begin
  RunHidden('if exist "%APPDATA%\ScaleTail" rmdir /S /Q "%APPDATA%\ScaleTail"');
  RunHidden('if exist "%LOCALAPPDATA%\ScaleTail" rmdir /S /Q "%LOCALAPPDATA%\ScaleTail"');
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
