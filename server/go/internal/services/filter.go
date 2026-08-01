package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
)

// FilterCondition 过滤条件
type FilterCondition struct {
	Field    string `json:"field"`    // title | desc | author | link
	Operator string `json:"operator"` // contains | notContains | equals | notEquals
	Value    string `json:"value"`
}

// CreateFilterRuleRequest 创建规则输入
type CreateFilterRuleRequest struct {
	Name       string            `json:"name"`
	Enabled    bool              `json:"enabled"`
	Priority   int               `json:"priority"`
	Scope      string            `json:"scope"`
	SourceID   *int              `json:"sourceId"`
	FolderID   *int              `json:"folderId"`
	Conditions []FilterCondition `json:"conditions"`
	Action     string            `json:"action"`
}

// FilterRuleWithConditions 返回给前端的规则（conditions 已解析）
type FilterRuleWithConditions struct {
	models.FilterRule
	Conditions []FilterCondition `json:"conditions"`
}

// 合法的 scope 与 action 取值，避免任意字符串写入数据库
var validFilterScopes = map[string]bool{
	"global": true,
	"source": true,
	"folder": true,
}

var validFilterActions = map[string]bool{
	"markRead":  true,
	"star":      true,
	"readLater": true,
}

var validFilterFields = map[string]bool{
	"title":  true,
	"desc":   true,
	"author": true,
	"link":   true,
}

var validFilterOperators = map[string]bool{
	"contains":    true,
	"notContains": true,
	"equals":      true,
	"notEquals":   true,
}

// validateFilterInput 校验规则输入的合法性
func validateFilterInput(input CreateFilterRuleRequest) error {
	if !validFilterScopes[input.Scope] {
		return fmt.Errorf("invalid scope: %q", input.Scope)
	}
	if !validFilterActions[input.Action] {
		return fmt.Errorf("invalid action: %q", input.Action)
	}
	// scope 为 source/folder 时必须指定对应 ID
	if input.Scope == "source" && input.SourceID == nil {
		return fmt.Errorf("sourceId is required when scope is source")
	}
	if input.Scope == "folder" && input.FolderID == nil {
		return fmt.Errorf("folderId is required when scope is folder")
	}
	// 校验 conditions 的 field 与 operator
	for _, c := range input.Conditions {
		if !validFilterFields[c.Field] {
			return fmt.Errorf("invalid condition field: %q", c.Field)
		}
		if !validFilterOperators[c.Operator] {
			return fmt.Errorf("invalid condition operator: %q", c.Operator)
		}
	}
	return nil
}

// CreateFilterRule 创建过滤规则
func (s *ReaderService) CreateFilterRule(input CreateFilterRuleRequest) (*FilterRuleWithConditions, error) {
	if err := validateFilterInput(input); err != nil {
		return nil, err
	}
	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		return nil, err
	}

	now := models.MilliTime{T: time.Now()}
	rule := models.FilterRule{
		Name:       input.Name,
		Enabled:    input.Enabled,
		Priority:   input.Priority,
		Scope:      input.Scope,
		SourceID:   input.SourceID,
		FolderID:   input.FolderID,
		Conditions: string(conditionsJSON),
		Action:     input.Action,
		CreatedAt:  now,
	}
	if err := s.getDb().Create(&rule).Error; err != nil {
		return nil, err
	}
	s.InvalidateFilterRulesCache()
	return s.wrapFilterRule(rule)
}

// GetFilterRules 获取所有过滤规则，按优先级降序。
// 使用内存缓存避免批量抓取时每篇新文章都查一次 DB。
// 调用 InvalidateFilterRulesCache 可使缓存失效。
func (s *ReaderService) GetFilterRules() ([]FilterRuleWithConditions, error) {
	s.filterRulesCacheMu.RLock()
	if s.filterRulesCache != nil {
		cached := s.filterRulesCache
		s.filterRulesCacheMu.RUnlock()
		return cached, nil
	}
	s.filterRulesCacheMu.RUnlock()

	var rules []models.FilterRule
	if err := s.getDb().Order("priority desc, id desc").Find(&rules).Error; err != nil {
		return nil, err
	}
	result := make([]FilterRuleWithConditions, 0, len(rules))
	for _, r := range rules {
		wrapped, err := s.wrapFilterRule(r)
		if err != nil {
			slog.Warn("failed to parse filter rule conditions, skipping", "rule_id", r.ID, "error", err)
			continue
		}
		result = append(result, *wrapped)
	}

	s.filterRulesCacheMu.Lock()
	s.filterRulesCache = result
	s.filterRulesCacheMu.Unlock()

	return result, nil
}

// InvalidateFilterRulesCache 使过滤规则缓存失效。
// 在规则增删改时调用。
func (s *ReaderService) InvalidateFilterRulesCache() {
	s.filterRulesCacheMu.Lock()
	s.filterRulesCache = nil
	s.filterRulesCacheMu.Unlock()
}

// UpdateFilterRule 更新规则
func (s *ReaderService) UpdateFilterRule(id int, input CreateFilterRuleRequest) (*FilterRuleWithConditions, error) {
	if err := validateFilterInput(input); err != nil {
		return nil, err
	}

	var rule models.FilterRule
	if err := s.getDb().First(&rule, id).Error; err != nil {
		return nil, err
	}

	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"name":       input.Name,
		"enabled":    input.Enabled,
		"priority":   input.Priority,
		"scope":      input.Scope,
		"sourceId":   input.SourceID,
		"folderId":   input.FolderID,
		"conditions": string(conditionsJSON),
		"action":     input.Action,
	}
	if err := s.getDb().Model(&rule).Updates(updates).Error; err != nil {
		return nil, err
	}

	if err := s.getDb().First(&rule, id).Error; err != nil {
		return nil, err
	}
	s.InvalidateFilterRulesCache()
	return s.wrapFilterRule(rule)
}

// DeleteFilterRule 删除规则
func (s *ReaderService) DeleteFilterRule(id int) error {
	if err := s.getDb().Delete(&models.FilterRule{}, id).Error; err != nil {
		return err
	}
	s.InvalidateFilterRulesCache()
	return nil
}

// wrapFilterRule 解析 conditions JSON
func (s *ReaderService) wrapFilterRule(rule models.FilterRule) (*FilterRuleWithConditions, error) {
	var conditions []FilterCondition
	if rule.Conditions != "" {
		if err := json.Unmarshal([]byte(rule.Conditions), &conditions); err != nil {
			return nil, err
		}
	}
	return &FilterRuleWithConditions{
		FilterRule: rule,
		Conditions: conditions,
	}, nil
}

// ApplyFilterRules 对指定文章应用过滤规则。
// 使用事务包裹所有规则动作，确保多规则叠加时数据一致性。
// 缓存失效（invalidateUnreadCount）延迟到事务提交后执行，避免事务回滚时误触发重算。
func (s *ReaderService) ApplyFilterRules(itemID int) error {
	var item models.ItemWithSource
	if err := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url, Source.folderId as source_folder_id").
		Joins("LEFT JOIN Source ON Item.sourceId = Source.id").
		Where("Item.id = ?", itemID).
		Scan(&item).Error; err != nil {
		return err
	}

	rules, err := s.GetFilterRules()
	if err != nil {
		return err
	}

	markReadTriggered := false
	err = s.getDb().Transaction(func(tx *gorm.DB) error {
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
			marked, err := s.applyRuleAction(tx, itemID, rule.Action)
			if err != nil {
				return err
			}
			if marked {
				markReadTriggered = true
			}
			// 一条规则匹配并执行后继续，允许多条规则叠加
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 事务提交成功后再失效缓存
	if markReadTriggered {
		s.invalidateUnreadCount()
	}
	return nil
}

func ruleMatchesScope(rule FilterRuleWithConditions, item models.ItemWithSource) bool {
	switch rule.Scope {
	case "source":
		// scope 为 source 时必须指定 sourceId，否则不匹配
		if rule.SourceID == nil {
			return false
		}
		return item.SourceID == *rule.SourceID
	case "folder":
		// scope 为 folder 时必须指定 folderId，否则不匹配
		if rule.FolderID == nil {
			return false
		}
		return item.SourceFolderID != nil && *item.SourceFolderID == *rule.FolderID
	default:
		// global scope 匹配所有文章
		return true
	}
}

func ruleMatchesConditions(conditions []FilterCondition, item models.ItemWithSource) bool {
	if len(conditions) == 0 {
		return false
	}
	for _, cond := range conditions {
		if !conditionMatches(cond, item) {
			return false
		}
	}
	return true
}

func conditionMatches(cond FilterCondition, item models.ItemWithSource) bool {
	if strings.TrimSpace(cond.Value) == "" {
		return false
	}

	value := getFieldValue(cond.Field, item)
	value = strings.ToLower(value)
	condValue := strings.ToLower(cond.Value)

	return compareValues(value, condValue, cond.Operator)
}

func getFieldValue(field string, item models.ItemWithSource) string {
	switch field {
	case "title":
		return item.Title
	case "desc":
		if item.Desc != nil {
			return *item.Desc
		}
	case "author":
		if item.Author != nil {
			return *item.Author
		}
	case "link":
		return item.Link
	}
	return ""
}

func compareValues(value, condValue, operator string) bool {
	switch operator {
	case "contains":
		return strings.Contains(value, condValue)
	case "notContains":
		return !strings.Contains(value, condValue)
	case "equals":
		return value == condValue
	case "notEquals":
		return value != condValue
	default:
		return strings.Contains(value, condValue)
	}
}

// applyRuleAction 执行单条规则动作。
// 返回 marked=true 表示执行了 markRead 动作（调用方据此决定是否失效未读数缓存）。
func (s *ReaderService) applyRuleAction(tx *gorm.DB, itemID int, action string) (bool, error) {
	switch action {
	case "markRead":
		if err := tx.Model(&models.Item{}).Where("id = ?", itemID).Update("isRead", true).Error; err != nil {
			return false, err
		}
		return true, nil
	case "star":
		return false, tx.Model(&models.Item{}).Where("id = ?", itemID).Update("isStarred", true).Error
	case "readLater":
		return false, tx.Model(&models.Item{}).Where("id = ?", itemID).Update("isReadLater", true).Error
	default:
		return false, fmt.Errorf("unknown filter action: %s", action)
	}
}

// TestFilterRule 测试指定规则匹配到的最近文章
// 取最近 200 篇文章进行匹配，返回命中的前 20 条
func (s *ReaderService) TestFilterRule(id int) ([]models.ItemWithSource, error) {
	var ruleModel models.FilterRule
	if err := s.getDb().First(&ruleModel, id).Error; err != nil {
		return nil, err
	}
	rule, err := s.wrapFilterRule(ruleModel)
	if err != nil {
		return nil, err
	}

	// 获取最近 200 篇文章
	items := []models.ItemWithSource{}
	if err := s.getDb().Table("Item").
		Select("Item.*, Source.name as source_name, Source.url as source_url, Source.folderId as source_folder_id").
		Joins("LEFT JOIN Source ON Source.id = Item.sourceId").
		Order("Item.createdAt DESC").
		Limit(200).
		Scan(&items).Error; err != nil {
		return nil, err
	}

	var matched []models.ItemWithSource
	for _, item := range items {
		if ruleMatchesScope(*rule, item) && ruleMatchesConditions(rule.Conditions, item) {
			matched = append(matched, item)
			if len(matched) >= 20 {
				break
			}
		}
	}
	return matched, nil
}


