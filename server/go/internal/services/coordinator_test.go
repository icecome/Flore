package services

import "testing"

// TestCoordinatorNewItemsAccumulate 验证本轮新增数的累加与完成时冻结逻辑（P1-6）。
// 这是根治「通知计数用全库未读差值近似」的数据通路核心：worker 在每个源完成后
// 累加 newItemsThisRound，本轮全部空闲时冻结到 lastRoundNewItems 并清零进行中计数。
func TestCoordinatorNewItemsAccumulate(t *testing.T) {
	c := &FetchCoordinator{}

	// 抓取进行中：累加但 LastRoundNewItems 尚未冻结（仍为 0）
	c.newItemsThisRound.Add(3)
	c.newItemsThisRound.Add(2)
	if got := c.LastRoundNewItems(); got != 0 {
		t.Fatalf("expected LastRoundNewItems=0 before freeze, got %d", got)
	}

	// 本轮完成：冻结当前累计并清零进行中计数
	c.lastRoundNewItems.Store(c.newItemsThisRound.Load())
	c.newItemsThisRound.Store(0)

	if got := c.LastRoundNewItems(); got != 5 {
		t.Fatalf("expected LastRoundNewItems=5 after freeze, got %d", got)
	}
	if got := c.newItemsThisRound.Load(); got != 0 {
		t.Fatalf("expected newItemsThisRound=0 after reset, got %d", got)
	}

	// 下一轮开始：再次累加不影响已冻结值，直到再次完成
	c.newItemsThisRound.Add(1)
	if got := c.LastRoundNewItems(); got != 5 {
		t.Fatalf("expected LastRoundNewItems unchanged=5 during next round, got %d", got)
	}
	if got := c.newItemsThisRound.Load(); got != 1 {
		t.Fatalf("expected newItemsThisRound=1 during next round, got %d", got)
	}
}
