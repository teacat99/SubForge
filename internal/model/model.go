package model

import "time"

type Subscription struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	AutoRefresh     bool       `json:"auto_refresh" gorm:"default:true"`
	IntervalSec     int        `json:"interval_sec" gorm:"default:86400"`
	LastFetched     *time.Time `json:"last_fetched"`
	NodeCount       int        `gorm:"-" json:"node_count"`
	AppendSubName   bool       `json:"append_sub_name" gorm:"default:false"`
	AppendTraffic   bool       `json:"append_traffic" gorm:"default:false"`
	TrafficUsed     int64      `json:"traffic_used"`
	TrafficTotal    int64      `json:"traffic_total"`
	SubExpiry       string     `json:"sub_expiry"`
	ExcludeKeywords string     `json:"exclude_keywords" gorm:"default:'剩余流量'"`
	SortOrder       int        `json:"sort_order" gorm:"default:0"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Node struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	SubscriptionID uint   `json:"subscription_id" gorm:"index"`
	Name           string `json:"name"`
	Alias          string `json:"alias" gorm:"default:''"`
	Type           string `json:"type"`
	Server         string `json:"server"`
	Port           int    `json:"port"`
	RawConfig      string `gorm:"type:text" json:"-"`
	Enabled        bool   `gorm:"default:true" json:"enabled"`
}

type ServiceGroup struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `json:"name"`
	Icon         string     `json:"icon"`
	RuleType     string     `json:"rule_type" gorm:"default:remote"`
	RuleURL      string     `json:"rule_url"`
	DefaultProxy string     `json:"default_proxy" gorm:"default:自动选择"`
	DirectRule   bool       `json:"direct_rule" gorm:"default:false"`
	SortOrder    int        `json:"sort_order"`
	RuleCount    int        `json:"rule_count"`
	Enabled      bool       `json:"enabled" gorm:"default:true"`
	AutoRefresh  bool       `json:"auto_refresh" gorm:"default:true"`
	IntervalSec  int        `json:"interval_sec" gorm:"default:86400"`
	LastFetched  *time.Time `json:"last_fetched"`
	CachedRules  string     `gorm:"type:text" json:"-"`
	Builtin      bool       `json:"builtin"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ProfileService struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	ProfileID      uint   `gorm:"uniqueIndex:idx_ps" json:"profile_id"`
	ServiceGroupID uint   `gorm:"uniqueIndex:idx_ps" json:"service_group_id"`
	Enabled        bool   `gorm:"default:true" json:"enabled"`
	SortOrder      int    `json:"sort_order"`
	DefaultProxy   string `json:"default_proxy" gorm:"default:''"`
}

type ProfileNode struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	ProfileID uint `gorm:"uniqueIndex:idx_pn" json:"profile_id"`
	NodeID    uint `gorm:"uniqueIndex:idx_pn" json:"node_id"`
	Enabled   bool `gorm:"default:true" json:"enabled"`
}

type UserProfile struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `json:"name"`
	Token         string     `gorm:"uniqueIndex" json:"token"`
	Enabled       bool       `gorm:"default:true" json:"enabled"`
	ExpiresAt     *time.Time `json:"expires_at"`
	CatchAll      bool       `gorm:"default:true" json:"catch_all"`
	GeoipCN       bool       `json:"geoip_cn" gorm:"default:true"`
	DnsPresetID   uint       `json:"dns_preset_id"`
	HostsPresetID uint       `json:"hosts_preset_id"`
	SortOrder     int        `json:"sort_order" gorm:"default:0"`
	CreatedAt     time.Time  `json:"created_at"`
}

type DnsPreset struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Config    string    `gorm:"type:text" json:"config"`
	Builtin   bool      `json:"builtin"`
	CreatedAt time.Time `json:"created_at"`
}

type HostsPreset struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"`
	Config    string    `gorm:"type:text" json:"config"`
	Builtin   bool      `json:"builtin"`
	CreatedAt time.Time `json:"created_at"`
}

type AdminUser struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Username string `gorm:"uniqueIndex" json:"username"`
	Password string `json:"-"`
}

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type PublishedProfile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Token     string    `gorm:"uniqueIndex" json:"token"`
	Config    string    `gorm:"type:text" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}
