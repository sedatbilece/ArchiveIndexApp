//go:build windows

package tarayici

import (
	"io/fs"
	"syscall"
)

// Windows dosya öznitelik bitleri.
//
// Bunları doğrudan okuyoruz çünkü os/fs katmanı bu bilgiyi vermiyor ve her
// biri gerçek bir hataya karşılık geliyor (bkz. aşağıdaki yorumlar).
const (
	ozSystem       = 0x00000004
	ozReparsePoint = 0x00000400
	ozOffline      = 0x00001000
	ozRecallOpen   = 0x00040000
	ozRecallData   = 0x00400000
)

func ozellikler(bilgi fs.FileInfo) uint32 {
	if bilgi == nil {
		return 0
	}
	veri, tamam := bilgi.Sys().(*syscall.Win32FileAttributeData)
	if !tamam || veri == nil {
		return 0
	}
	return veri.FileAttributes
}

// ReparseNoktasiMi symlink, junction veya mount point olup olmadığını söyler.
//
// filepath.WalkDir symlink izlemez, AMA dizin junction'ları Go sürümleri
// arasında tutarsız davranır. Özniteliği doğrudan okumak, klasik
// "C:\Documents and Settings" sonsuz döngüsüne karşı kesin çözümdür.
func ReparseNoktasiMi(bilgi fs.FileInfo) bool {
	return ozellikler(bilgi)&ozReparsePoint != 0
}

// BulutYerTutucuMu dosyanın buluttan indirilmesi gereken bir yer tutucu
// (dehydrated placeholder) olup olmadığını söyler.
//
// BURADAKİ EN ÖNEMLİ KONTROL BU. OneDrive yer tutucusunu okumak dosyayı
// sessizce indirir; 100.000 dosyalık bir arşivde bu, kullanıcının bağlantısı
// üzerinden gigabaytlarca indirme ve saatler demektir. Böyle dosyaları
// yalnızca adı, boyutu ve tarihiyle indeksliyoruz.
func BulutYerTutucuMu(bilgi fs.FileInfo) bool {
	oz := ozellikler(bilgi)
	return oz&(ozOffline|ozRecallOpen|ozRecallData) != 0
}

// ozellikSistemMi yalnızca SYSTEM bitine bakar.
//
// Klasör atlamada bunu kullanıyoruz, GizliSistemMi'yi değil: kullanıcının
// arşiv klasörü gizli işaretlenmiş olabilir ve onu sessizce atlamak sessiz
// veri kaybı olurdu.
func ozellikSistemMi(bilgi fs.FileInfo) bool {
	return ozellikler(bilgi)&ozSystem != 0
}
