//go:build windows

package main

import "syscall"

func startForceKiller() {}

func rawExit(code int) {
	syscall.Exit(code)
}
