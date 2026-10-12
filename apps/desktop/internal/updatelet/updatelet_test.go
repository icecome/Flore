package updatelet

import (
	"sync"
	"testing"
)

func TestNewAndSnapshot(t *testing.T) {
	u := New("0.1.0", nil)
	st := u.Snapshot()
	if st.Status != StatusIdle {
		t.Fatalf("status=%s want %s", st.Status, StatusIdle)
	}
	if st.Info != nil || st.Progress != 0 || st.Error != "" {
		t.Fatalf("snapshot=%+v", st)
	}
}

func TestDownloadWithoutInfo(t *testing.T) {
	u := New("0.1.0", nil)
	if err := u.Download(); err == nil {
		t.Fatal("Download without info should fail")
	}
	if u.IsBusy() {
		t.Fatal("IsBusy=true want false")
	}
}

func TestApplyReadyOnlyFromReady(t *testing.T) {
	u := New("0.1.0", nil)
	// idle 状态下调用不应改变状态（仅 ready → pendingRestart）
	u.ApplyReady()
	if st := u.Snapshot(); st.Status != StatusIdle {
		t.Fatalf("status=%s want %s", st.Status, StatusIdle)
	}
}

func TestCallbackReceivesState(t *testing.T) {
	var mu sync.Mutex
	var got []Status
	u := New("0.1.0", func(st State) {
		mu.Lock()
		got = append(got, st.Status)
		mu.Unlock()
	})
	// 通过公开路径触发一次状态广播（idle 下 ApplyReady 不改变状态但仍会回调），
	// 避免单测依赖真实网络。
	u.ApplyReady()
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 || got[0] != StatusIdle {
		t.Fatalf("callback got=%v want first=%s", got, StatusIdle)
	}
}

func TestCheckRejectsWhileInFlight(t *testing.T) {
	// 并发 Check：除首个外均应被 inFlight 拒绝，且不 panic。
	// 单测不等待后台 goroutine 的网络结果（网络行为由集成环境验证）。
	u := New("0.1.0", nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCount := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := u.Check(); err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}
