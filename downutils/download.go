package downutils

import (
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
	// StallCheckInterval 下载停滞检测间隔（秒）
	StallCheckInterval = 10
	// ProgressUpdateInterval 下载进度更新间隔（毫秒）
	ProgressUpdateInterval = 500
	// DownloadBufferSize 下载缓冲区大小
	DownloadBufferSize = 32 * 1024 // 32KB
	// CacheFileName 缓存文件名
	CacheFileName = ".download_cache.json"
	// CacheExpireHours 缓存过期时间（小时）
	CacheExpireHours = 1
)

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

	// 使用defer确保在函数退出时处理临时文件
	var downloadSuccess bool
	defer func() {
		out.Close()
		if !downloadSuccess {
			// 下载失败，删除临时文件
			os.Remove(tempFile)
		}
	}()

	// 创建进度显示
	done := make(chan struct{})
	defer close(done)

	// 使用原子变量记录已下载字节数，避免频繁的文件系统调用
	var downloadedBytes atomic.Int64

	// 下载速度计算变量
	startTime := time.Now()
	lastUpdate := startTime
	var lastSize int64 = 0
	var speedBytesPerSec float64 = 0
	var noProgressDuration time.Duration = 0
	updateInterval := time.Duration(ProgressUpdateInterval) * time.Millisecond

	// 创建一个通道用于检测下载是否停滞
	stalled := make(chan struct{}, 1)
	defer close(stalled)

	// 监控下载进度，检测是否停滞
	go func() {
		var lastProgressSize int64 = 0
		stallCheckTicker := time.NewTicker(StallCheckInterval * time.Second)
		defer stallCheckTicker.Stop()

		for {
			select {
			case <-stallCheckTicker.C:
				// 使用原子变量读取当前下载大小
				currentSize := downloadedBytes.Load()

				if currentSize == lastProgressSize {
					noProgressDuration += StallCheckInterval * time.Second
					if noProgressDuration >= 10*time.Second {
						// 10秒内没有进度，显示警告信息
						fmt.Printf("\r    下载停滞: 已等待 %s 无数据传输...                    ",
							formatDuration(noProgressDuration))
					} else {
						// 显示短暂停滞信息
						fmt.Printf("\r    下载似乎暂停了，但仍在等待数据...                    ")
					}
				} else {
					// 有进度，重置停滞计时
					noProgressDuration = 0
					lastProgressSize = currentSize
				}
			case <-done:
				return
			}
		}
	}()

	go func() {
		if fileSize > 0 {
			ticker := time.NewTicker(updateInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// 使用原子变量读取当前下载大小
					currentSize := downloadedBytes.Load()

					// 计算下载速度 (bytes/second)
					currentTime := time.Now()
					timeElapsed := currentTime.Sub(lastUpdate).Seconds()
					if timeElapsed > 0 {
						instantSpeed := float64(currentSize-lastSize) / timeElapsed

						// 平滑速度计算 (指数移动平均)
						if speedBytesPerSec == 0 {
							speedBytesPerSec = instantSpeed
						} else {
							speedBytesPerSec = 0.7*speedBytesPerSec + 0.3*instantSpeed
						}

						lastSize = currentSize
						lastUpdate = currentTime
					}

					// 计算进度百分比
					progress := float64(currentSize) / float64(fileSize) * 100

					// 格式化速度显示
					speedStr := formatSize(int64(speedBytesPerSec)) + "/s"

					// 显示进度、速度和剩余时间
					if speedBytesPerSec > MinValidSpeed {
						// 只有当速度大于最小有效值时才计算剩余时间
						remainingBytes := fileSize - currentSize
						remainingSeconds := float64(remainingBytes) / speedBytesPerSec
						// 限制最大预估时间为24小时，避免不合理的估计
						if remainingSeconds > 86400 { // 24小时 = 86400秒
							remainingSeconds = 86400
						}
						remainingTime := time.Duration(remainingSeconds) * time.Second

						fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 速度: %s 剩余时间: %s",
							progress,
							formatSize(currentSize),
							formatSize(fileSize),
							speedStr,
							formatDuration(remainingTime))
					} else if speedBytesPerSec > 0 {
						// 速度极低但不为0，显示速度但不显示剩余时间
						fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 速度: %s 剩余时间: 未知",
							progress,
							formatSize(currentSize),
							formatSize(fileSize),
							speedStr)
					} else {
						// 速度为0，等待恢复
						fmt.Printf("\r    下载进度: %.1f%% (%s/%s) 等待数据传输...",
							progress,
							formatSize(currentSize),
							formatSize(fileSize))
					}

				case <-done:
					return
				}
			}
		} else {
			// 对于未知大小的文件，只显示已下载大小和速度
			ticker := time.NewTicker(updateInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// 使用原子变量读取当前下载大小
					currentSize := downloadedBytes.Load()

					// 计算下载速度
					currentTime := time.Now()
					timeElapsed := currentTime.Sub(lastUpdate).Seconds()
					if timeElapsed > 0 {
						instantSpeed := float64(currentSize-lastSize) / timeElapsed
						if speedBytesPerSec == 0 {
							speedBytesPerSec = instantSpeed
						} else {
							speedBytesPerSec = 0.7*speedBytesPerSec + 0.3*instantSpeed
						}

						lastSize = currentSize
						lastUpdate = currentTime
					}

					// 格式化速度显示
					speedStr := formatSize(int64(speedBytesPerSec)) + "/s"

					// 显示已下载大小和速度
					if speedBytesPerSec > MinValidSpeed {
						fmt.Printf("\r    已下载: %s 速度: %s",
							formatSize(currentSize),
							speedStr)
					} else {
						fmt.Printf("\r    已下载: %s 等待数据传输...",
							formatSize(currentSize))
					}

				case <-done:
					return
				}
			}
		}
	}()

	// 创建一个自定义的Writer，用于跟踪已写入的字节数
	countingWriter := &CountingWriter{
		Writer:     out,
		BytesCount: &downloadedBytes,
	}

	// 使用缓冲区复制内容，提高效率
	buf := make([]byte, DownloadBufferSize)
	_, err = io.CopyBuffer(countingWriter, resp.Body, buf)
	if err != nil {
		return fmt.Errorf("下载内容失败: %w", err)
	}

	// 下载完成，清除进度条行
	fmt.Print("\r                                                                                          \r")

	// 显示总下载时间和平均速度
	totalTime := time.Since(startTime)
	totalBytes := downloadedBytes.Load()
	avgSpeed := float64(totalBytes) / totalTime.Seconds()
	fmt.Printf("    下载完成: 总大小 %s, 用时 %s, 平均速度 %s/s\n",
		formatSize(totalBytes),
		formatDuration(totalTime),
		formatSize(int64(avgSpeed)))

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
