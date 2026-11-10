package dao

import "time"

type BenchmarkAccount struct {
	ID                uint64     `gorm:"primaryKey" json:"id"`
	Platform          string     `gorm:"size:32" json:"platform"`
	PlatformAccountID string     `gorm:"size:191;uniqueIndex" json:"platform_account_id"`
	DouyinNumber      string     `gorm:"size:64" json:"douyin_number"`
	HomepageURL       string     `gorm:"size:1024" json:"homepage_url"`
	DisplayName       string     `gorm:"size:255" json:"display_name"`
	Enabled           bool       `json:"enabled"`
	IntervalMinutes   int        `json:"interval_minutes"`
	Threshold         int64      `json:"threshold"`
	MaxItems          int        `json:"max_items"`
	NotifyNew         bool       `json:"notify_new"`
	NotifyThreshold   bool       `json:"notify_threshold"`
	HistoryCursor     string     `gorm:"type:text" json:"-"`
	HistoryEnd        *time.Time `json:"-"`
	HistoryComplete   bool       `json:"history_complete"`
	BaselineAt        *time.Time `json:"baseline_at"`
	LastCheckedAt     *time.Time `json:"last_checked_at"`
	NextCheckAt       time.Time  `gorm:"index" json:"next_check_at"`
	LastStatus        string     `json:"last_status"`
	LastError         string     `json:"last_error"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Video struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	AccountID      uint64     `gorm:"uniqueIndex:account_video;index" json:"account_id"`
	PlatformID     string     `gorm:"size:191;uniqueIndex:account_video" json:"platform_id"`
	URL            string     `json:"url"`
	Title          string     `json:"title"`
	CoverURL       string     `json:"cover_url"`
	Hearts         *int64     `json:"hearts"`
	Nonce          string     `json:"nonce"`
	Likes          int64      `json:"likes"`
	LikesExact     bool       `json:"likes_exact"`
	LikesDisplay   string     `json:"likes_display"`
	Comments       int64      `json:"comments"`
	Favorites      int64      `json:"favorites"`
	Shares         int64      `json:"shares"`
	PublishedAt    *time.Time `json:"published_at"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	Source         string     `json:"source"`
	FirstOver10kAt *time.Time `gorm:"column:first_over_10k_at;index" json:"first_over_10k_at"`
	AlreadyOver10k bool       `gorm:"column:already_over_10k" json:"already_over_10k"`
}

type VideoTranscript struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	VideoID    uint64     `gorm:"uniqueIndex" json:"video_id"`
	AccountID  uint64     `gorm:"index" json:"account_id"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Caption    string     `gorm:"type:text" json:"caption"`
	Text       string     `gorm:"type:longtext" json:"text"`
	SRT        string     `gorm:"type:longtext" json:"srt"`
	Engine     string     `json:"engine"`
	Duration   float64    `json:"duration"`
	Error      string     `json:"error"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type Notification struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	EventKey  string    `gorm:"size:191;uniqueIndex" json:"-"`
	AccountID uint64    `gorm:"index" json:"account_id"`
	VideoID   uint64    `json:"video_id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

type CrawlRun struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	AccountID  uint64     `gorm:"index" json:"account_id"`
	Kind       string     `json:"kind"`
	Status     string     `gorm:"index" json:"status"`
	Scanned    int        `json:"scanned"`
	NewCount   int        `json:"new_count"`
	AlertCount int        `json:"alert_count"`
	Complete   bool       `json:"complete"`
	StopReason string     `json:"stop_reason"`
	Message    string     `json:"message"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}
