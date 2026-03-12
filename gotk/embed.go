//go:build !tinygo

package gotk

import (
	_ "embed"
	"net/http"
)

//go:embed client.js
var clientJS []byte

// ClientJSHandler returns an http.HandlerFunc that serves the thin client JS.
// Mount at "/gotk/client.js".
func ClientJSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(clientJS)
	}
}
