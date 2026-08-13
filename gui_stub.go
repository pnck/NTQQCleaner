//go:build !wails

package main

import "fmt"

// runGUI is replaced by main_wails.go when built with -tags wails.
// In CLI-only builds, invoking the (default) gui command shows usage
// instead of failing.
func runGUI() error {
	fmt.Println("此构建不含 GUI（GUI 版用 `make build` / `-tags wails` 构建）。CLI 用法：")
	fmt.Println()
	usage()
	return nil
}
