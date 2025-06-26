package downutils

import (
	"fmt"
	"os"
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
)

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
