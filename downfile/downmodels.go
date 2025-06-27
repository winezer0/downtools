package downfile

import (
	"context"
	"sync/atomic"
	"time"
)

// ModuleItem 下载项目结构
type ModuleItem struct {
	Module       string   `yaml:"module"`
	FileName     string   `yaml:"filename"`
	DownloadURLs []string `yaml:"download-urls"`
	KeepUpdated  bool     `yaml:"keep-updated"`
}

// Config 配置文件结构
type Config map[string][]ModuleItem

// ProgressTracker 下载进度跟踪器
type ProgressTracker struct {
	BytesCount   *atomic.Int64      // 已下载字节数
	FileSize     int64              // 文件总大小
	StartTime    time.Time          // 下载开始时间
	LastUpdate   time.Time          // 上次更新时间
	LastSize     int64              // 上次记录的大小
	Speed        float64            // 当前下载速度
	Done         chan struct{}      // 完成信号
	Name         string             // 下载的文件名
	Cancel       context.CancelFunc // 用于取消下载的函数
	Ctx          context.Context    // 下载上下文
	CancelReason atomic.Value       // 取消原因
}

// NewProgressTracker 创建新的进度跟踪器
func NewProgressTracker(fileSize int64, name string) *ProgressTracker {
	now := time.Now()
	var counter atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())

	tracker := &ProgressTracker{
		BytesCount: &counter,
		FileSize:   fileSize,
		StartTime:  now,
		LastUpdate: now,
		LastSize:   0,
		Speed:      0,
		Done:       make(chan struct{}),
		Name:       name,
		Ctx:        ctx,
		Cancel:     cancel,
	}

	// 初始化取消原因为空字符串
	tracker.CancelReason.Store("")

	return tracker
}

// DownloadError 自定义错误类型
type DownloadError struct {
	StatusCode int
	Message    string
	Type       string
}

func (e DownloadError) Error() string {
	return e.Message
}

// 常量定义
const (
	// MinValidSpeed 最小有效下载速度 (bytes/second)，低于此值视为停滞
	MinValidSpeed = 10.0
	// MinRequiredSpeed 最小要求下载速度 (bytes/second)，低于此值判定为网络问题
	MinRequiredSpeed = 1024.0 // 1KB/s
	// SpeedCheckInterval 下载速度检测间隔（秒）
	SpeedCheckInterval = 5
	// ProgressUpdateInterval 下载进度更新间隔（毫秒）
	ProgressUpdateInterval = 500
	// DownloadBufferSize 下载缓冲区大小
	DownloadBufferSize = 32 * 1024 // 32KB
	// CacheFileName 缓存文件名
	CacheFileName = ".download_cache.json"
	// CacheExpireHours 缓存过期时间（小时）
	CacheExpireHours = 1
)

// 错误类型常量
const (
	// ErrResourceNotFound 资源不存在错误（404）
	ErrResourceNotFound = "RESOURCE_NOT_FOUND"
	// ErrLowSpeed 下载速度过低错误
	ErrLowSpeed = "DOWNLOAD_SPEED_TOO_LOW"
)
