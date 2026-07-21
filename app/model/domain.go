package model

import (
	"time"

	"gorm.io/gorm"
)

// Domain 域名配置模型
type Domain struct {
	ID                   uint64         `gorm:"primaryKey" json:"id"` // 自增主键
	WorkspaceID          uint64         `gorm:"not null;default:1;index" json:"workspace_id"`
	Protocol             string         `gorm:"size:10;default:'https';not null" json:"protocol"`     // 协议头 http或https
	Domain               string         `gorm:"uniqueIndex;size:100;not null" json:"domain"`          // 域名  例如 n3.ink
	SiteName             string         `gorm:"size:100;default:''" json:"site_name"`                 // 网站名称
	ICPNumber            string         `gorm:"size:50;default:''" json:"icp_number"`                 // ICP备案号码
	PoliceNumber         string         `gorm:"size:50;default:''" json:"police_number"`              // 公安备案号码
	PassQueryParams      bool           `gorm:"default:false" json:"pass_query_params"`               // 是否透传GET参数
	Description          string         `gorm:"type:text" json:"description"`                         // 描述
	IsActive             bool           `gorm:"default:true" json:"is_active"`                        // 是否激活
	RandomSuffixLength   *int           `gorm:"default:2" json:"random_suffix_length"`                // 随机后缀位数 (0-10)，使用指针以区分0和未设置
	EnableChecksum       *bool          `gorm:"default:true" json:"enable_checksum"`                  // 是否启用校验位，使用指针以区分false和未设置
	EnableXorObfuscation *bool          `gorm:"default:false" json:"enable_xor_obfuscation"`          // 是否启用XOR混淆，使用指针以区分false和未设置
	EnableAntiRed        *bool          `gorm:"default:false" json:"enable_anti_red"`                 // 是否启用微信/QQ防红
	XorSecret            *uint64        `json:"xor_secret"`                                           // XOR密钥，创建时由服务层随机生成
	XorRot               *int           `json:"xor_rot"`                                              // 旋转位数，创建时由服务层随机生成
	DefaultStartNumber   *uint64        `gorm:"default:0" json:"default_start_number"`                // 默认开始数字，使用指针以区分0和未设置
	DuplicatePolicy      *string        `gorm:"size:20;default:'by_request'" json:"duplicate_policy"` // 重复URL策略 deny(不允许重复)/allow(允许重复)/by_request(创建时自行决定)，空或未知值按by_request处理
	CreatedAt            time.Time      `json:"created_at"`                                           // 创建时间
	UpdatedAt            time.Time      `json:"updated_at"`                                           // 更新时间
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`                                       // 删除时间
}

func (Domain) TableName() string {
	return "domains"
}

// 重复URL策略取值
const (
	DuplicatePolicyDeny      = "deny"       // 不允许重复，创建时强制查重并返回既有短链
	DuplicatePolicyAllow     = "allow"      // 允许重复，永远新建，忽略请求参数
	DuplicatePolicyByRequest = "by_request" // 由创建请求的 find_if_exists 参数决定（默认）
)

// GetDuplicatePolicy 获取归一化后的重复URL策略，空或未知值按 by_request 处理
func (d *Domain) GetDuplicatePolicy() string {
	if d.DuplicatePolicy != nil {
		switch *d.DuplicatePolicy {
		case DuplicatePolicyDeny, DuplicatePolicyAllow:
			return *d.DuplicatePolicy
		}
	}
	return DuplicatePolicyByRequest
}
