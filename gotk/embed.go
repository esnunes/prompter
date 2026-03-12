//go:build !tinygo

package gotk

import (
	_ "embed"
	"net/http"
)

//go:embed client.js
var clientJS []byte

//go:embed wasm_exec.js
var wasmExecJS []byte

//go:embed app.wasm
var appWASM []byte

// ClientJSHandler returns an http.HandlerFunc that serves the thin client JS.
// Mount at "/gotk/client.js".
func ClientJSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(clientJS)
	}
}

// WasmExecJSHandler returns an http.HandlerFunc that serves TinyGo's wasm_exec.js runtime.
// Mount at "/gotk/wasm_exec.js".
func WasmExecJSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(wasmExecJS)
	}
}

// AppWASMHandler returns an http.HandlerFunc that serves the compiled WASM binary.
// Mount at "/gotk/app.wasm".
func AppWASMHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(appWASM)
	}
}
