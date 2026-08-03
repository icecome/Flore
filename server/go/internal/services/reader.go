package services

import (
	"log/slog"
	"net/http"
	"sync"

	"gorm.io/gorm"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
)

// ReaderService 阅读器业务逻辑
type ReaderService struct {
	db   *gorm.DB
	dbMu sync.RWMutex

	// unreadCountCache 缓存各订阅源未读数，避免每次 GetSources 都 GROUP BY 聚合。
	// 变更事件（标已读/抓取写入/删源/导入恢复）通过 invalidateUnreadCount 整体失效，
	// 下次访问自动重算；进程重启后首次访问亦自愈。
	unreadCountCache map[int]int64
	unreadCountMu    sync.RWMutex
	unreadCountReady bool

	// cachedHTTPClient 缓存的 HTTP 客户端，在代理设置变更时重建
	cachedHTTPClient    *http.Client
	cachedHTTPClientMu  sync.RWMutex
	cachedHTTPClientSeq int64
	proxySettingsSeq    int64

	// coordinator 是抓取动作的唯一调度权威，由 main 启动时注入。
	// 所有抓取入口（手动/调度/托盘）统一走 coordinator.Submit，去重与并发控制集中在此。
	coordinator *FetchCoordinator

	// filterRulesCache 缓存过滤规则，避免每篇新文章都查一次 DB
	filterRulesCache    []FilterRuleWithConditions
	filterRulesCacheMu  sync.RWMutex
	filterRulesCacheSeq int64 // 规则变更时递增，使缓存失效

	// indexChan 接收需建索引/应用规则的 itemID，后台 goroutine 批量处理
	// 避免每条新文章单独开事务，将 N 次 DB 写操作合并为 1 次批量事务
	indexChan chan int
	// indexWG 等待所有待处理 itemID 完成索引/规则应用后退出
	indexWG sync.WaitGroup
}

// 单例：确保 handler 和 scheduler 共享同一个实例，
// 数据库恢复后只需更新一次 db 引用即可同步所有调用方
var (
	readerServiceInstance *ReaderService
	readerServiceOnce     sync.Once
)

// NewReaderService 创建阅读器服务（单例）
func NewReaderService() *ReaderService {
	readerServiceOnce.Do(func() {
		s := &ReaderService{
			db:        database.DB,
			indexChan: make(chan int, 2048),
		}
		s.proxySettingsSeq = s.loadProxySettingsSeq()
		// 启动后台批量索引 goroutine，消费 indexChan 中的 itemID
		s.indexWG.Add(1)
		go s.indexWorker()
		readerServiceInstance = s
	})
	return readerServiceInstance
}

func (s *ReaderService) loadProxySettingsSeq() int64 {
	if s.GetSettingBool("proxyEnabled", false) && s.GetSetting("proxyUrl") != "" {
		return 1
	}
	return 0
}

func (s *ReaderService) getDb() *gorm.DB {
	s.dbMu.RLock()
	defer s.dbMu.RUnlock()
	return s.db
}

func (s *ReaderService) setDb(db *gorm.DB) {
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	s.db = db
}

// execLocked 在持有读锁期间执行 SQL。
// 根因修复 m-03：原 getDb() 在返回前就释放读锁，导致 ExportDatabase/Backup/Vacuum
// 在 Exec 执行时持有的 *gorm.DB 可能被并发的导入（写锁）关闭，产生竞态。
// 此处将读锁持有到 Exec 完成，杜绝该窗口。
func (s *ReaderService) execLocked(query string, args ...interface{}) error {
	s.dbMu.RLock()
	defer s.dbMu.RUnlock()
	return s.db.Exec(query, args...).Error
}

// SetCoordinator 注入抓取协调器，由 main 在启动 coordinator 后调用。
func (s *ReaderService) SetCoordinator(c *FetchCoordinator) {
	s.coordinator = c
}

// Coordinator 返回抓取协调器，供 handler/scheduler 调用 Submit。
func (s *ReaderService) Coordinator() *FetchCoordinator {
	return s.coordinator
}

// WaitIndexChan 等待后台批量索引 goroutine 处理完所有待处理 itemID。
// 供 Coordinator.worker 在抓取完成后调用，确保本轮新文章索引和规则应用均已完成。
func (s *ReaderService) WaitIndexChan() {
	s.indexWG.Wait()
}

// GetSources 获取所有订阅源，包含未读数与健康状态
func (s *ReaderService) indexWorker() {
	defer s.indexWG.Done()
	const batchSize = 256
	batch := make([]int, 0, batchSize)
	for {
		// 先取第一个 ID 阻塞，后续批量非阻塞补充
		id := <-s.indexChan
		batch = append(batch, id)
		for len(batch) < batchSize {
			select {
			case id := <-s.indexChan:
				batch = append(batch, id)
			default:
				goto process
			}
		}
	process:
		if err := s.indexBatch(batch); err != nil {
			slog.Warn("index batch failed", "count", len(batch), "error", err)
		}
		batch = batch[:0]
	}
}

// indexBatch 批量写入 FTS5 索引并应用过滤规则，单次事务完成。
func (s *ReaderService) indexBatch(itemIDs []int) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 批量删除旧索引
		if err := tx.Exec("DELETE FROM ItemSearch WHERE itemId IN ?", itemIDs).Error; err != nil {
			return err
		}
		// 批量插入新索引
		if err := tx.Exec(`
			INSERT INTO ItemSearch(title, content, itemId)
			SELECT i.title, COALESCE(i.desc, ''), i.id
			FROM Item i WHERE i.id IN ?
		`, itemIDs).Error; err != nil {
			return err
		}
		// 批量应用过滤规则：先查所有待处理文章及其源信息
		type itemSourceRow struct {
			ItemID         int    `gorm:"column:itemId"`
			Title          string `gorm:"column:title"`
			Desc           string `gorm:"column:desc"`
			SourceID       int    `gorm:"column:sourceId"`
			SourceName     string `gorm:"column:sourceName"`
			SourceFolderID *int   `gorm:"column:sourceFolderId"`
		}
		var rows []itemSourceRow
		if err := tx.Table("Item").
			Select(`Item.id as itemId, Item.title, COALESCE(Item.desc, '') as desc,
			        Item.sourceId, Source.name as sourceName, Source.folderId as sourceFolderId`).
			Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
			Where("Item.id IN ?", itemIDs).
			Scan(&rows).Error; err != nil {
			return err
		}
		// 应用过滤规则（复用现有逻辑，但改为批量）
		rules, err := s.GetFilterRules()
		if err != nil {
			return err
		}
		for _, row := range rows {
			item := models.ItemWithSource{
				Item:           models.Item{ID: row.ItemID, Title: row.Title, Desc: &row.Desc, SourceID: row.SourceID},
				SourceName:     row.SourceName,
				SourceFolderID: row.SourceFolderID,
			}
			for _, rule := range rules {
				if !rule.Enabled {
					continue
				}
				if !ruleMatchesScope(rule, item) {
					continue
				}
				if !ruleMatchesConditions(rule.Conditions, item) {
					continue
				}
				if _, err := s.applyRuleAction(tx, row.ItemID, rule.Action); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
