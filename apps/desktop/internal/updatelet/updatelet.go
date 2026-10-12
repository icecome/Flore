// Package updatelet 提供桌面端更新流程的状态机封装。
//
// 设计目标：更新下载必须独立于设置弹窗的生命周期。下载由后台 goroutine
// 驱动，状态集中在单一快照（State）中线程安全地存取，并在每次状态变化时
// 通过事件回调推送给订阅者。前端据此渲染进度条 / 按钮态：
//   - 关闭设置弹窗不影响后台下载（下载进程存活于桌面壳，而非前端组件）；
//   - 重新打开设置弹窗时，一次性拉取快照即可复原"正在下载 + 进度"；
//   - 前后端无需依赖"弹窗挂载期间持续轮询"来维持进度。
package updatelet

import (
	"fmt"
	"sync"

	"desktop/internal/updater"
)

// Status 描述更新流程的纵切状态。
type Status string

const (
	StatusIdle           Status = "idle"           // 无更新信息、无进行中任务
	StatusChecking       Status = "checking"       // 正在向远端查询更新
	StatusAvailable      Status = "available"      // 已发现新版本，等待用户发起下载
	StatusDownloading    Status = "downloading"    // 后台下载进行中
	StatusReady          Status = "ready"          // 下载完成，等待确认重启以应用
	StatusPendingRestart Status = "pendingRestart" // 已请求重启，进程即将退出
	StatusError          Status = "error"          // 检查或下载失败
)

// State 是更新流程的线程安全快照，前端一次性拉取即可复原全部展示状态。
type State struct {
	Status   Status              `json:"status"`
	Info     *updater.UpdateInfo `json:"info"`            // 最近一次有效更新信息；无则为 null
	Progress float64             `json:"progress"`        // 下载进度 0~1；非下载态为 0
	Error    string              `json:"error,omitempty"` // 最近一次失败原因，仅 Status==error 时非空
}

// Callback 由后端注入，在状态变化后触发（通常转发为 Wails 前端事件）。
type Callback func(State)

// Updater 是更新流程的状态机。
type Updater struct {
	appVer   string
	cb       Callback
	mu       sync.Mutex
	state    State
	inFlight bool // 是否存在进行中的检查或下载任务
}

// New 创建更新状态机。appVer 为当前版本号；cb 在每次状态变更后调用，可空。
func New(appVer string, cb Callback) *Updater {
	return &Updater{appVer: appVer, cb: cb, state: State{Status: StatusIdle}}
}

// Snapshot 返回当前状态的副本。
func (u *Updater) Snapshot() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// mutate 原子地应用状态变更并广播。
func (u *Updater) mutate(fn func(*State)) {
	u.mu.Lock()
	next := u.state
	fn(&next)
	u.state = next
	u.mu.Unlock()
	if u.cb != nil {
		u.cb(next)
	}
}

// Check 在后台发起版本检查（非阻塞）。存在进行中任务时立即返回错误。
func (u *Updater) Check() error {
	u.mu.Lock()
	if u.inFlight {
		u.mu.Unlock()
		return fmt.Errorf("更新任务进行中，请稍候")
	}
	u.inFlight = true
	u.mu.Unlock()

	go func() {
		u.mutate(func(s *State) {
			s.Status = StatusChecking
			s.Info = nil
			s.Progress = 0
			s.Error = ""
		})

		info, err := updater.CheckForUpdate(u.appVer)

		u.mu.Lock()
		u.inFlight = false
		u.mu.Unlock()

		if err != nil {
			u.mutate(func(s *State) {
				s.Status = StatusError
				s.Error = err.Error()
			})
			return
		}
		if info == nil {
			u.mutate(func(s *State) {
				s.Status = StatusIdle
				s.Info = nil
				s.Error = ""
			})
			return
		}
		u.mutate(func(s *State) {
			s.Status = StatusAvailable
			s.Info = info
			s.Error = ""
		})
	}()
	return nil
}

// Download 在后台下载已确认的更新（非阻塞）。无可用更新信息或已有任务进行中时返回错误。
// 下载完成后状态为 ready；调用方确认后需触发进程退出，由外部脚本完成文件替换。
func (u *Updater) Download() error {
	u.mu.Lock()
	if u.inFlight {
		u.mu.Unlock()
		return fmt.Errorf("更新任务进行中，请稍候")
	}
	info := u.state.Info
	if info == nil {
		u.mu.Unlock()
		return fmt.Errorf("当前没有可应用的更新")
	}
	u.inFlight = true
	u.mu.Unlock()

	go func() {
		u.mutate(func(s *State) {
			s.Status = StatusDownloading
			s.Progress = 0
			s.Error = ""
		})

		err := updater.ApplyUpdate(info, func(p float64) {
			u.mutate(func(s *State) { s.Progress = p })
		})

		u.mu.Lock()
		u.inFlight = false
		u.mu.Unlock()

		if err != nil {
			u.mutate(func(s *State) {
				s.Status = StatusError
				s.Progress = 0
				s.Error = err.Error()
			})
			return
		}
		u.mutate(func(s *State) {
			s.Status = StatusReady
			s.Progress = 1
			s.Error = ""
		})
	}()
	return nil
}

// ApplyReady 由前端在 ready 状态确认重启时调用，把状态置为 pendingRestart 并返回。
// 调用方随后负责退出进程以交由外部脚本完成文件替换。
func (u *Updater) ApplyReady() {
	u.mutate(func(s *State) {
		if s.Status == StatusReady {
			s.Status = StatusPendingRestart
		}
	})
}

// IsBusy 返回是否存在进行中的后台任务（供前端禁用按钮）。
func (u *Updater) IsBusy() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.inFlight
}
