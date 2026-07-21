package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/mliev/dwz/dwz-server/v2/app/dto"
	"cnb.cool/mliev/dwz/dwz-server/v2/app/model"
	pkghelper "cnb.cool/mliev/dwz/dwz-server/v2/pkg/helper"
	"cnb.cool/mliev/dwz/dwz-server/v2/pkg/interfaces"
)

// dedupFakeIDGenerator 顺序发号的测试用生成器，替代测试环境中不存在的 Redis/本地发号器
type dedupFakeIDGenerator struct{ counter uint64 }

func (g *dedupFakeIDGenerator) InitializeDomainCounter(uint64, uint64) error { return nil }
func (g *dedupFakeIDGenerator) GenerateID(uint64, context.Context) (uint64, error) {
	return atomic.AddUint64(&g.counter, 1), nil
}
func (g *dedupFakeIDGenerator) GenerateShortCode(domainID uint64, ctx context.Context) (string, *uint64, error) {
	return g.GenerateShortCodeWithConfig(domainID, ctx, interfaces.ShortCodeConfig{})
}
func (g *dedupFakeIDGenerator) GenerateShortCodeWithConfig(uint64, context.Context, interfaces.ShortCodeConfig) (string, *uint64, error) {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("t%d", n), &n, nil
}
func (g *dedupFakeIDGenerator) ResetDomainCounter(uint64, uint64) error { return nil }

var dedupIDGeneratorOnce sync.Once

func newDedupTestService(t *testing.T, policy *string) (*shortLinkRegressionHelper, *ShortLinkService) {
	t.Helper()
	helper := newShortLinkRegressionHelper(t)
	dedupIDGeneratorOnce.Do(func() {
		pkghelper.SetIdGenerator(&dedupFakeIDGenerator{})
	})
	if err := helper.GetDatabase().Create(&model.Domain{
		WorkspaceID:     1,
		Protocol:        "https",
		Domain:          "dwz.do",
		IsActive:        true,
		DuplicatePolicy: policy,
	}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return helper, NewShortLinkService(helper, context.Background())
}

func strPtr(value string) *string { return &value }

func dedupCreate(t *testing.T, svc *ShortLinkService, req *dto.CreateShortLinkRequest) *dto.ShortLinkResponse {
	t.Helper()
	if req.Domain == "" {
		req.Domain = "dwz.do"
	}
	resp, err := svc.CreateShortLinkInWorkspace(req, "203.0.113.10", 1, 7)
	if err != nil {
		t.Fatalf("create short link: %v", err)
	}
	return resp
}

func TestDedupByRequestPolicy(t *testing.T) {
	helper, svc := newDedupTestService(t, nil) // 域名未设置策略 → 默认 by_request

	first := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if first.IsExisting {
		t.Fatalf("first create should not be existing: %+v", first)
	}

	// url_hash 无条件写入
	var stored model.ShortLink
	if err := helper.GetDatabase().First(&stored, first.ID).Error; err != nil {
		t.Fatalf("load stored link: %v", err)
	}
	if len(stored.URLHash) != 64 {
		t.Fatalf("url_hash not written on create: %q", stored.URLHash)
	}

	second := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if !second.IsExisting || second.ID != first.ID || second.ShortCode != first.ShortCode {
		t.Fatalf("expected reuse of first link, got: %+v", second)
	}

	// 未传 find_if_exists → 保持现状，允许重复
	third := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/page",
	})
	if third.IsExisting || third.ID == first.ID {
		t.Fatalf("create without flag should not dedupe: %+v", third)
	}

	// 不同URL不复用
	other := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/other",
		FindIfExists: true,
	})
	if other.IsExisting {
		t.Fatalf("different URL should not dedupe: %+v", other)
	}
}

func TestDedupDenyPolicy(t *testing.T) {
	_, svc := newDedupTestService(t, strPtr(model.DuplicatePolicyDeny))

	first := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/page",
	})
	// deny 下即使不传参数也强制查重
	second := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/page",
	})
	if !second.IsExisting || second.ID != first.ID {
		t.Fatalf("deny policy should force reuse: %+v", second)
	}

	// 带自定义短码豁免查重，照常新建
	custom := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/page",
		CustomCode:  "vip",
	})
	if custom.IsExisting || custom.ID == first.ID || custom.ShortCode != "vip" {
		t.Fatalf("custom code should be exempt from dedupe: %+v", custom)
	}

	// 自定义短码的链接不作为复用来源：再次普通创建仍复用 first 而不是 custom
	fourth := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/page",
	})
	if !fourth.IsExisting || fourth.ID != first.ID {
		t.Fatalf("dedupe should reuse non-custom link only: %+v", fourth)
	}
}

func TestDedupAllowPolicy(t *testing.T) {
	_, svc := newDedupTestService(t, strPtr(model.DuplicatePolicyAllow))

	first := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	// allow 下请求参数被忽略，永远新建
	second := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if second.IsExisting || second.ID == first.ID {
		t.Fatalf("allow policy must ignore find_if_exists: %+v", second)
	}
}

func TestDedupDifferentUTMNotReused(t *testing.T) {
	_, svc := newDedupTestService(t, nil)

	first := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		UTMSource:    "newsletter",
		FindIfExists: true,
	})
	// UTM 不同 → 合并后的目标URL不同 → 不复用
	second := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		UTMSource:    "weibo",
		FindIfExists: true,
	})
	if second.IsExisting || second.ID == first.ID {
		t.Fatalf("different UTM should not dedupe: %+v", second)
	}
	// 相同 UTM → 复用
	third := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		UTMSource:    "newsletter",
		FindIfExists: true,
	})
	if !third.IsExisting || third.ID != first.ID {
		t.Fatalf("same UTM should dedupe: %+v", third)
	}
}

func TestDedupSkipsExpiredAndInactive(t *testing.T) {
	helper, svc := newDedupTestService(t, nil)
	db := helper.GetDatabase()

	first := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	// 已过期的链接不复用
	expired := time.Now().Add(-time.Hour)
	if err := db.Model(&model.ShortLink{}).Where("id = ?", first.ID).
		Update("expire_at", expired).Error; err != nil {
		t.Fatalf("expire first link: %v", err)
	}
	second := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if second.IsExisting {
		t.Fatalf("expired link must not be reused: %+v", second)
	}

	// 已停用的链接不复用
	if err := db.Model(&model.ShortLink{}).Where("id = ?", second.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate second link: %v", err)
	}
	third := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if third.IsExisting {
		t.Fatalf("inactive link must not be reused: %+v", third)
	}

	// 软删除的链接不复用
	if err := db.Delete(&model.ShortLink{}, third.ID).Error; err != nil {
		t.Fatalf("soft delete third link: %v", err)
	}
	fourth := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/page",
		FindIfExists: true,
	})
	if fourth.IsExisting {
		t.Fatalf("soft-deleted link must not be reused: %+v", fourth)
	}
}

func TestDedupBatchCreatePassesFlag(t *testing.T) {
	helper, svc := newDedupTestService(t, nil)

	resp, err := svc.BatchCreateShortLinksInWorkspace(&dto.BatchCreateShortLinkRequest{
		URLs:         []string{"https://example.com/a", "https://example.com/a"},
		Domain:       "dwz.do",
		FindIfExists: true,
	}, "203.0.113.10", helper, 1, 7)
	if err != nil {
		t.Fatalf("batch create: %v", err)
	}
	if len(resp.Failed) != 0 || len(resp.Success) != 2 {
		t.Fatalf("unexpected batch result: %+v", resp)
	}
	if resp.Success[0].IsExisting {
		t.Fatalf("first batch item should be new: %+v", resp.Success[0])
	}
	if !resp.Success[1].IsExisting || resp.Success[1].ID != resp.Success[0].ID {
		t.Fatalf("second batch item should reuse first: %+v", resp.Success[1])
	}
}

func TestDedupUpdateRefreshesURLHash(t *testing.T) {
	helper, svc := newDedupTestService(t, nil)

	created := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL: "https://example.com/before",
	})
	updated, err := svc.UpdateShortLinkInWorkspace(created.ID, &dto.UpdateShortLinkRequest{
		OriginalURL: "https://example.com/after",
	}, 1, 7)
	if err != nil {
		t.Fatalf("update short link: %v", err)
	}
	if updated.OriginalURL != "https://example.com/after" {
		t.Fatalf("unexpected updated URL: %q", updated.OriginalURL)
	}

	var stored model.ShortLink
	if err := helper.GetDatabase().First(&stored, created.ID).Error; err != nil {
		t.Fatalf("load updated link: %v", err)
	}
	if stored.URLHash != hashOriginalURL(stored.OriginalURL) {
		t.Fatalf("url_hash not refreshed after update: url=%q hash=%q", stored.OriginalURL, stored.URLHash)
	}

	reused := dedupCreate(t, svc, &dto.CreateShortLinkRequest{
		OriginalURL:  "https://example.com/after",
		FindIfExists: true,
	})
	if !reused.IsExisting || reused.ID != created.ID {
		t.Fatalf("updated link should be reused by its new URL: %+v", reused)
	}
}
