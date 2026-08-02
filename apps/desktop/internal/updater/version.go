package updater

import (
	"strconv"
	"strings"
)

// parseVersion 解析 "X.Y.Z[-suffix]" 形式（允许前导 v）。
// 返回 major/minor/patch 与可选 suffix；解析失败 ok=false。
func parseVersion(v string) (major, minor, patch int, suffix string, ok bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return 0, 0, 0, "", false
	}
	core := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core = v[:i]
		suffix = v[i+1:]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, "", false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, "", false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, "", false
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, "", false
	}
	return major, minor, patch, suffix, true
}

// CompareVersion 比较两个版本号。
// 返回值：a 比 b 新返回 1，相等返回 0，a 比 b 旧返回 -1。
// 针对本项目 "X.Y.Z-YYYYMMDD" 日期后缀约定：核心版本相同时，带日期后缀者更新。
func CompareVersion(a, b string) int {
	ma, mi, pa, sa, oka := parseVersion(a)
	mb, mi2, pb, sb, okb := parseVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	if ma != mb {
		if ma > mb {
			return 1
		}
		return -1
	}
	if mi != mi2 {
		if mi > mi2 {
			return 1
		}
		return -1
	}
	if pa != pb {
		if pa > pb {
			return 1
		}
		return -1
	}
	return compareSuffix(sa, sb)
}

// compareSuffix 核心版本相同后比较后缀。
// 本项目后缀为构建日期（YYYYMMDD），数值越大越新；无后缀视为正式版但本项目约定
// 日期构建更新，故无后缀 < 有日期后缀。
func compareSuffix(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" && b != "" {
		return -1
	}
	if b == "" && a != "" {
		return 1
	}
	na, ea := strconv.Atoi(a)
	nb, eb := strconv.Atoi(b)
	if ea == nil && eb == nil {
		if na > nb {
			return 1
		}
		if na < nb {
			return -1
		}
		return 0
	}
	return strings.Compare(a, b)
}
