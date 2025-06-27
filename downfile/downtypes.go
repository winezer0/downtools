package downfile

// ModuleItem 下载项目结构
type ModuleItem struct {
	Module       string   `yaml:"module"`
	FileName     string   `yaml:"filename"`
	DownloadURLs []string `yaml:"download-urls"`
	KeepUpdated  bool     `yaml:"keep-updated"`
}

// Config 配置文件结构
type Config map[string][]ModuleItem

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
