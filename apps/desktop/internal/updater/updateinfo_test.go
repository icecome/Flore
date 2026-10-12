package updater

import "testing"

func TestBuildUpdateInfo(t *testing.T) {
	plat := currentPlatform()
	mkManifest := func(latest, minSupported string, platform string) *Manifest {
		return &Manifest{
			Latest:       latest,
			MinSupported: minSupported,
			Notes:        "notes",
			Assets: []Asset{{
				Platform: platform,
				FileName: "app.zip",
				Size:     128,
				SHA256:   "deadbeef",
				URLs:     []string{"https://example.com/app.zip"},
			}},
		}
	}

	t.Run("无更新时返回 nil", func(t *testing.T) {
		info, err := buildUpdateInfo(mkManifest("0.1.0", "", plat), "0.2.0")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if info != nil {
			t.Fatalf("info=%v want nil", info)
		}
	})

	t.Run("有更新且非强制", func(t *testing.T) {
		info, err := buildUpdateInfo(mkManifest("0.3.0", "0.1.0", plat), "0.2.0")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if info == nil {
			t.Fatal("info is nil")
		}
		if info.Forced {
			t.Fatal("Forced=true want false")
		}
		if info.LatestVersion != "0.3.0" || info.Size != 128 {
			t.Fatalf("info=%+v", info)
		}
	})

	t.Run("当前版本低于 minSupported 时强制更新", func(t *testing.T) {
		info, err := buildUpdateInfo(mkManifest("0.3.0", "0.2.5", plat), "0.2.0")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if info == nil || !info.Forced {
			t.Fatalf("info=%+v want Forced=true", info)
		}
	})

	t.Run("minSupported 为空不强制", func(t *testing.T) {
		info, err := buildUpdateInfo(mkManifest("0.3.0", "", plat), "0.2.0")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if info == nil || info.Forced {
			t.Fatalf("info=%+v want Forced=false", info)
		}
	})

	t.Run("manifest 缺 latest 返回错误", func(t *testing.T) {
		if _, err := buildUpdateInfo(mkManifest("", "", plat), "0.2.0"); err == nil {
			t.Fatal("want error")
		}
	})

	t.Run("平台不匹配返回错误", func(t *testing.T) {
		if _, err := buildUpdateInfo(mkManifest("0.3.0", "", "nonexist/arch"), "0.2.0"); err == nil {
			t.Fatal("want error")
		}
	})
}
