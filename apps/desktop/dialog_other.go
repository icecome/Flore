//go:build !windows

package main

import (
	"fmt"
	"os"
)

// showFatalError 在非 Windows 平台退化为写 stderr。
func showFatalError(title, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, message)
}
