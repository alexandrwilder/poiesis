# LOG_ · one line installs it on Windows (PowerShell):
#
#     irm https://raw.githubusercontent.com/alexandrwilder/log_/main/install.ps1 | iex
#
# Draft, not yet tested on Windows: downloads the command, puts it on your path, runs setup.
$ErrorActionPreference = "Stop"
$repo = if ($env:LOG_REPO) { $env:LOG_REPO } else { "alexandrwilder/log_" }
$dir = Join-Path $env:LOCALAPPDATA "LOG_"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$url = "https://github.com/$repo/releases/latest/download/log__windows_$arch.zip"
Write-Host "downloading LOG_ from $url"
$zip = Join-Path $env:TEMP "log_.zip"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dir -Force
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -notlike "*$dir*") { [Environment]::SetEnvironmentVariable("Path", "$path;$dir", "User") }
& (Join-Path $dir "log_.exe") setup
Write-Host "installed: $dir\log_.exe  (open a new terminal and run: log_)"
