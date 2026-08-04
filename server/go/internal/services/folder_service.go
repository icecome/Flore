package services

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

func (s *ReaderService) GetFolders() ([]models.Folder, error) {
	var folders []models.Folder
	if err := s.getDb().Find(&folders).Error; err != nil {
		return nil, err
	}
	return folders, nil
}

// CreateFolder 创建文件夹
func (s *ReaderService) CreateFolder(name string) (*models.Folder, error) {
	now := models.MilliTime{T: time.Now()}
	folder := models.Folder{Name: name, CreatedAtTime: now, UpdatedAtTime: now}
	if err := s.getDb().Create(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

// UpdateFolder 更新文件夹
func (s *ReaderService) UpdateFolder(id int, name string) error {
	return s.getDb().Model(&models.Folder{}).Where("id = ?", id).Update("name", name).Error
}

// DeleteFolder 删除文件夹，保留下级文件夹并提升到根层级
func (s *ReaderService) DeleteFolder(id int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 将文件夹内的 Source 移出到根目录
		if err := tx.Model(&models.Source{}).Where("folderId = ?", id).Update("folderId", nil).Error; err != nil {
			return err
		}
		// 将子文件夹提升到根层级
		if err := tx.Model(&models.Folder{}).Where("parentId = ?", id).Update("parentId", nil).Error; err != nil {
			return err
		}
		// 删除文件夹本身
		return tx.Delete(&models.Folder{}, id).Error
	})
}

// MoveSourceFolder 移动订阅源到指定文件夹，并自动清理空文件夹
func (s *ReaderService) MoveSourceFolder(sourceID int, folderID *int) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		// 获取源当前的文件夹
		var source models.Source
		if err := tx.First(&source, sourceID).Error; err != nil {
			return err
		}
		oldFolderID := source.FolderID

		// 移动源
		if err := tx.Model(&models.Source{}).Where("id = ?", sourceID).Update("folderId", folderID).Error; err != nil {
			return err
		}

		// 如果源原来属于某个文件夹，检查并清理空文件夹
		if oldFolderID != nil {
			return s.cleanupEmptyFolder(tx, *oldFolderID)
		}
		return nil
	})
}

// getAllFolderAllDescendantIDs 获取文件夹及其所有子文件夹的ID（包含自身）。
// 一次查询加载全部文件夹，在内存中按父子关系 BFS 遍历，避免逐层查询的 N+1。
func (s *ReaderService) getAllFolderAllDescendantIDs(folderID int) ([]int, error) {
	var folders []models.Folder
	if err := s.getDb().Find(&folders).Error; err != nil {
		return nil, err
	}
	childrenOf := make(map[int][]int)
	for _, f := range folders {
		if f.ParentID != nil {
			childrenOf[*f.ParentID] = append(childrenOf[*f.ParentID], f.ID)
		}
	}

	allIDs := []int{folderID}
	queue := []int{folderID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, childID := range childrenOf[parent] {
			allIDs = append(allIDs, childID)
			queue = append(queue, childID)
		}
	}
	return allIDs, nil
}

// buildItemQuery 构建文章查询的公共条件（供 GetItems / CountItems 复用）
const (
	// opmlMaxDepth 限制 outline 递归深度。正常 OPML 层级很少超过 5 层，32 层足够冗余
	opmlMaxDepth = 32
	// opmlMaxNodes 限制单次导入的 outline 节点总数。10000 个订阅源已远超普通用户需求
	opmlMaxNodes = 10000
)

// OPML 导入导出结构
type opmlDocument struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	Outlines []opmlOutline `xml:"outline"`
}

// ImportOPML 使用 Go 原生 XML 解析导入 OPML。
//
// XML DoS 防御说明：
//   - Go 标准库 encoding/xml 默认不展开 DTD 实体，billion laughs 等实体扩展攻击天然无效
//   - handler 层已通过 http.MaxBytesReader 限制 body 大小为 10MB
//   - 此处额外限制 outline 节点总数与递归深度，防止深层嵌套导致栈溢出或节点爆炸消耗内存
func (s *ReaderService) ImportOPML(xmlData string) error {
	doc, err := s.parseOPMLDocument(xmlData)
	if err != nil {
		return err
	}
	return s.importOPMLBody(doc.Body.Outlines)
}

func (s *ReaderService) parseOPMLDocument(xmlData string) (*opmlDocument, error) {
	var doc opmlDocument
	if err := xml.Unmarshal([]byte(xmlData), &doc); err != nil {
		return nil, fmt.Errorf("parse opml failed: %w", err)
	}
	return &doc, nil
}

func (s *ReaderService) importOPMLBody(outlines []opmlOutline) error {
	return s.getDb().Transaction(func(tx *gorm.DB) error {
		folderMap := make(map[string]int)
		visited := 0
		return s.importOPMLOutlines(tx, outlines, folderMap, &visited)
	})
}

func (s *ReaderService) importOPMLOutlines(tx *gorm.DB, outlines []opmlOutline, folderMap map[string]int, visited *int) error {
	for _, outline := range outlines {
		if err := s.importOutlineRecursive(tx, outline, nil, "", folderMap, 0, visited); err != nil {
			return err
		}
	}
	return nil
}

// importOutlineRecursive 递归处理 OPML outline
// 叶子节点（有xmlUrl）创建源，非叶子节点视为文件夹
// 使用 ParentID 创建多层级文件夹结构
//
// depth 为当前递归深度，visited 为整棵树已访问的节点计数器（指针）。
// 超过 opmlMaxDepth 或 opmlMaxNodes 时返回错误，防御深层嵌套与节点爆炸。
func (s *ReaderService) importOutlineRecursive(tx *gorm.DB, outline opmlOutline, parentID *int, parentPath string, folderMap map[string]int, depth int, visited *int) error {
	if err := s.checkOPMLDepthLimit(depth, visited); err != nil {
		return err
	}

	folderName := firstNonEmpty(outline.Text, outline.Title)
	currentPath := buildOPMLPath(parentPath, folderName)

	if err := s.importOPMLChildren(tx, outline, parentID, currentPath, folderMap, depth, visited); err != nil {
		return err
	}

	if outline.XMLURL != "" {
		return s.createSourceFromOPML(tx, parentID, outline)
	}
	return nil
}

func (s *ReaderService) checkOPMLDepthLimit(depth int, visited *int) error {
	if depth > opmlMaxDepth {
		return fmt.Errorf("opml: outline nesting depth exceeds limit %d", opmlMaxDepth)
	}
	*visited++
	if *visited > opmlMaxNodes {
		return fmt.Errorf("opml: total outline nodes exceeds limit %d", opmlMaxNodes)
	}
	return nil
}

func (s *ReaderService) importOPMLChildren(tx *gorm.DB, outline opmlOutline, parentID *int, currentPath string, folderMap map[string]int, depth int, visited *int) error {
	for _, child := range outline.Outlines {
		currentFolderID, err := resolveOPMLFolderID(tx, firstNonEmpty(outline.Text, outline.Title), currentPath, parentID, folderMap)
		if err != nil {
			return err
		}
		if err := s.importOutlineRecursive(tx, child, currentFolderID, currentPath, folderMap, depth+1, visited); err != nil {
			return err
		}
	}
	return nil
}

// buildOPMLPath 构建 OPML 当前全路径
func buildOPMLPath(parentPath, folderName string) string {
	if folderName == "" {
		return parentPath
	}
	if parentPath != "" {
		return parentPath + "/" + folderName
	}
	return folderName
}

// resolveOPMLFolderID 创建或获取当前层级文件夹 ID
func resolveOPMLFolderID(tx *gorm.DB, folderName, currentPath string, parentID *int, folderMap map[string]int) (*int, error) {
	if folderName == "" || currentPath == "" {
		return parentID, nil
	}
	if id, exists := folderMap[currentPath]; exists {
		return &id, nil
	}
	now := models.MilliTime{T: time.Now()}
	newFolder := models.Folder{
		Name:          folderName,
		ParentID:      parentID,
		CreatedAtTime: now,
		UpdatedAtTime: now,
	}
	if err := tx.Create(&newFolder).Error; err != nil {
		return nil, fmt.Errorf("failed to create OPML folder %q: %w", folderName, err)
	}
	folderMap[currentPath] = newFolder.ID
	return &newFolder.ID, nil
}

// createSourceFromOPML 从 OPML outline 创建订阅源（轻量 URL 格式校验）
func (s *ReaderService) createSourceFromOPML(tx *gorm.DB, folderID *int, outline opmlOutline) error {
	url := outline.XMLURL
	if url == "" {
		return nil
	}
	// 轻量级校验：只检查格式，不发起 DNS 查询。SSRF 防护在 fetcher.go 拉取时做
	if err := ValidateURLOnly(url); err != nil {
		slog.Warn("opml: skipped invalid URL", "url", url, "error", err)
		return nil // 跳过无效 URL，继续处理下一个
	}
	name := firstNonEmpty(outline.Text, outline.Title, url)
	now := models.MilliTime{T: time.Now()}
	source := models.Source{
		Name:          name,
		URL:           url,
		FolderID:      folderID,
		ListRule:      "rss",
		Interval:      120,
		Active:        true,
		CreatedAtTime: now,
		UpdatedAtTime: now,
	}
	return tx.Create(&source).Error
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ExportOPML 使用 Go 原生 XML 生成 OPML，支持嵌套文件夹树
func (s *ReaderService) ExportOPML() (string, error) {
	sources, err := s.GetSources()
	if err != nil {
		return "", err
	}
	folders, err := s.GetFolders()
	if err != nil {
		return "", err
	}

	folderSources, noFolderSources := groupSourcesByFolder(sources)
	folderChildren, rootFolders := buildFolderTree(folders)

	outlines := buildOPMLOutlines(noFolderSources, rootFolders, folderSources, folderChildren)

	doc := opmlDocument{
		Version: "2.0",
		Head:    opmlHead{Title: "Flore Subscriptions"},
		Body:    opmlBody{Outlines: outlines},
	}

	output, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return xml.Header + string(output), nil
}

// groupSourcesByFolder 按 folderId 分组源
func groupSourcesByFolder(sources []models.Source) (map[int][]models.Source, []models.Source) {
	folderSources := make(map[int][]models.Source)
	var noFolderSources []models.Source
	for _, src := range sources {
		if src.FolderID != nil {
			folderSources[*src.FolderID] = append(folderSources[*src.FolderID], src)
		} else {
			noFolderSources = append(noFolderSources, src)
		}
	}
	return folderSources, noFolderSources
}

// buildFolderTree 构建文件夹树：parentId -> children
func buildFolderTree(folders []models.Folder) (map[int][]models.Folder, []models.Folder) {
	folderChildren := make(map[int][]models.Folder)
	var rootFolders []models.Folder
	for _, f := range folders {
		if f.ParentID != nil {
			folderChildren[*f.ParentID] = append(folderChildren[*f.ParentID], f)
		} else {
			rootFolders = append(rootFolders, f)
		}
	}
	return folderChildren, rootFolders
}

// buildOPMLOutlines 构建 OPML outline 列表
func buildOPMLOutlines(noFolderSources []models.Source, rootFolders []models.Folder, folderSources map[int][]models.Source, folderChildren map[int][]models.Folder) []opmlOutline {
	var buildFolderOutlines func(folderID int) []opmlOutline
	buildFolderOutlines = func(folderID int) []opmlOutline {
		var children []opmlOutline
		for _, cf := range folderChildren[folderID] {
			children = append(children, opmlOutline{
				Text:     cf.Name,
				Title:    cf.Name,
				Outlines: buildFolderOutlines(cf.ID),
			})
		}
		for _, src := range folderSources[folderID] {
			children = append(children, opmlOutline{
				Text:   src.Name,
				Title:  src.Name,
				Type:   "rss",
				XMLURL: src.URL,
			})
		}
		return children
	}

	var outlines []opmlOutline
	for _, src := range noFolderSources {
		outlines = append(outlines, opmlOutline{
			Text:   src.Name,
			Title:  src.Name,
			Type:   "rss",
			XMLURL: src.URL,
		})
	}
	for _, folder := range rootFolders {
		outlines = append(outlines, opmlOutline{
			Text:     folder.Name,
			Title:    folder.Name,
			Outlines: buildFolderOutlines(folder.ID),
		})
	}
	return outlines
}

// CountItems 获取文章总数（支持 sourceId/folderId/unread/starred/readLater 筛选）
