package utils

import (
	"runtime/debug"

	"github.com/gomessguii/logger"
)

// Go roda fn numa goroutine protegida contra panic. Em Go, um panic não
// recuperado em qualquer goroutine encerra o processo, e com ele todas as
// instâncias; o whatsmeow só protege os próprios handlers de evento.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// Recover registra e contém um panic. Use como primeiro defer de uma
// goroutine anônima: `defer utils.Recover("nome")`.
func Recover(name string) {
	if r := recover(); r != nil {
		logger.LogError("recovered panic in goroutine %s: %v\n%s", name, r, debug.Stack())
	}
}
