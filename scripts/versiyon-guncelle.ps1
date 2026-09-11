<#
.SYNOPSIS
    version.txt icindeki surumu bir sonraki yama numarasina tasir ve
    tarih damgasini bugune gunceller: v<buyuk>.<kucuk>.<yama>.<GGAAYY>

.NOTES
    Iki yerden cagrilir: kullanici acikca "versiyon guncelle" istediginde,
    veya kur.ps1'in en sonunda otomatik olarak (her kurulum = yeni surum).
#>

$ErrorActionPreference = 'Stop'

$VersiyonDosyasi = Join-Path (Split-Path -Parent $PSScriptRoot) 'version.txt'

$mevcut = if (Test-Path $VersiyonDosyasi) { (Get-Content $VersiyonDosyasi -Raw).Trim() } else { '' }

if ($mevcut -match '^v(\d+)\.(\d+)\.(\d+)\.\d{6}$') {
    $buyuk = [int]$matches[1]
    $kucuk = [int]$matches[2]
    $yama = [int]$matches[3] + 1
} else {
    $buyuk = 0
    $kucuk = 0
    $yama = 1
}

$tarih = Get-Date -Format 'ddMMyy'
$yeniVersiyon = "v$buyuk.$kucuk.$yama.$tarih"

Set-Content -Path $VersiyonDosyasi -Value $yeniVersiyon -NoNewline -Encoding ASCII
Write-Host "Versiyon: $mevcut -> $yeniVersiyon"

return $yeniVersiyon
