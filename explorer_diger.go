//go:build !windows

package main

import "errors"

func klasordeGoster(string) error {
	return errors.New("bu islem yalnizca Windows uzerinde desteklenir")
}
