package services

import (
	"testing"

	"github.com/rss/go-server/internal/models"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
func boolPtr(b bool) *bool    { return &b }

func TestValidateFilterInput(t *testing.T) {
	cases := []struct {
		name    string
		input   CreateFilterRuleRequest
		wantErr bool
	}{
		{
			name: "合法 global 规则",
			input: CreateFilterRuleRequest{
				Name: "r", Enabled: true, Scope: "global", Action: "markRead",
				Conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "广告"}},
			},
		},
		{
			name: "合法 source 规则",
			input: CreateFilterRuleRequest{
				Scope: "source", Action: "star", SourceID: intPtr(1),
				Conditions: []FilterCondition{{Field: "link", Operator: "equals", Value: "https://example.com"}},
			},
		},
		{
			name: "合法 folder 规则",
			input: CreateFilterRuleRequest{
				Scope: "folder", Action: "readLater", FolderID: intPtr(2),
				Conditions: []FilterCondition{{Field: "author", Operator: "notContains", Value: "bot"}},
			},
		},
		{name: "非法 scope", input: CreateFilterRuleRequest{Scope: "everything", Action: "markRead"}, wantErr: true},
		{name: "非法 action", input: CreateFilterRuleRequest{Scope: "global", Action: "delete"}, wantErr: true},
		{name: "source scope 缺 sourceId", input: CreateFilterRuleRequest{Scope: "source", Action: "markRead"}, wantErr: true},
		{name: "folder scope 缺 folderId", input: CreateFilterRuleRequest{Scope: "folder", Action: "markRead"}, wantErr: true},
		{
			name: "非法 condition field",
			input: CreateFilterRuleRequest{Scope: "global", Action: "markRead",
				Conditions: []FilterCondition{{Field: "content", Operator: "contains", Value: "x"}}},
			wantErr: true,
		},
		{
			name: "非法 condition operator",
			input: CreateFilterRuleRequest{Scope: "global", Action: "markRead",
				Conditions: []FilterCondition{{Field: "title", Operator: "regex", Value: "x"}}},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilterInput(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFilterInput() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestRuleMatchesScope(t *testing.T) {
	item := models.ItemWithSource{
		Item:           models.Item{SourceID: 7},
		SourceFolderID: intPtr(3),
	}
	noFolderItem := models.ItemWithSource{Item: models.Item{SourceID: 7}}
	cases := []struct {
		name   string
		rule   FilterRuleWithConditions
		target models.ItemWithSource
		match  bool
	}{
		{name: "global 匹配全部", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "global"}}, target: item, match: true},
		{name: "source 匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "source", SourceID: intPtr(7)}}, target: item, match: true},
		{name: "source 不匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "source", SourceID: intPtr(8)}}, target: item, match: false},
		{name: "source 缺 sourceId 不匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "source"}}, target: item, match: false},
		{name: "folder 匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "folder", FolderID: intPtr(3)}}, target: item, match: true},
		{name: "folder 不匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "folder", FolderID: intPtr(4)}}, target: item, match: false},
		{name: "item 无 folder 时 folder 不匹配", rule: FilterRuleWithConditions{FilterRule: models.FilterRule{Scope: "folder", FolderID: intPtr(3)}}, target: noFolderItem, match: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ruleMatchesScope(tc.rule, tc.target); got != tc.match {
				t.Fatalf("ruleMatchesScope() = %v, want %v", got, tc.match)
			}
		})
	}
}

func TestRuleMatchesConditions(t *testing.T) {
	desc := "文章摘要内容"
	author := "张三"
	item := models.ItemWithSource{
		Item: models.Item{Title: "Breaking News 2026", Link: "https://example.com/a", Desc: &desc, Author: &author},
	}
	noDescItem := models.ItemWithSource{Item: models.Item{Title: "t", Link: "l"}}
	cases := []struct {
		name       string
		conditions []FilterCondition
		target     models.ItemWithSource
		match      bool
	}{
		{name: "title contains 命中（大小写不敏感）", conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "breaking"}}, target: item, match: true},
		{name: "title contains 未命中", conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "旧闻"}}, target: item, match: false},
		{name: "desc notContains 命中", conditions: []FilterCondition{{Field: "desc", Operator: "notContains", Value: "无关词"}}, target: item, match: true},
		{name: "author equals 命中", conditions: []FilterCondition{{Field: "author", Operator: "equals", Value: "张三"}}, target: item, match: true},
		{name: "link notEquals 命中", conditions: []FilterCondition{{Field: "link", Operator: "notEquals", Value: "https://other.com"}}, target: item, match: true},
		{name: "多条件 AND 语义全命中", conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "news"}, {Field: "author", Operator: "equals", Value: "张三"}}, target: item, match: true},
		{name: "多条件 AND 语义部分命中", conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "news"}, {Field: "author", Operator: "equals", Value: "李四"}}, target: item, match: false},
		{name: "空条件列表不匹配", conditions: nil, target: item, match: false},
		{name: "空值条件不匹配", conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "  "}}, target: item, match: false},
		{name: "desc 为 nil 时 contains 不匹配", conditions: []FilterCondition{{Field: "desc", Operator: "contains", Value: "x"}}, target: noDescItem, match: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ruleMatchesConditions(tc.conditions, tc.target); got != tc.match {
				t.Fatalf("ruleMatchesConditions() = %v, want %v", got, tc.match)
			}
		})
	}
}

func TestFilterRuleCRUD(t *testing.T) {
	s := newTestService(t)

	created, err := s.CreateFilterRule(CreateFilterRuleRequest{
		Name: "过滤广告", Enabled: true, Priority: 5, Scope: "global", Action: "markRead",
		Conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "广告"}},
	})
	if err != nil {
		t.Fatalf("CreateFilterRule: %v", err)
	}
	if created.ID == 0 || created.Conditions[0].Value != "广告" {
		t.Fatalf("unexpected created rule: %+v", created)
	}

	rules, err := s.GetFilterRules()
	if err != nil {
		t.Fatalf("GetFilterRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "过滤广告" {
		t.Fatalf("unexpected rules: %+v", rules)
	}

	// 缓存路径：再次读取应命中内存缓存且结果一致
	cached, err := s.GetFilterRules()
	if err != nil {
		t.Fatalf("GetFilterRules (cached): %v", err)
	}
	if len(cached) != 1 {
		t.Fatalf("cached rules = %d, want 1", len(cached))
	}

	updated, err := s.UpdateFilterRule(created.ID, CreateFilterRuleRequest{
		Name: "改名", Enabled: false, Scope: "global", Action: "star",
		Conditions: []FilterCondition{{Field: "title", Operator: "equals", Value: "x"}},
	})
	if err != nil {
		t.Fatalf("UpdateFilterRule: %v", err)
	}
	if updated.Name != "改名" || updated.Enabled {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	if err := s.DeleteFilterRule(created.ID); err != nil {
		t.Fatalf("DeleteFilterRule: %v", err)
	}
	rules, err = s.GetFilterRules()
	if err != nil {
		t.Fatalf("GetFilterRules after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("rules after delete = %d, want 0", len(rules))
	}
}

func TestApplyFilterRules(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	source := models.Source{Name: "测试源", URL: "https://example.com/feed", ListRule: "rss", Active: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	desc := "促销内容"
	items := []models.Item{
		{SourceID: source.ID, Title: "正常文章", Link: "https://example.com/1"},
		{SourceID: source.ID, Title: "限时特惠广告", Link: "https://example.com/2", Desc: &desc},
		{SourceID: source.ID, Title: "值得收藏的深度文", Link: "https://example.com/3"},
	}
	for i := range items {
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("create item %d: %v", i, err)
		}
	}

	// 规则 1：title 含"广告" → markRead；规则 2：title 含"收藏" → star
	if _, err := s.CreateFilterRule(CreateFilterRuleRequest{
		Name: "广告已读", Scope: "global", Action: "markRead",
		Conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "广告"}},
	}); err != nil {
		t.Fatalf("create rule 1: %v", err)
	}
	if _, err := s.CreateFilterRule(CreateFilterRuleRequest{
		Name: "深度加星", Scope: "global", Action: "star",
		Conditions: []FilterCondition{{Field: "title", Operator: "contains", Value: "收藏"}},
	}); err != nil {
		t.Fatalf("create rule 2: %v", err)
	}

	for _, item := range items {
		if err := s.ApplyFilterRules(item.ID); err != nil {
			t.Fatalf("ApplyFilterRules(%d): %v", item.ID, err)
		}
	}

	var ad models.Item
	if err := db.First(&ad, items[1].ID).Error; err != nil {
		t.Fatalf("load ad item: %v", err)
	}
	if !ad.IsRead {
		t.Error("广告文章应被 markRead 规则标记为已读")
	}
	var deep models.Item
	if err := db.First(&deep, items[2].ID).Error; err != nil {
		t.Fatalf("load deep item: %v", err)
	}
	if !deep.IsStarred || deep.IsRead {
		t.Errorf("深度文应仅被加星（isStarred=%v, isRead=%v）", deep.IsStarred, deep.IsRead)
	}
	var normal models.Item
	if err := db.First(&normal, items[0].ID).Error; err != nil {
		t.Fatalf("load normal item: %v", err)
	}
	if normal.IsRead || normal.IsStarred {
		t.Error("正常文章不应被任何规则命中")
	}
}
