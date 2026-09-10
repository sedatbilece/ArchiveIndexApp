<#
.SYNOPSIS
    Arsiv Indeks'in otomatik baslatma kurulumunu geri alir: gorevi siler,
    calisan sureci durdurur, kurulum dizinini ve kisayolu kaldirir.

.NOTES
    Veri dizinine (%APPDATA%\ArsivIndeks - ayarlar, indeks, log) DOKUNMAZ.
#>

$ErrorActionPreference = 'Stop'

$GorevAdi = 'ArsivIndeks'
$KurulumDizini = Join-Path $env:LOCALAPPDATA 'Programs\ArsivIndeks'
$kisayolYolu = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Arsiv Indeks.url'

Write-Host "Gorev kaldiriliyor: $GorevAdi"
try {
    Unregister-ScheduledTask -TaskName $GorevAdi -Confirm:$false -ErrorAction Stop
} catch {
    Write-Host "  (gorev zaten yoktu)"
}

Write-Host "Calisan surec durduruluyor (varsa)..."
Get-Process -Name 'arsiv-indeks' -ErrorAction SilentlyContinue | Stop-Process -Force

if (Test-Path $KurulumDizini) {
    Write-Host "Kurulum dizini siliniyor: $KurulumDizini"
    Remove-Item -Path $KurulumDizini -Recurse -Force
}

if (Test-Path $kisayolYolu) {
    Write-Host "Baslat menusu kisayolu siliniyor..."
    Remove-Item -Path $kisayolYolu -Force
}

Write-Host ""
Write-Host "Kaldirildi. Ayarlar ve indeks korundu: $env:APPDATA\ArsivIndeks"
