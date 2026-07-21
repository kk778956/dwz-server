// 2026071610100000_5108_backfill_short_links_url_hash 为 short_links 历史行
// 回填 url_hash（original_url 的 SHA-256 hex），供创建短链时按 URL 查重使用。
//
// 设计与 0015_backfill_click_statistics_region 一致：
//   - goose 跟踪、后台协程异步执行，不阻塞 HTTP 启动；多副本同时启动时
//     允许任务并行，条件更新保证重复执行能够幂等收敛。
//   - 首次安装路径 helper.GetDatabase() 为 nil 时表必空，直接 return nil 记账。
//   - 幂等：只处理 url_hash 为空的行，按 id 键集分页扫描，重跑收敛。
//   - 失败恢复：运维手工 DELETE FROM goose_db_version 对应版本后重启触发重跑。
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"cnb.cool/mliev/dwz/dwz-server/v2/pkg/helper"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

const urlHashBackfillPageSize = 500

func init() {
	goose.AddNamedMigrationNoTxContext(
		"2026071610100000_5108_backfill_short_links_url_hash.go",
		upBackfillShortLinksURLHash,
		downBackfillShortLinksURLHash,
	)
}

func upBackfillShortLinksURLHash(_ context.Context, _ *sql.DB) error {
	h := helper.GetHelper()
	logger := h.GetLogger()

	// 首次安装路径：容器 DB 未注入，short_links 必然是刚 CREATE 的空表，无需回填
	if h.GetDatabase() == nil {
		logger.Info("[migration url_hash] 安装阶段：容器 DB 未注入，表必空，跳过回填")
		return nil
	}

	logger.Info("[migration url_hash] 已受理，后台协程执行回填（不阻塞启动）")
	go runURLHashBackfillAsync()
	return nil
}

func runURLHashBackfillAsync() {
	h := helper.GetHelper()
	logger := h.GetLogger()
	db := h.GetDatabase()
	if db == nil {
		logger.Error("[migration url_hash] 协程启动时 DB 不可用，放弃")
		return
	}

	logger.Info("[migration url_hash] 后台回填开始")
	startedAt := time.Now()

	type row struct {
		ID          uint64 `gorm:"column:id"`
		OriginalURL string `gorm:"column:original_url"`
	}

	var totalAffected int64
	lastID := uint64(0)
	for {
		var rows []row
		if err := db.
			Table("short_links").
			Select("id, original_url").
			Where("id > ?", lastID).
			Where("url_hash = '' OR url_hash IS NULL").
			Order("id ASC").
			Limit(urlHashBackfillPageSize).
			Find(&rows).Error; err != nil {
			logger.Error(fmt.Sprintf("[migration url_hash] 扫描失败(lastID=%d): %s", lastID, err.Error()))
			return
		}
		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			sum := sha256.Sum256([]byte(r.OriginalURL))
			res := db.
				Table("short_links").
				Where("id = ?", r.ID).
				Where("original_url = ?", r.OriginalURL).
				Where("url_hash = '' OR url_hash IS NULL").
				Update("url_hash", hex.EncodeToString(sum[:]))
			if res.Error != nil {
				logger.Error(fmt.Sprintf("[migration url_hash] 更新失败(id=%d): %s", r.ID, res.Error.Error()))
				return
			}
			totalAffected += res.RowsAffected
		}
		lastID = rows[len(rows)-1].ID
	}

	logger.Info(fmt.Sprintf(
		"[migration url_hash] 回填完成: rows_affected=%d, 耗时 %s",
		totalAffected, time.Since(startedAt),
	))
}

// downBackfillShortLinksURLHash 把 url_hash 置空，满足 goose 回滚契约。
func downBackfillShortLinksURLHash(_ context.Context, _ *sql.DB) error {
	h := helper.GetHelper()
	db := h.GetDatabase()
	if db == nil {
		return errors.New("[migration url_hash] 数据库不可用")
	}
	return db.
		Session(&gorm.Session{AllowGlobalUpdate: true}).
		Table("short_links").
		Update("url_hash", "").Error
}
