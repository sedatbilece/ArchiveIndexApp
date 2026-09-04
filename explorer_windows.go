//go:build windows

package main

import (
	"os/exec"
	"syscall"
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
