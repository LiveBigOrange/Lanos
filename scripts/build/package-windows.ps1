param(
    [string]$Version = "0.1.0",
    [string]$OutputDir = "dist",
    [switch]$SkipBuild = $false
)

$ErrorActionPreference = "Stop"

$Version = $Version -replace '^v', ''

Write-Host "Building Lanos Windows installer v$Version"

$ProjectRoot = Resolve-Path "$PSScriptRoot/../.."
$FlutterRelease = "$ProjectRoot/ui/build/windows/x64/runner/Release"
$GcdExe = "$ProjectRoot/core/gcd.exe"

if (-not $SkipBuild) {
    Set-Location "$ProjectRoot/ui"
    flutter build windows --release
    Set-Location "$ProjectRoot/core"
    go build -trimpath -ldflags="-s -w" -o gcd.exe ./cmd/gcd
}

Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$staging = (Resolve-Path "$OutputDir/staging").Path
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path "$staging/app" | Out-Null
New-Item -ItemType Directory -Force -Path "$staging/core" | Out-Null

if (Test-Path $FlutterRelease) {
    Copy-Item -Recurse "$FlutterRelease/*" "$staging/app/"
    Write-Host "Copied Flutter release from $FlutterRelease"
} else {
    Write-Error "Flutter release not found at $FlutterRelease"
}

if (Test-Path $GcdExe) {
    Copy-Item "$GcdExe" "$staging/core/"
    Write-Host "Copied gcd.exe from $GcdExe"
} else {
    Write-Error "gcd.exe not found at $GcdExe"
}

$iss = @"
[Setup]
AppName=Lanos
AppVersion=$Version
AppPublisher=Lanos
AppPublisherURL=https://github.com/LiveBigOrange/Lanos
DefaultDirName={autopf}\Lanos
DefaultGroupName=Lanos
OutputDir=$OutputDir
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
Source: "$staging\app\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "$staging\core\gcd.exe"; DestDir: "{app}\core"; Flags: ignoreversion

[Icons]
Name: "{group}\Lanos"; Filename: "{app}\lanos.exe"
Name: "{group}\{cm:UninstallProgram,Lanos}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Lanos"; Filename: "{app}\lanos.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\lanos.exe"; Description: "{cm:LaunchProgram,Lanos}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
"@

$issPath = "$OutputDir/lanos.iss"
$iss | Out-File -FilePath $issPath -Encoding UTF8

Write-Host "Running Inno Setup..."
iscc "$issPath"

Write-Host "Done. Installer: $OutputDir/Lanos-Setup-$Version.exe"
