package services

import (
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Scheduler 最小后台调度器：按全局轮询周期周期性向 Coordinator 提交全部 active 源的抓取任务。
//
// 设计原则（全量轮询 + 304 协商）：
//   - 不再做「到期才抓」的筛选——RSS 源何时发文章不可预测，「到期制」必然漏抓已发布文章。
//   - 每个轮询周期提交所有 active 源（跳过退避中的坏源），未变化的源靠 HTTP 304 协商零成本跳过。
//   - 因此无论手动刷新还是自动调度，源内已正常发布的文章一定会被抓到，行为完全一致。
// 职责极简：到点调用 coordinator.SubmitAllSources，由 Coordinator 统一负责去重/并发/退避。
type Scheduler struct {
	service *ReaderService

	// checkInterval 检查节拍：多久判断一次「是否到了轮询时间」，与轮询周期解耦。
	checkInterval time.Duration
	ticker        *time.Ticker
	stop          chan struct{}
	wg            sync.WaitGroup
	running       bool
	mu            sync.Mutex

	lastFullPollAt  time.Time // 上次全量轮询时间，用于按 fetchInterval/defaultInterval 节流
	lastBackupAt    time.Time // 上次自动备份时间
	lastRetentionAt time.Time // 上次文章清理时间
}

// NewScheduler 创建调度器。checkInterval 指定检查节拍（默认 1 分钟），
// 实际全量轮询周期由设置表 fetchInterval/defaultInterval 控制（默认 30 分钟）。
func NewScheduler(service *ReaderService, checkInterval time.Duration) *Scheduler {
	if checkInterval <= 0 {
		checkInterval = 1 * time.Minute
	}
	sch := &Scheduler{
		service:       service,
		checkInterval: checkInterval,
		stop:          make(chan struct{}),
	}
	sch.loadPersistedScheduleState()
	return sch
}

// loadPersistedScheduleState 从 Setting 表恢复上次执行时间，
// 避免进程重启后立刻重复触发备份/清理（M-R3 根因：调度状态不应是临时变量）
func (sch *Scheduler) loadPersistedScheduleState() {
	if t := sch.service.GetSettingInt("lastBackupAtUnix", 0); t > 0 {
		sch.lastBackupAt = time.Unix(int64(t), 0)
	}
	if t := sch.service.GetSettingInt("lastRetentionAtUnix", 0); t > 0 {
		sch.lastRetentionAt = time.Unix(int64(t), 0)
	}
}

func (sch *Scheduler) persistLastBackupAt(t time.Time) {
	if err := sch.service.UpdateSettings(map[string]string{
		"lastBackupAtUnix": strconv.FormatInt(t.Unix(), 10),
	}); err != nil {
		slog.Warn("scheduler: failed to persist lastBackupAt", "error", err)
	}
}

func (sch *Scheduler) persistLastRetentionAt(t time.Time) {
	if err := sch.service.UpdateSettings(map[string]string{
		"lastRetentionAtUnix": strconv.FormatInt(t.Unix(), 10),
	}); err != nil {
		slog.Warn("scheduler: failed to persist lastRetentionAt", "error", err)
	}
}

// Start 启动调度器。首轮行为遵循「启动时自动抓取」设置（autoFetchOnStart，由前端拥有）：
//   - 开启：立即全量轮询一次，打开即最新；
//   - 关闭：不立即抓取，把首次轮询推迟到第一个周期（fetchInterval），
//     避免与前端 useFetchOrchestration 的启动抓取逻辑冲突。
// 之后按 checkInterval 节拍检查，达到轮询周期则再触发全量抓取。
func (sch *Scheduler) Start() {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	if sch.running {
		return
	}
	sch.running = true
	sch.ticker = time.NewTicker(sch.checkInterval)

	if sch.service.GetSettingBool("autoFetchOnStart", false) {
		// 首轮立即执行（去重保护下与周期抓取并发无害）
		sch.wg.Add(1)
		go func() {
			defer sch.wg.Done()
			select {
			case <-sch.stop:
				return
			default:
				sch.runOnce()
			}
		}()
	} else {
		// 关闭启动抓取：把上次全量轮询时间置为现在，启动后首个周期才轮询
		sch.lastFullPollAt = time.Now()
	}

	// 周期性调度
	sch.wg.Add(1)
	go func() {
		defer sch.wg.Done()
		for {
			select {
			case <-sch.ticker.C:
				sch.runOnce()
			case <-sch.stop:
				return
			}
		}
	}()
}

// Stop 停止调度器并等待当前轮次完成。
func (sch *Scheduler) Stop() {
	sch.mu.Lock()
	if !sch.running {
		sch.mu.Unlock()
		return
	}
	sch.running = false
	if sch.ticker != nil {
		sch.ticker.Stop()
	}
	close(sch.stop)
	sch.mu.Unlock()
	sch.wg.Wait()
}

// runOnce 执行一轮调度：达到轮询周期则全量提交所有 active 源给 Coordinator，
// 并触发备份与文章清理。未达轮询周期则跳过抓取（备份/清理有各自节流）。
func (sch *Scheduler) runOnce() {
	periodMin := sch.pollPeriodMinutes()
	sch.mu.Lock()
	due := sch.lastFullPollAt.IsZero() || time.Since(sch.lastFullPollAt) >= time.Duration(periodMin)*time.Minute
	sch.mu.Unlock()

	if due && sch.service.Coordinator() != nil {
		n := sch.service.Coordinator().SubmitAllSources(true)
		slog.Info("scheduler: full-poll submitted", "queued", n, "period_min", periodMin)
		sch.mu.Lock()
		sch.lastFullPollAt = time.Now()
		sch.mu.Unlock()
	} else {
		slog.Debug("scheduler: full-poll not due yet", "period_min", periodMin)
	}

	// 备份与文章清理（与抓取独立，互不阻塞）
	sch.wg.Add(1)
	go func() {
		defer sch.wg.Done()
		sch.maybeRunBackup()
		sch.maybeRunRetention()
	}()
}

// pollPeriodMinutes 返回全量轮询周期（分钟）。优先 fetchInterval，回退 defaultInterval，默认 30。
// 旧的「到期制」调度已废弃：RSS 源发布时间不可预测，按固定周期全量轮询 + 304 协商才是最优解。
func (sch *Scheduler) pollPeriodMinutes() int {
	if m := sch.service.GetSettingInt("fetchInterval", 0); m > 0 {
		return m
	}
	if m := sch.service.GetSettingInt("defaultInterval", 30); m > 0 {
		return m
	}
	return 30
}

// maybeRunBackup 根据设置决定是否触发自动备份
func (sch *Scheduler) maybeRunBackup() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler: panic during backup", "panic", r)
		}
	}()

	if !sch.service.GetSettingBool("backupAutoEnabled", false) {
		return
	}

	intervalMin := sch.service.GetSettingInt("backupAutoInterval", 1440)
	if intervalMin <= 0 {
		return
	}

	now := time.Now()
	sch.mu.Lock()
	lastBackup := sch.lastBackupAt
	sch.mu.Unlock()
	if !lastBackup.IsZero() && now.Sub(lastBackup) < time.Duration(intervalMin)*time.Minute {
		return
	}

	name, err := sch.service.CreateCompressedBackup()
	if err != nil {
		slog.Warn("scheduler: auto backup failed", "error", err)
		return
	}
	sch.mu.Lock()
	sch.lastBackupAt = now
	sch.mu.Unlock()
	sch.persistLastBackupAt(now)
	slog.Debug("scheduler: auto backup created", "name", name)

	// 清理过期备份
	maxKeep := sch.service.GetSettingInt("backupMaxKeep", 10)
	maxDays := sch.service.GetSettingInt("backupMaxDays", 30)
	if deleted, err := sch.service.CleanupBackups(maxKeep, maxDays); err != nil {
		slog.Warn("scheduler: backup cleanup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("scheduler: cleaned expired backups", "deleted", deleted)
	}

	// 自动整理压缩：备份完成后对实时库做一次 VACUUM，回收删除产生的空闲空间（M-P1）
	if err := sch.service.Vacuum(); err != nil {
		slog.Warn("scheduler: auto vacuum failed", "error", err)
	}
}

// maybeRunRetention 根据设置决定是否触发文章清理
func (sch *Scheduler) maybeRunRetention() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler: panic during retention cleanup", "panic", r)
		}
	}()

	now := time.Now()
	sch.mu.Lock()
	lastRetention := sch.lastRetentionAt
	sch.mu.Unlock()
	if !lastRetention.IsZero() && now.Sub(lastRetention) < 24*time.Hour {
		return
	}

	retentionDays := sch.service.GetSettingInt("articleRetentionDays", 0)
	retentionMax := sch.service.GetSettingInt("articleRetentionMax", 0)
	if retentionDays <= 0 && retentionMax <= 0 {
		return
	}

	excludeStarred := sch.service.GetSettingBool("retentionExcludeStarred", true)
	excludeReadLater := sch.service.GetSettingBool("retentionExcludeReadLater", true)

	deleted, err := sch.service.CleanupArticles(retentionDays, retentionMax, excludeStarred, excludeReadLater)
	if err != nil {
		slog.Warn("scheduler: article retention cleanup failed", "error", err)
		return
	}
	sch.mu.Lock()
	sch.lastRetentionAt = now
	sch.mu.Unlock()
	sch.persistLastRetentionAt(now)
	if deleted > 0 {
		slog.Debug("scheduler: retention cleanup done", "deleted", deleted)
		// 清理后回收空间，避免实时库体积虚高（M-P1）
		if err := sch.service.Vacuum(); err != nil {
			slog.Warn("scheduler: auto vacuum after retention failed", "error", err)
		}
	}
}
