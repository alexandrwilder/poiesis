# Poiesis · one line installs it on Windows (PowerShell):
#
#     irm https://raw.githubusercontent.com/alexandrwilder/poiesis/main/install.ps1 | iex
#
# Draft, not yet tested on Windows: downloads the command, puts it on your path, runs setup.
$ErrorActionPreference = "Stop"
$repo = if ($env:POIESIS_REPO) { $env:POIESIS_REPO } else { "alexandrwilder/poiesis" }
$dir = Join-Path $env:LOCALAPPDATA "Poiesis"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$url = "https://github.com/$repo/releases/latest/download/poiesis_windows_$arch.zip"
Write-Host "downloading Poiesis from $url"
$zip = Join-Path $env:TEMP "poiesis.zip"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dir -Force
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -notlike "*$dir*") { [Environment]::SetEnvironmentVariable("Path", "$path;$dir", "User") }
& (Join-Path $dir "poiesis.exe") setup
Write-Host "installed: $dir\poiesis.exe  (open a new terminal and run: poiesis)"
