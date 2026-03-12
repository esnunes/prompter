//go:build tinygo

package main

import (
	"syscall/js"

	"github.com/esnunes/prompter/gotk"
)

var registry *gotk.CommandRegistry

func init() {
	registry = gotk.NewCommandRegistry()
	registry.Register("scroll-conversation", ScrollConversation)
	registry.Register("update-form-visibility", UpdateFormVisibility)
	registry.Register("check-enter", CheckEnter)
}

func main() {
	// Expose listCommands and execCommand to JavaScript
	js.Global().Set("__gotk_listCommands", js.FuncOf(func(this js.Value, args []js.Value) any {
		return registry.ListCommandsJSON()
	}))
	js.Global().Set("__gotk_execCommand", js.FuncOf(func(this js.Value, args []js.Value) any {
		cmd := args[0].String()
		payload := args[1].String()
		return registry.ExecCommandJSON(cmd, payload)
	}))

	// Keep the Go program running (required for WASM)
	select {}
}
