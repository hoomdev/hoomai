# hoomAI installer para Windows
# Uso: irm https://raw.githubusercontent.com/hoomdev/hoomai/main/install.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo = if ($Env:HOOM_REPO) { $Env:HOOM_REPO } else { "hoomdev/hoomai" }
$Url  = "https://github.com/$Repo/releases/latest/download/hoom-windows-amd64.exe"
$Dir  = Join-Path $Env:LOCALAPPDATA "hoom"
$Bin  = Join-Path $Dir "hoom.exe"

Write-Host "hoomAI: descargando release mas reciente de $Repo..." -ForegroundColor Green
New-Item -ItemType Directory -Force -Path $Dir | Out-Null
Invoke-WebRequest -Uri $Url -OutFile $Bin -UseBasicParsing

# agregar al PATH del usuario si falta
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$Dir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$Dir", "User")
    $Env:Path = "$Env:Path;$Dir"
    Write-Host "hoomAI: $Dir agregado al PATH del usuario (abre una terminal nueva si no lo toma)" -ForegroundColor Yellow
}

& $Bin version
Write-Host "hoomAI: listo. Siguiente paso: cd a tu proyecto y ejecuta 'hoom init'" -ForegroundColor Green
