package downutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// 错误类型常量
const (
	// ErrResourceNotFound 资源不存在错误（404）
	ErrResourceNotFound = "RESOURCE_NOT_FOUND"
	// ErrLowSpeed 下载速度过低错误
	ErrLowSpeed = "DOWNLOAD_SPEED_TOO_LOW"
)

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
	// LowSpeedDuration 判定低速持续时间（秒）
	LowSpeedDuration = 15
)

// ProgressTracker 下载进度跟踪器
type ProgressTracker struct {
	BytesCount    *atomic.Int64      // 已下载字节数
	FileSize      int64              // 文件总大小
	StartTime     time.Time          // 下载开始时间
	LastUpdate    time.Time          // 上次更新时间
	LastSize      int64              // 上次记录的大小
	Speed         float64            // 当前下载速度
	LowSpeedCount int                // 低速检测计数
	Done          chan struct{}      // 完成信号
	Name          string             // 下载的文件名
	Cancel        context.CancelFunc // 用于取消下载的函数
	Ctx           context.Context    // 下载上下文
	CancelReason  atomic.Value       // 取消原因
}

// NewProgressTracker 创建新的进度跟踪器
func NewProgressTracker(fileSize int64, name string) *ProgressTracker {
	now := time.Now()
	var counter atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())

	tracker := &ProgressTracker{
		BytesCount:    &counter,
		FileSize:      fileSize,
		StartTime:     now,
		LastUpdate:    now,
		LastSize:      0,
		Speed:         0,
		LowSpeedCount: 0,
		Done:          make(chan struct{}),
		Name:          name,
		Ctx:           ctx,
		Cancel:        cancel,
	}

	// 初始化取消原因为空字符串
	tracker.CancelReason.Store("")

	return tracker
}

// Close 关闭进度跟踪器
func (pt *ProgressTracker) Close() {
	close(pt.Done)
}

// GetCountingWriter 获取计数Writer
func (pt *ProgressTracker) GetCountingWriter(w io.Writer) io.Writer {
	return &CountingWriter{
		Writer:     w,
		BytesCount: pt.BytesCount,
	}
}

// MonitorSpeed 监控下载速度
func (pt *ProgressTracker) MonitorSpeed() {
	speedCheckTicker := time.NewTicker(SpeedCheckInterval * time.Second)
	defer speedCheckTicker.Stop()

	for {
		select {
		case <-speedCheckTicker.C:
			// 检查当前下载速度是否低于最小要求
			if pt.Speed < MinRequiredSpeed {
				pt.LowSpeedCount++

				// 提示用户当前速度过低
				fmt.Printf("\r    警告: 下载速度 (%s/s) 低于最小要求 (%s/s), 已持续 %d 秒...                   ",
					formatSize(int64(pt.Speed)),
					formatSize(int64(MinRequiredSpeed)),
					pt.LowSpeedCount*SpeedCheckInterval)

				// 如果连续多次检测到低速，则取消下载
				if pt.LowSpeedCount*SpeedCheckInterval >= LowSpeedDuration {
					pt.CancelReason.Store(ErrLowSpeed)

					// 取消下载
					pt.Cancel()

					// 显示取消消息
					fmt.Printf("\r    下载已取消: 速度过低 (%s/s)，低于最小要求 (%s/s)，网络可能存在问题                   \n",
						formatSize(int64(pt.Speed)),
						formatSize(int64(MinRequiredSpeed)))
					return
				}
			} else {
				// 速度恢复正常，重置计数
				if pt.LowSpeedCount > 0 {
					fmt.Printf("\r    下载速度已恢复正常: %s/s                                               ",
						formatSize(int64(pt.Speed)))
					pt.LowSpeedCount = 0
				}
			}
		case <-pt.Done:
			return
		}
	}
}

// DisplayProgress 显示下载进度
func (pt *ProgressTracker) DisplayProgress() {
	updateInterval := time.Duration(ProgressUpdateInterval) * time.Millisecond

	if pt.FileSize > 0 {
		// 已知文件大小的情况
		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pt.updateSpeed()
				pt.displayKnownSizeProgress()
			case <-pt.Done:
				return
			case <-pt.Ctx.Done():
				return
			}
		}
	} else {
		// 未知文件大小的情况
		ticker := time.NewTicker(updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pt.updateSpeed()
				pt.displayUnknownSizeProgress()
			case <-pt.Done:
				return
			case <-pt.Ctx.Done():
				return
			}
		}
	}
}

// updateSpeed 更新下载速度
func (pt *ProgressTracker) updateSpeed() {
	// 使用原子变量读取当前下载大小
	currentSize := pt.BytesCount.Load()

	// 计算下载速度 (bytes/second)
	currentTime := time.Now()
	timeElapsed := currentTime.Sub(pt.LastUpdate).Seconds()
	if timeElapsed > 0 {
		instantSpeed := float64(currentSize-pt.LastSize) / timeElapsed

		// 平滑速度计算 (指数移动平均)
		if pt.Speed == 0 {
			pt.Speed = instantSpeed
		} else {
			pt.Speed = 0.7*pt.Speed + 0.3*instantSpeed
		}

		pt.LastSize = currentSize
		pt.LastUpdate = currentTime
	}
}

// displayKnownSizeProgress 显示已知文件大小的下载进度
func (pt *ProgressTracker) displayKnownSizeProgress() {
	currentSize := pt.BytesCount.Load()
	progress := float64(currentSize) / float64(pt.FileSize) * 100
	speedStr := formatSize(int64(pt.Speed)) + "/s"

	if pt.Speed > MinValidSpeed {
		// 只有当速度大于最小有效值时才计算剩余时间
		remainingBytes := pt.FileSize - currentSize
		remainingSeconds := float64(remainingBytes) / pt.Speed
		// 限制最大预估时间为24小时，避免不合理的估计
		if remainingSeconds > 86400 { // 24小时 = 86400秒
			remainingSeconds = 86400
		}
		remainingTime := time.Duration(remainingSeconds) * time.Second

		fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 速度: %s 剩余时间: %s",
			progress,
			formatSize(currentSize),
			formatSize(pt.FileSize),
			speedStr,
			formatDuration(remainingTime))
	} else if pt.Speed > 0 {
		// 速度极低但不为0，显示速度但不显示剩余时间
		fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 速度: %s 剩余时间: 未知",
			progress,
			formatSize(currentSize),
			formatSize(pt.FileSize),
			speedStr)
	} else {
		// 速度为0，等待恢复
		fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 等待数据传输...",
			progress,
			formatSize(currentSize),
			formatSize(pt.FileSize))
	}
}

// displayUnknownSizeProgress 显示未知文件大小的下载进度
func (pt *ProgressTracker) displayUnknownSizeProgress() {
	currentSize := pt.BytesCount.Load()
	speedStr := formatSize(int64(pt.Speed)) + "/s"

	if pt.Speed > MinValidSpeed {
		fmt.Printf("\r    已下载: %s 速度: %s",
			formatSize(currentSize),
			speedStr)
	} else {
		fmt.Printf("\r    已下载: %s 等待数据传输...",
			formatSize(currentSize))
	}
}

// DisplaySummary 显示下载摘要
func (pt *ProgressTracker) DisplaySummary() {
	// 如果是因为取消而终止的下载，不显示下载摘要
	if pt.GetCancelReason() != "" {
		return
	}

	// 清除进度条行
	fmt.Print("\r                                                                                          \r")

	// 显示总下载时间和平均速度
	totalTime := time.Since(pt.StartTime)
	totalBytes := pt.BytesCount.Load()
	avgSpeed := float64(totalBytes) / totalTime.Seconds()
	fmt.Printf("    下载完成: 总大小 %s, 用时 %s, 平均速度 %s/s\n",
		formatSize(totalBytes),
		formatDuration(totalTime),
		formatSize(int64(avgSpeed)))
}

// GetCancelReason 获取下载取消的原因
func (pt *ProgressTracker) GetCancelReason() string {
	return pt.CancelReason.Load().(string)
}

// SetCancelReason 设置下载取消的原因
func (pt *ProgressTracker) SetCancelReason(reason string) {
	pt.CancelReason.Store(reason)
}

// DownloadFile 下载文件
func DownloadFile(client *http.Client, downloadUrl, storePath string, keepOldFile bool) error {
	// 创建目标文件的目录（如果不存在）
	if err := os.MkdirAll(filepath.Dir(storePath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 创建临时文件（使用唯一名称避免冲突）
	tempFile := storePath + fmt.Sprintf(".%d.download", time.Now().UnixNano())
	out, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		return err
	}

	// 设置User-Agent以避免某些服务器的限制
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		// 对于404错误，返回特殊错误类型
		if resp.StatusCode == http.StatusNotFound {
			return DownloadError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("资源不存在，HTTP状态码: %d (404 Not Found)", resp.StatusCode),
				Type:       ErrResourceNotFound,
			}
		}
		return fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode)
	}

	// 获取文件大小
	fileSize := resp.ContentLength
	fileName := filepath.Base(storePath)

	// 使用defer确保在函数退出时处理临时文件
	var downloadSuccess bool
	defer func() {
		out.Close()
		if !downloadSuccess {
			// 下载失败，删除临时文件
			os.Remove(tempFile)
		}
	}()

	// 创建进度跟踪器
	tracker := NewProgressTracker(fileSize, fileName)
	defer tracker.Close()

	// 启动进度监控协程
	go tracker.MonitorSpeed()
	go tracker.DisplayProgress()

	// 创建计数Writer
	countingWriter := tracker.GetCountingWriter(out)

	// 使用带上下文的缓冲区复制内容，支持取消
	buf := make([]byte, DownloadBufferSize)
	_, err = copyBufferWithContext(tracker.Ctx, countingWriter, resp.Body, buf)

	// 检查是否是因为速度过低取消导致的错误
	cancelReason := tracker.GetCancelReason()
	if cancelReason == ErrLowSpeed {
		return DownloadError{
			Message: fmt.Sprintf("下载已取消: 速度过低，低于最小要求 (%s/s)，网络可能存在问题",
				formatSize(int64(MinRequiredSpeed))),
			Type: ErrLowSpeed,
		}
	}

	// 检查其他错误
	if err != nil {
		return fmt.Errorf("下载内容失败: %w", err)
	}

	// 显示下载摘要
	tracker.DisplaySummary()

	// 关闭文件，确保内容写入磁盘
	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭文件失败: %w", err)
	}

	// 标记下载成功，避免在defer中删除临时文件
	downloadSuccess = true

	// 处理旧文件（如果存在）
	if FileExists(storePath) {
		if keepOldFile {
			// 保留旧文件，重命名为.old
			oldFilePath := storePath + ".old"
			// 如果已经存在.old文件，先删除它
			if FileExists(oldFilePath) {
				if err := os.Remove(oldFilePath); err != nil {
					return fmt.Errorf("删除旧的备份文件失败: %w", err)
				}
			}
			// 重命名当前文件为.old
			if err := os.Rename(storePath, oldFilePath); err != nil {
				return fmt.Errorf("备份旧文件失败: %w", err)
			}
			fmt.Printf("    已备份旧文件为: %s\n", oldFilePath)
		} else {
			// 不保留旧文件，直接删除
			if err := os.Remove(storePath); err != nil {
				return fmt.Errorf("错误:删除旧文件失败: %w", err)
			}
		}
	}

	// 重命名临时文件为最终文件名
	if err := os.Rename(tempFile, storePath); err != nil {
		return fmt.Errorf("错误: 重命名临时文件失败: %w", err)
	}

	// 更新文件下载时间缓存
	if err := UpdateFileDownloadTime(storePath); err != nil {
		fmt.Printf("    错误: 更新下载缓存失败: %v\n", err)
	}

	return nil
}

// copyBufferWithContext 带上下文的数据复制，支持取消操作
func copyBufferWithContext(ctx context.Context, dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	if buf == nil {
		buf = make([]byte, 32*1024)
	}

	for {
		select {
		case <-ctx.Done():
			// 上下文取消，停止复制
			return written, ctx.Err()
		default:
			// 继续复制
			nr, er := src.Read(buf)
			if nr > 0 {
				nw, ew := dst.Write(buf[0:nr])
				if nw > 0 {
					written += int64(nw)
				}
				if ew != nil {
					err = ew
					return
				}
				if nr != nw {
					err = io.ErrShortWrite
					return
				}
			}
			if er != nil {
				if er != io.EOF {
					err = er
				}
				return
			}
		}
	}
}

// CountingWriter 是一个包装io.Writer的结构，用于跟踪写入的字节数
type CountingWriter struct {
	Writer     io.Writer
	BytesCount *atomic.Int64
}

// Write 实现io.Writer接口，并原子地更新计数器
func (w *CountingWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if n > 0 {
		w.BytesCount.Add(int64(n))
	}
	return n, err
}
