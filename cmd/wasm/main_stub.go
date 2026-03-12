//go:build !tinygo

package main

// Stub main for standard Go builds. The real WASM entry point
// is in main.go (tinygo build tag) which uses syscall/js.
func main() {}
