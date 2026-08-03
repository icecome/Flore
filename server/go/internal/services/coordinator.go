package services

import (
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rss/go-server/internal/models"
)

// FetchCoordinator 是抓取动作的唯一调度权威。
// 所有触发入口（手动刷新、调度器、托盘、autoFetch）统一通过 Submit 提交任务，
// 由 Coordinator 负责去重、并发控制、状态查询。
//
// 设计原则：一个抓取动作从入口到完成是一条直线，
// 没有冷却期、没有 CAS 兜底、没有前端 guard，去重只在此处发生一次。
type FetchCoordinator struct {
	service *ReaderService

	// inFlight: 正在抓取的 sourceID 集合
	// pending: 已入队但未开始的 sourceID 集合
	// 两者互斥，同一 sourceID 任意时刻最多存在于一处
	inFlight map[int]struct{}
	pending  map[int]struct{}
	stopped  bool // Stop 后置 true，Submit 优先检查避免向已关闭 taskCh 发送
	mu       sync.Mutex

	taskCh chan int // 任务队列，worker 从此 chan 取 sourceID

	wg   sync.WaitGroup // 优雅关闭：等待所有 worker 退出
	stop chan struct{}

	// startedAt 记录本轮抓起始时间，全部空闲时清零，用于判断 fetching 状态
	startedAt atomic.Int64 // unix nano，0 表示空闲

	// newItemsThisRound 本轮进行中实时累计新增条目数，由 worker 在每个源完成后累加。
	// 抓取期间前端可读到进行中累计值；完成时刻被冻结到 lastRoundNewItems 后清零。
	newItemsThisRound atomic.Int64
	// lastRoundNewItems 本轮完成时冻结的最终新增数，供 FetchStatus 返回给前端做通知计数。
	lastRoundNewItems atomic.Int64

	// settingsCache: 并发度等设置缓存，避免每源查 DB
	concurrency int
}

// NewFetchCoordinator 创建协调器，但不启动 worker。
// 调用 Start() 启动 worker 池。
func NewFetchCoordinator(service *ReaderService) *FetchCoordinator {
	c := &FetchCoordinator{
		service:     service,
		inFlight:    make(map[int]struct{}),
		pending:     make(map[int]struct{}),
		taskCh:      make(chan int, 256),
		stop:        make(chan struct{}),
		concurrency: 10,
	}
	return c
}

// Start 启动 worker 池。并发度从设置表读取（默认 10，上限 30）。
func (c *FetchCoordinator) Start() {
	n := c.service.GetSettingInt("fetchConcurrency", 10)
	if n <= 0 {
		n = 10
	}
	if n > 30 {
		n = 30
	}
	c.concurrency = n
	for i := 0; i < n; i++ {
		c.wg.Add(1)
		go c.worker()
	}
	slog.Info("fetch coordinator started", "workers", n)
}

// Stop 优雅关闭：先标记 stopped 拒绝新任务（Submit 在 mu 内检查），
// 再关闭 stop 通知 Submit 的 select 立即返回，
// 然后关闭 taskCh 让 worker 处理完队列剩余任务后退出，
// 最后等待所有 worker 完成。
// 安全性：stopped 标志保证 Submit 不会在 close(taskCh) 后向其发送。
func (c *FetchCoordinator) Stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	c.mu.Unlock()

	close(c.stop)
	close(c.taskCh)
	c.wg.Wait()
	slog.Info("fetch coordinator stopped")
}

// Submit 提交单个源抓取任务。
// 重复提交（已在 inFlight 或 pending）自动丢弃，返回 false。
// 队列已满时也丢弃并返回 false，避免阻塞调用方。
func (c *FetchCoordinator) Submit(sourceID int) bool {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return false
	}
	if _, ok := c.inFlight[sourceID]; ok {
		c.mu.Unlock()
		return false
	}
	if _, ok := c.pending[sourceID]; ok {
		c.mu.Unlock()
		return false
	}
	c.pending[sourceID] = struct{}{}
	// 首个任务标记本轮开始
	if c.startedAt.Load() == 0 {
		c.startedAt.Store(time.Now().UnixNano())
	}
	c.mu.Unlock()

	select {
	case c.taskCh <- sourceID:
		return true
	case <-c.stop:
		c.mu.Lock()
		delete(c.pending, sourceID)
		c.mu.Unlock()
		return false
	default:
		// 队列满，丢弃任务
		c.mu.Lock()
		delete(c.pending, sourceID)
		c.mu.Unlock()
		slog.Warn("fetch coordinator queue full, dropped", "source_id", sourceID)
		return false
	}
}

// SubmitAll 批量提交，返回成功入队数量。
func (c *FetchCoordinator) SubmitAll(sourceIDs []int) int {
	count := 0
	for _, id := range sourceIDs {
		if c.Submit(id) {
			count++
		}
	}
	return count
}

// SubmitAllSources 查询所有 active 源并提交抓取。
// 供手动"刷新全部"和调度器使用。
// skipBackedOff=true 时排除处于退避期的僵尸源（抓取持续失败的源），
// 避免每次全量刷新都被这些源（超时/503）拖慢整体，同时正常源仍全部更新。
func (c *FetchCoordinator) SubmitAllSources(skipBackedOff bool) int {
	var sourceIDs []int64
	if err := c.service.getDb().Model(&models.Source{}).Where("active = ?", true).Pluck("id", &sourceIDs).Error; err != nil {
		slog.Error("fetch coordinator: query sources failed", "error", err)
		return 0
	}
	if skipBackedOff {
		var backedOff []int64
		if err := c.service.getDb().Model(&models.SourceHealth{}).
			Where("nextRetryAtUnix > ?", time.Now().Unix()).
			Pluck("sourceId", &backedOff).Error; err != nil {
			slog.Warn("fetch coordinator: query backed-off sources failed", "error", err)
		} else if len(backedOff) > 0 {
			excluded := make(map[int64]struct{}, len(backedOff))
			for _, id := range backedOff {
				excluded[id] = struct{}{}
			}
			filtered := sourceIDs[:0]
			for _, id := range sourceIDs {
				if _, ok := excluded[id]; !ok {
					filtered = append(filtered, id)
				}
			}
			sourceIDs = filtered
		}
	}
	ids := make([]int, len(sourceIDs))
	for i, id := range sourceIDs {
		ids[i] = int(id)
	}
	return c.SubmitAll(ids)
}

// backoffSeconds 根据连续失败次数返回退避秒数（指数退避，基准 5 分钟，封顶 6 小时）。
func backoffSeconds(failCount int) int {
	const baseSec = 300
	const capSec = 21600
	if failCount < 1 {
		return 0
	}
	backoff := baseSec
	for i := 1; i < failCount && backoff < capSec; i++ {
		backoff *= 2
	}
	if backoff > capSec {
		backoff = capSec
	}
	return backoff
}

// nextRetryUnix 返回下次允许抓取的时间戳（Unix 秒），0 表示无退避。
func nextRetryUnix(failCount int) int64 {
	secs := backoffSeconds(failCount)
	if secs <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(secs) * time.Second).Unix()
}

// SubmitFolder 提交文件夹下所有源抓取。
func (c *FetchCoordinator) SubmitFolder(folderID int) int {
	var sourceIDs []int64
	if err := c.service.getDb().Model(&models.Source{}).Where("folderId = ? AND active = ?", folderID, true).Pluck("id", &sourceIDs).Error; err != nil {
		slog.Error("fetch coordinator: query folder sources failed", "folder_id", folderID, "error", err)
		return 0
	}
	ids := make([]int, len(sourceIDs))
	for i, id := range sourceIDs {
		ids[i] = int(id)
	}
	return c.SubmitAll(ids)
}

// IsFetching 返回当前是否有抓取任务进行中（含队列待处理）。
// 供前端轮询判断何时停止刷新按钮旋转。
func (c *FetchCoordinator) IsFetching() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inFlight) > 0 || len(c.pending) > 0
}

// LastRoundNewItems 返回上一轮抓取完成时的累计新增条目数。
// 仅当抓取空闲（fetching=false）时为该轮最终值，供前端通知计数使用，
// 根治此前用「全库未读差值」近似新增数导致的计数失真（C-02）。
func (c *FetchCoordinator) LastRoundNewItems() int {
	return int(c.lastRoundNewItems.Load())
}

// worker 从任务队列取 sourceID，执行抓取流水线。
// 新文章 FTS5 索引由后台 goroutine 异步处理，worker 不等待，降低串行阻塞。
func (c *FetchCoordinator) worker() {
	defer c.wg.Done()
	for sourceID := range c.taskCh {
		// 单源 panic 不能拖垮整个 worker：recover 并记录，同时清理 inFlight 记账，
		// 避免该源永久卡在去重状态而无法再次提交。
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("fetch worker panic recovered",
						"source_id", sourceID, "panic", r,
						"stack", string(debug.Stack()))
					c.mu.Lock()
					delete(c.inFlight, sourceID)
					c.mu.Unlock()
				}
			}()
			c.processTask(sourceID)
		}()
	}
}

// processTask 处理单个抓取任务：记账迁移 + 抓取 + 记账回收。
func (c *FetchCoordinator) processTask(sourceID int) {
	// 从 pending 移到 inFlight
	c.mu.Lock()
	delete(c.pending, sourceID)
	c.inFlight[sourceID] = struct{}{}
	c.mu.Unlock()

	// 执行抓取（含 upsert + 健康更新），新文章索引由后台 goroutine 异步处理
	newCount, err := c.service.FetchSourceFeed(sourceID)
	if err != nil {
		slog.Warn("fetch source feed failed", "source_id", sourceID, "error", err)
	} else {
		c.newItemsThisRound.Add(int64(newCount))
	}

	// 完成后移除 inFlight，若全部空闲则清零本轮起始时间
	c.mu.Lock()
	delete(c.inFlight, sourceID)
	if len(c.inFlight) == 0 && len(c.pending) == 0 {
		// 本轮完成：冻结新增数供 fetch-status 读取，并清零进行中计数
		c.lastRoundNewItems.Store(c.newItemsThisRound.Load())
		c.newItemsThisRound.Store(0)
		c.startedAt.Store(0)
	}
	c.mu.Unlock()
}
