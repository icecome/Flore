//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// createNewProcessGroupFlag 让子进程脱离父进程的 Ctrl 事件传播。
const createNewProcessGroupFlag = windows.CREATE_NEW_PROCESS_GROUP

// createBreakawayFromJobFlag 让新进程脱离当前 Job Object，
// 用于 RestartApp 启动的新实例：否则当前实例退出关闭 Job 时会连带把新实例杀掉。
const createBreakawayFromJobFlag = 0x01000000 // CREATE_BREAKAWAY_FROM_JOB

// hiddenSysProcAttr 返回隐藏控制台窗口的子进程属性（用于后端服务）。
func hiddenSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

// detachedSysProcAttr 返回「脱离当前 Job Object + 独立进程组」的子进程属性，
// 供 RestartApp 启动新实例使用。
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    false,
		CreationFlags: createNewProcessGroupFlag | createBreakawayFromJobFlag,
	}
}

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

// ensureJobObject 惰性创建进程级 Job Object。
// 使用 JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE：当主进程（无论正常退出、崩溃还是被强杀）
// 的最后一个 Job 句柄关闭时，内核会自动终止 Job 内所有子进程，杜绝孤儿后端。
// 同时开启 JOB_OBJECT_LIMIT_BREAKAWAY_OK，允许 RestartApp 的新实例显式脱离。
func ensureJobObject() (windows.Handle, error) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = fmt.Errorf("create job object: %w", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
					windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK,
			},
		}
		if _, err := windows.SetInformationJobObject(
			h,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(h)
			jobErr = fmt.Errorf("set job object limits: %w", err)
			return
		}
		jobHandle = h
	})
	return jobHandle, jobErr
}

// assignToJob 把已启动的子进程加入主进程 Job Object。
// 必须在 cmd.Start() 成功之后调用。
func assignToJob(p *os.Process) error {
	if p == nil {
		return fmt.Errorf("nil process")
	}
	job, err := ensureJobObject()
	if err != nil {
		return err
	}
	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(p.Pid),
	)
	if err != nil {
		return fmt.Errorf("open process %d: %w", p.Pid, err)
	}
	defer windows.CloseHandle(h)

	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		return fmt.Errorf("assign process %d to job: %w", p.Pid, err)
	}
	return nil
}

// waitForProcessExit 阻塞等待指定 PID 的进程退出，最长等待 timeout。
// 用于 RestartApp 场景：新实例必须等旧实例释放单实例互斥体后才能启动。
func waitForProcessExit(pid int, timeout time.Duration) {
	if pid <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// 进程已不存在（或无权限），无需等待
		return
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
}
