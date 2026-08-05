package services

import (
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Scheduler 最小后台调度器：按订阅源 interval 周期性向 Coordinator 提交抓取任务。
//
// 职责极简：到点查询需抓取的源，调用 coordinator.SubmitAll。
// 不再做并发控制、不去重、不加 atomic 标志——这些都由 Coordinator 统一负责。
type Scheduler struct {
	service  *ReaderService
	interval time.Duration
	ticker   *time.Ticker
	stop     chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex

	lastBackupAt    time.Time // 上次自动备份时间
	lastRetentionAt time.Time // 上次文章清理时间
}

// NewScheduler 创建调度器。checkInterval 指定检查周期，默认 5 分钟。
func NewScheduler(service *ReaderService, checkInterval time.Duration) *Scheduler {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Minute
	}
	sch := &Scheduler{
		service:  service,
		interval: checkInterval,
		stop:     make(chan struct{}),
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

// Start 启动调度器。启动后立即触发首轮抓取（去重保护下与周期抓取并发无害），
// 之后按 checkInterval 周期触发。
func (sch *Scheduler) Start() {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	if sch.running {
		return
	}
	sch.running = true
	sch.ticker = time.NewTicker(sch.interval)

	// 校正历史数据：旧逻辑可能把 nextCheckAtUnix 推到远未来，导致源长期不被调度
	sch.resetStaleNextCheckAt()

	// 首轮立即执行
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

// runOnce 执行一轮调度：查询到期的源并提交给 Coordinator，并触发备份与文章清理。
// 使用索引查询（NextCheckAtUnix <= now）替代全表扫描，O(log N) 而非 O(N)。
func (sch *Scheduler) runOnce() {
	toFetch, err := sch.service.getDueSources()
	if err != nil {
		slog.Error("scheduler: failed to query due sources", "error", err)
		return
	}

	if len(toFetch) > 0 && sch.service.Coordinator() != nil {
		n := sch.service.Coordinator().SubmitAll(toFetch)
		slog.Info("scheduler: submitted fetch tasks", "total", len(toFetch), "queued", n)
	} else {
		slog.Debug("scheduler: no sources due for fetch")
	}

	// 备份与文章清理（与抓取独立，互不阻塞）
	sch.wg.Add(1)
	go func() {
		defer sch.wg.Done()
		sch.maybeRunBackup()
		sch.maybeRunRetention()
	}()
}

// getDueSources 查询需要抓取的源 ID 列表。
// 优先使用 NextCheckAtUnix 索引（自适应调度），兼容无此字段的旧数据（回退到固定 interval）。
func (s *ReaderService) getDueSources() ([]int, error) {
	type dueSource struct {
		SourceID int `gorm:"column:source_id"`
	}
	var rows []dueSource
	now := time.Now().Unix()

	// 查询所有 active 源中 NextCheckAtUnix <= now 或 NextCheckAtUnix = 0（旧数据/首次）的源
	if err := s.getDb().Raw(`
		SELECT h.sourceId as source_id
		FROM SourceHealth h
		JOIN Source s ON s.id = h.sourceId
		WHERE s.active = true
		  AND (h.nextCheckAtUnix <= ? OR h.nextCheckAtUnix = 0)
		  AND h.nextRetryAtUnix = 0
		ORDER BY h.nextCheckAtUnix NULLS FIRST
	`, now).Scan(&rows).Error; err != nil {
		return nil, err
	}

	ids := make([]int, len(rows))
	for i, row := range rows {
		ids[i] = row.SourceID
	}
	return ids, nil
}

// resetStaleNextCheckAt 校正历史数据：被旧逻辑推到远未来（默认 3 天）的 nextCheckAtUnix
// 会导致源长期不被调度。启动时（首轮抓取前）将未处于失败退避的源拉回 now，使其立即进入调度。
func (sch *Scheduler) resetStaleNextCheckAt() {
	now := time.Now().Unix()
	res := sch.service.getDb().Exec(`
		UPDATE SourceHealth
		SET nextCheckAtUnix = ?
		WHERE nextRetryAtUnix = 0
		  AND nextCheckAtUnix > ?
	`, now, now)
	if res.Error != nil {
		slog.Warn("scheduler: failed to reset stale nextCheckAt", "error", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("scheduler: reset stale nextCheckAt for sources", "count", res.RowsAffected)
	}
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
