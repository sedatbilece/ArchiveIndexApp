//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// klasordeGoster dosyayı Windows Gezgini'nde seçili olarak açar.
//
// Komut satırını SysProcAttr.CmdLine ile elle kuruyoruz. Sebebi: explorer.exe
// standart dışı argüman ayrıştırır ve `/select,` ile yolun tek bir argüman
// gibi görünmesini, yolun ise ayrıca tırnaklanmasını bekler. Go'nun normal
// exec.Command argüman kaçırması boşluklu yollarda tüm ifadeyi tırnaklar
// (`"/select,C:\a b\c.txt"`) ve explorer bunu ayrıştıramaz.
//
// `cmd /c start` KULLANMIYORUZ: argüman enjeksiyonuna açıktır.
func klasordeGoster(yol string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `explorer.exe /select,"` + yol + `"`,
	}
	// explorer.exe BAŞARILI olduğunda bile 1 çıkış kodu döndürür; hatayı
	// bilerek yoksayıyoruz. Start() zaten sürecin başlatılamadığı durumu
	// yakalıyor.
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // zombi süreç bırakmayalım
	return nil
}

var shellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

// sistemdeAc dosyayı Windows'ta varsayılan uygulamasıyla açar.
//
// ShellExecuteW'yi doğrudan çağırıyoruz: komut satırı kurulmadığı için
// dosya adındaki & ^ " gibi karakterlerle argüman enjeksiyonu mümkün değil.
func sistemdeAc(yol string) error {
	islem, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	dosya, err := syscall.UTF16PtrFromString(yol)
	if err != nil {
		return err
	}
	dizin, err := syscall.UTF16PtrFromString(filepath.Dir(yol))
	if err != nil {
		return err
	}
	const swGoster = 1 // SW_SHOWNORMAL
	r, _, _ := shellExecute.Call(0,
		uintptr(unsafe.Pointer(islem)), uintptr(unsafe.Pointer(dosya)), 0,
		uintptr(unsafe.Pointer(dizin)), swGoster)
	// ShellExecute 32'den büyük bir değer döndürürse başarılıdır.
	if r <= 32 {
		return fmt.Errorf("ShellExecute hata kodu %d", r)
	}
	return nil
}
