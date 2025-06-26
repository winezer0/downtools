package downutils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 常量定义
const (
	// 最小有效下载速度 (bytes/second)，低于此值视为停滞
	MinValidSpeed = 10.0
	// 下载停滞检测间隔（秒）
	StallCheckInterval = 10
	// 下载进度更新间隔（毫秒）
	ProgressUpdateInterval = 500
	// 下载缓冲区大小
	DownloadBufferSize = 32 * 1024 // 32KB
	// 缓存文件名
	CacheFileName = ".download_cache.json"
	// 缓存过期时间（小时）
	CacheExpireHours = 1
)

// 下载缓存结构
type DownloadCache struct {
	Files map[string]time.Time `json:"files"` // 文件路径 -> 最后下载时间
}

// 检查文件是否存在
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// 转换GitHub URL为原始内容URL
func ConvertGitHubURL(url string) string {
	// 只转换blob URL，不转换releases下载链接
	if strings.Contains(url, "/releases/") {
		return url
	}
	return strings.Replace(strings.Replace(url, "github.com", "raw.githubusercontent.com", 1), "/blob/", "/", 1)
}

// 格式化时间为易读格式
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	if d.Hours() >= 1 {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%d小时%d分%d秒", h, m, s)
	} else if d.Minutes() >= 1 {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%d分%d秒", m, s)
	}
	return fmt.Sprintf("%d秒", int(d.Seconds()))
}

// 格式化大小为易读格式
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// 获取缓存文件路径
func GetCacheFilePath() string {
	// 缓存文件保存在用户主目录下
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取主目录，则使用当前目录
		return CacheFileName
	}
	return filepath.Join(homeDir, CacheFileName)
}

// 加载下载缓存
func LoadDownloadCache() *DownloadCache {
	cacheFilePath := GetCacheFilePath()
	cache := &DownloadCache{
		Files: make(map[string]time.Time),
	}

	// 如果缓存文件不存在，返回空缓存
	if !FileExists(cacheFilePath) {
		return cache
	}

	// 读取缓存文件
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		fmt.Printf("警告: 读取缓存文件失败: %v\n", err)
		return cache
	}

	// 解析JSON
	if err := json.Unmarshal(data, cache); err != nil {
		fmt.Printf("警告: 解析缓存文件失败: %v\n", err)
		return &DownloadCache{
			Files: make(map[string]time.Time),
		}
	}

	return cache
}

// 保存下载缓存
func SaveDownloadCache(cache *DownloadCache) error {
	cacheFilePath := GetCacheFilePath()

	// 将缓存转换为JSON
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化缓存失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(cacheFilePath, data, 0644); err != nil {
		return fmt.Errorf("写入缓存文件失败: %w", err)
	}

	return nil
}

// 更新文件下载时间
func UpdateFileDownloadTime(filePath string) error {
	// 规范化文件路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	// 加载缓存
	cache := LoadDownloadCache()

	// 更新文件下载时间
	cache.Files[absPath] = time.Now()

	// 保存缓存
	return SaveDownloadCache(cache)
}

// 检查文件是否需要更新
func NeedsUpdate(filePath string) bool {
	// 如果文件不存在，需要下载
	if !FileExists(filePath) {
		return true
	}

	// 规范化文件路径
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		// 如果无法获取绝对路径，保守起见返回需要更新
		return true
	}

	// 加载缓存
	cache := LoadDownloadCache()

	// 获取文件最后下载时间
	lastDownload, exists := cache.Files[absPath]
	if !exists {
		// 如果没有记录，需要更新
		return true
	}

	// 检查是否超过缓存过期时间
	return time.Since(lastDownload).Hours() > CacheExpireHours
}

// 清理过期缓存记录
func CleanupExpiredCache() {
	cache := LoadDownloadCache()
	now := time.Now()
	changed := false

	// 检查每个文件记录
	for path, lastDownload := range cache.Files {
		// 如果文件不存在或者时间超过7天，从缓存中删除
		if !FileExists(path) || now.Sub(lastDownload).Hours() > 168 { // 7天 = 168小时
			delete(cache.Files, path)
			changed = true
		}
	}

	// 如果有变化，保存缓存
	if changed {
		SaveDownloadCache(cache)
	}
}
