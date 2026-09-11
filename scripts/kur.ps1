<#
.SYNOPSIS
    Arsiv Indeks'i derler, %LOCALAPPDATA%\Programs\ArsivIndeks altina kurar
    ve oturum acilisinda otomatik baslamasi icin bir Gorev Zamanlayici gorevi
    kaydeder.

.NOTES
    Gelistirme kopyasina (bu depodaki .exe) dokunmaz; ayri bir kurulum
    kopyasi kullanir ki `go build` calisan sureci kilitlemesin.
#>

$ErrorActionPreference = 'Stop'

$GorevAdi = 'ArsivIndeks'
$Adres = '127.0.0.1:8080'
$KurulumDizini = Join-Path $env:LOCALAPPDATA 'Programs\ArsivIndeks'
$ExeAdi = 'arsiv-indeks.exe'
$KurulumExe = Join-Path $KurulumDizini $ExeAdi
$DepoKoku = Split-Path -Parent $PSScriptRoot

Write-Host "1/6 Calisan surec durduruluyor (varsa)..."
Get-Process -Name 'arsiv-indeks' -ErrorAction SilentlyContinue | Stop-Process -Force
try { Unregister-ScheduledTask -TaskName $GorevAdi -Confirm:$false -ErrorAction Stop } catch {}
Start-Sleep -Milliseconds 500

Write-Host "2/6 Kurulum dizini hazirlaniyor: $KurulumDizini"
New-Item -ItemType Directory -Force -Path $KurulumDizini | Out-Null

Write-Host "3/6 Versiyon guncelleniyor..."
# version.txt derlemeden ONCE guncellenir ki go:embed az sonraki derlemeye
# guncel surumu gomsun; aksi halde calisan binary bir onceki kurulumun
# surum numarasini gosterir.
$yeniVersiyon = & (Join-Path $PSScriptRoot 'versiyon-guncelle.ps1')

Write-Host "4/6 Derleniyor (-H=windowsgui, konsolsuz)..."
# Depodaki (git'e islenmis) arsiv-indeks.exe'ye hic dokunulmaz; cikti
# dogrudan kurulum dizinine yazilir.
Push-Location $DepoKoku
try {
    & go build -ldflags "-H=windowsgui" -o $KurulumExe .
    if ($LASTEXITCODE -ne 0) { throw "go build basarisiz (exit $LASTEXITCODE)" }
} finally {
    Pop-Location
}

Write-Host "5/6 Gorev Zamanlayici gorevi kaydediliyor: $GorevAdi"
$eylem = New-ScheduledTaskAction -Execute $KurulumExe -Argument "-adres $Adres" -WorkingDirectory $KurulumDizini
$tetikleyici = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$asil = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited
$ayarlar = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable

try {
    Register-ScheduledTask -TaskName $GorevAdi -Action $eylem -Trigger $tetikleyici `
        -Principal $asil -Settings $ayarlar -Force | Out-Null
} catch {
    Write-Error "Gorev kaydedilemedi: $($_.Exception.Message)`nBazi kurumsal makinelerde bu adim yonetici PowerShell gerektirebilir; bu pencereyi 'Yonetici olarak calistir' ile yeniden acip tekrar deneyin."
    exit 1
}

Write-Host "6/6 Baslat menusune kisayol birakiliyor..."
$baslatMenu = [Environment]::GetFolderPath('StartMenu')
$kisayolYolu = Join-Path $baslatMenu 'Arsiv Indeks.url'
@"
[InternetShortcut]
URL=http://$Adres
"@ | Set-Content -Path $kisayolYolu -Encoding ASCII

Write-Host "Gorev baslatiliyor..."
Start-ScheduledTask -TaskName $GorevAdi
Start-Sleep -Seconds 2

$durum = Get-ScheduledTask -TaskName $GorevAdi | Select-Object -ExpandProperty State

Write-Host ""
Write-Host "Kuruldu. Gorev durumu: $durum"
Write-Host "Surum:      $yeniVersiyon"
Write-Host "Adres:      http://$Adres"
Write-Host "Kurulum:    $KurulumExe"
Write-Host "Veri/Log:   $env:APPDATA\ArsivIndeks"
Write-Host "Kaldirmak icin: scripts\kaldir.ps1"
