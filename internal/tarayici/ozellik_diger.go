//go:build !windows

package tarayici

import "io/fs"

// Windows dışı platformlarda öznitelik kontrolleri devre dışıdır. Bu dosyanın
// tek amacı testlerin ve `go vet ./...` çağrısının her yerde çalışmasıdır.

// ReparseNoktasiMi symlink kontrolünü fs bilgisinden yapar.
func ReparseNoktasiMi(bilgi fs.FileInfo) bool {
	return bilgi != nil && bilgi.Mode()&fs.ModeSymlink != 0
}

// BulutYerTutucuMu Windows dışında her zaman false döner.
func BulutYerTutucuMu(fs.FileInfo) bool { return false }

// ozellikSistemMi Windows dışında her zaman false döner.
func ozellikSistemMi(fs.FileInfo) bool { return false }
