//go:build darwin

package main

// #include <stdlib.h>
import "C"

//export goTrayOpen
func goTrayOpen() { go trayOpen(trayRoot) }

//export goTrayRecord
func goTrayRecord() { go trayRecord(trayRoot) }
