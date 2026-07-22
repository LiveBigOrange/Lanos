param(
    [string]$Version = "0.1.0",
    [string]$OutputDir = "dist",
    [switch]$SkipBuild = $false
)

$ErrorActionPreference = "Stop"

$Version = $Version -replace '^v', ''

Write-Host "Packaging Lanos Windows v$Version"

$ProjectRoot = Resolve-Path "$PSScriptRoot/../.."
$GcdExe = "$ProjectRoot/core/gcd.exe"

if (-not $SkipBuild) {
    Set-Location "$ProjectRoot/core"
    go build -trimpath -ldflags="-s -w" -o gcd.exe ./cmd/gcd
}

Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$staging = "$PSScriptRoot/$OutputDir/staging"
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path "$staging/app" | Out-Null

if (Test-Path $GcdExe) {
    Copy-Item "$GcdExe" "$staging/app/"
    Write-Host "Copied gcd.exe from $GcdExe"
} else {
    Write-Error "gcd.exe not found at $GcdExe"
}

$portable = "$PSScriptRoot/$OutputDir/portable/Lanos"
if (Test-Path $portable) { Remove-Item -Recurse -Force $portable }
New-Item -ItemType Directory -Force -Path $portable | Out-Null

Copy-Item "$staging/app/gcd.exe" $portable/

Compress-Archive -Path $portable -DestinationPath "$PSScriptRoot/$OutputDir/Lanos-$Version-windows-portable.zip" -Force
Write-Host "Created portable ZIP: $OutputDir/Lanos-$Version-windows-portable.zip"

$iss = @"
[Setup]
AppName=Lanos
AppVersion=$Version
AppPublisher=Lanos
AppPublisherURL=https://github.com/LiveBigOrange/Lanos
DefaultDirName={autopf}\Lanos
DefaultGroupName=Lanos
OutputDir=$PSScriptRoot\$OutputDir
OutputBaseFilename=Lanos-Setup-$Version
Compression=lzma2/max
SolidCompression=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "$staging\app\gcd.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Lanos"; Filename: "{app}\gcd.exe"
Name: "{group}\{cm:UninstallProgram,Lanos}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Lanos"; Filename: "{app}\gcd.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\gcd.exe"; Description: "{cm:LaunchProgram,Lanos}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "taskkill.exe"; Parameters: "/F /IM gcd.exe"; Flags: runhidden; RunOnceId: "KillGcd"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
"@

$issPath = "$OutputDir/lanos.iss"
$iss | Out-File -FilePath $issPath -Encoding UTF8

Write-Host "Running Inno Setup..."
iscc "$issPath"

Write-Host "Done. Installer: $OutputDir/Lanos-Setup-$Version.exe"
