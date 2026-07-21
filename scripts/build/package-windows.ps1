# package-windows.ps1 — Builds Windows installer using Inno Setup
# Requires: Inno Setup 6 (iscc.exe in PATH)
param(
    [string]$Version = "0.1.0",
    [string]$OutputDir = "dist",
    [switch]$SkipBuild = $false
)

$ErrorActionPreference = "Stop"

Write-Host "Building Lanos Windows installer v$Version"

if (-not $SkipBuild) {
    Set-Location "$PSScriptRoot/../../ui"
    flutter build windows --release
    Set-Location "$PSScriptRoot/../../core"
    go build -trimpath -ldflags="-s -w" -o gcd.exe ./cmd/gcd
}

# Create output directory
Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

# Copy files to staging
$staging = "$OutputDir/staging"
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path "$staging/app" | Out-Null
New-Item -ItemType Directory -Force -Path "$staging/core" | Out-Null

Copy-Item -Recurse "$PSScriptRoot/../../ui/build/windows/x64/runner/Release/*" "$staging/app/"
Copy-Item "$PSScriptRoot/../../core/gcd.exe" "$staging/core/"

# Create Inno Setup script
$iss = @"
[Setup]
AppName=Lanos
AppVersion=$Version
AppPublisher=Lanos
AppPublisherURL=https://lanos.app
AppSupportURL=https://lanos.app/support
AppUpdatesURL=https://lanos.app/download
DefaultDirName={autopf}\Lanos
DefaultGroupName=Lanos
OutputDir=$OutputDir
OutputBaseFilename=Lanos-Setup-$Version
SetupIconFile=$staging\app\data\flutter_assets\assets\icon.ico
Compression=lzma2/max
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "quicklaunchicon"; Description: "{cm:CreateQuickLaunchIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked; OnlyBelowVersion: 6.1

[Files]
Source: "$staging\app\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "$staging\core\gcd.exe"; DestDir: "{app}\core"; Flags: ignoreversion

[Icons]
Name: "{group}\Lanos"; Filename: "{app}\lanos.exe"
Name: "{group}\{cm:UninstallProgram,Lanos}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Lanos"; Filename: "{app}\lanos.exe"; Tasks: desktopicon
Name: "{userappdata}\Microsoft\Internet Explorer\Quick Launch\Lanos"; Filename: "{app}\lanos.exe"; Tasks: quicklaunchicon

[Run]
Filename: "{app}\lanos.exe"; Description: "{cm:LaunchProgram,Lanos}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
"@

$issPath = "$OutputDir/lanos.iss"
$iss | Out-File -FilePath $issPath -Encoding UTF8

# Run Inno Setup
Write-Host "Running Inno Setup..."
iscc "$issPath"

Write-Host "Done. Installer: $OutputDir/Lanos-Setup-$Version.exe"
