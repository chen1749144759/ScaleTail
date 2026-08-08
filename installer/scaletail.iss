#ifndef AppVersion
  #define AppVersion "0.0.8"
#endif

[Setup]
AppName=ScaleTail
AppVersion={#AppVersion}
DefaultDirName={autopf}\ScaleTail
DefaultGroupName=ScaleTail
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
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

[Files]
Source: "..\dist\electron\win-unpacked\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs restartreplace uninsrestartdelete

[Icons]
Name: "{autoprograms}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{autodesktop}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; Tasks: desktopicon; IconFilename: "{app}\resources\app\resources\app.ico"
Name: "{commonstartup}\ScaleTail"; Filename: "{app}\ScaleTailUI.exe"; IconFilename: "{app}\resources\app\resources\app.ico"

[Run]
Filename: "{app}\ScaleTailUI.exe"; Parameters: "--open-dashboard"; Description: "启动 ScaleTail 客户端"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
Type: filesandordirs; Name: "{commonappdata}\ScaleTail"
Type: filesandordirs; Name: "{commonappdata}\ScaleTailOTA"

[Code]
var
  ServiceExistedBeforeSetup: Boolean;
  SetupServiceReady: Boolean;

procedure RunHidden(CommandLine: String);
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{cmd}'), '/C ' + CommandLine, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure ExecRequired(FileName: String; Parameters: String; Operation: String);
var
  ResultCode: Integer;
begin
  if not Exec(ExpandConstant(FileName), Parameters, '', SW_HIDE,
    ewWaitUntilTerminated, ResultCode) then
    RaiseException(Operation + '：无法启动命令');
  if ResultCode <> 0 then
    RaiseException(Format('%s失败，退出码 %d', [Operation, ResultCode]));
end;

function ScaleTailServiceExists(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec(ExpandConstant('{sys}\sc.exe'), 'query ScaleTail', '', SW_HIDE,
    ewWaitUntilTerminated, ResultCode) and (ResultCode = 0);
end;

procedure StartScaleTailServiceRequired();
var
  ResultCode: Integer;
begin
  if not Exec(ExpandConstant('{sys}\sc.exe'), 'start ScaleTail', '', SW_HIDE,
    ewWaitUntilTerminated, ResultCode) then
    RaiseException('启动 ScaleTail 服务：无法启动命令');
  if (ResultCode <> 0) and (ResultCode <> 1056) then
    RaiseException(Format('启动 ScaleTail 服务失败，退出码 %d', [ResultCode]));
end;

procedure InstallAndStartScaleTailService();
begin
  ExecRequired('{app}\scaletaild.exe', 'install-system-daemon', '安装或更新 ScaleTail 服务');
  StartScaleTailServiceRequired();
end;

procedure StopScaleTailForFileOps();
begin
  RunHidden('taskkill /F /T /IM ScaleTail.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM ScaleTailUI.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletail.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM scaletail-localapi.exe >NUL 2>NUL');
  RunHidden('taskkill /F /T /IM ScaleTailUpdateHelper.exe >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" stop ScaleTail >NUL 2>NUL');
  RunHidden('timeout /T 2 /NOBREAK >NUL');
  RunHidden('taskkill /F /T /IM scaletaild.exe >NUL 2>NUL');
end;

procedure UninstallScaleTailService();
begin
  if FileExists(ExpandConstant('{app}\scaletaild.exe')) then
    RunHidden('call "' + ExpandConstant('{app}\scaletaild.exe') + '" uninstall-system-daemon >NUL 2>NUL');
  RunHidden('call "' + ExpandConstant('{sys}\sc.exe') + '" delete ScaleTail >NUL 2>NUL');
end;

procedure CleanupCurrentUserScaleTailData();
begin
  RunHidden('if exist "%APPDATA%\ScaleTail" rmdir /S /Q "%APPDATA%\ScaleTail"');
  RunHidden('if exist "%LOCALAPPDATA%\ScaleTail" rmdir /S /Q "%LOCALAPPDATA%\ScaleTail"');
  RunHidden('"' + ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe') +
    '" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "Get-CimInstance Win32_UserProfile | Where-Object { -not $_.Special -and $_.LocalPath } | ForEach-Object { Remove-Item -LiteralPath (Join-Path $_.LocalPath ''AppData\Roaming\ScaleTail'') -Recurse -Force -ErrorAction SilentlyContinue; Remove-Item -LiteralPath (Join-Path $_.LocalPath ''AppData\Local\ScaleTail'') -Recurse -Force -ErrorAction SilentlyContinue }"');
end;

procedure CleanupScaleTailSystemArtifacts();
begin
  RunHidden('"' + ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe') +
    '" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$policy = Get-NetQosPolicy -Name ''ScaleTail-UploadThrottle'' -PolicyStore ActiveStore -ErrorAction SilentlyContinue; if ($policy) { Remove-NetQosPolicy -Name ''ScaleTail-UploadThrottle'' -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue }"');
end;

function IsOTAUpdate(): Boolean;
begin
  Result := (ExpandConstant('{param:OTAUPDATE|0}') = '1') and
    (ExpandConstant('{param:OTAMARKER|}') <> '');
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  ServiceExistedBeforeSetup := ScaleTailServiceExists();
  SetupServiceReady := False;
  StopScaleTailForFileOps();
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    InstallAndStartScaleTailService();
    SetupServiceReady := True;
    if IsOTAUpdate() then
      ExecRequired('{app}\ScaleTailUpdateHelper.exe',
        'signal --marker-id=' + ExpandConstant('{param:OTAMARKER|}'),
        '完成 ScaleTail 自动更新');
  end;
end;

procedure DeinitializeSetup();
var
  ResultCode: Integer;
begin
  if ServiceExistedBeforeSetup and not SetupServiceReady then
    Exec(ExpandConstant('{sys}\sc.exe'), 'start ScaleTail', '', SW_HIDE,
      ewWaitUntilTerminated, ResultCode);
end;

function InitializeUninstall(): Boolean;
begin
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then begin
    StopScaleTailForFileOps();
    UninstallScaleTailService();
    CleanupScaleTailSystemArtifacts();
    CleanupCurrentUserScaleTailData();
  end;
end;
