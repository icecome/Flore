//go:build !windows

package main

import (
	"os"
	"syscall"
	"time"
)

// 非 Windows 平台没有 Job Object / CreationFlags，这些常量仅为让调用方保持同一份代码。
const (
	createNewProcessGroupFlag  = 0
	createBreakawayFromJobFlag = 0
)

// hiddenSysProcAttr 在非 Windows 平台无需特殊属性。
func hiddenSysProcAttr() *syscall.SysProcAttr { return nil }

// detachedSysProcAttr 在非 Windows 平台无需特殊属性。
func detachedSysProcAttr() *syscall.SysProcAttr { return nil }

// assignToJob 在非 Windows 平台为 noop。
func assignToJob(p *os.Process) error { return nil }

// waitForProcessExit 轮询等待指定 PID 退出，最长等待 timeout。
func waitForProcessExit(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p, err := os.FindProcess(pid)
		if err != nil {
			return
		}
		if err := p.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
