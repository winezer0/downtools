package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DownloadItem 下载项目结构
type DownloadItem struct {
	Name         string   `yaml:"name"`
	File         string   `yaml:"file"`
	DownloadURLs []string `yaml:"download-urls"`
	KeepUpdated  bool     `yaml:"keep-updated"`
}

// Config 配置文件结构
type Config map[string][]DownloadItem

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

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "config.yaml", "配置文件路径")
	outputDir := flag.String("output", "downloads", "下载文件保存目录")
	connectTimeout := flag.Int("connect-timeout", 30, "连接超时时间（秒）")
	idleTimeout := flag.Int("idle-timeout", 60, "空闲超时时间（秒）")
	retries := flag.Int("retries", 3, "下载失败重试次数")
	keepOld := flag.Bool("keep-old", false, "保留旧文件（重命名为.old）")
	flag.Parse()

	// 显示程序信息
	fmt.Println("自动下载工具 v1.1")
	fmt.Printf("配置文件: %s\n", *configFile)
	fmt.Printf("输出目录: %s\n", *outputDir)
	fmt.Printf("连接超时: %d秒\n", *connectTimeout)
	fmt.Printf("空闲超时: %d秒\n", *idleTimeout)
	fmt.Printf("重试次数: %d次\n", *retries)
	fmt.Printf("保留旧文件: %v\n\n", *keepOld)

	// 读取配置文件
	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return
	}

	// 创建下载目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("创建下载目录失败: %v\n", err)
		return
	}

	// 设置HTTP客户端，只设置连接超时，不设置读取超时
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(*connectTimeout) * time.Second, // 连接超时
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       time.Duration(*idleTimeout) * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: time.Duration(*connectTimeout) * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		// 不设置整体超时，避免大文件下载中断
	}

	// 处理所有配置组
	totalItems := 0
	successItems := 0

	for groupName, items := range config {
		fmt.Printf("\n处理配置组: %s\n", groupName)
		// 为每个组创建子目录
		groupDir := filepath.Join(*outputDir, groupName)
		if err := os.MkdirAll(groupDir, 0755); err != nil {
			fmt.Printf("创建目录 %s 失败: %v\n", groupDir, err)
			continue
		}

		success := processGroup(items, groupDir, httpClient, *retries, *keepOld)
		totalItems += len(items)
		successItems += success
	}

	fmt.Printf("\n所有下载任务完成: 成功 %d/%d\n", successItems, totalItems)
}

// 加载配置文件
func loadConfig(filename string) (Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析YAML失败: %w", err)
	}

	return config, nil
}

// 处理配置组
func processGroup(items []DownloadItem, downloadDir string, client *http.Client, retries int, keepOld bool) int {
	successCount := 0
	for _, item := range items {
		filePath := filepath.Join(downloadDir, item.File)

		// 检查文件是否存在
		fileExists := fileExists(filePath)
		if fileExists && !item.KeepUpdated {
			fmt.Printf("  文件 %s 已存在且不需要更新，跳过下载\n", item.File)
			successCount++
			continue
		}

		fmt.Printf("  开始下载 %s...\n", item.Name)
		success := false

		// 尝试从每个URL下载
		for _, url := range item.DownloadURLs {
			// 处理GitHub URL
			downloadURL := url
			if strings.Contains(url, "github.com") && strings.Contains(url, "/blob/") {
				downloadURL = convertGitHubURL(url)
				fmt.Printf("    转换GitHub URL: %s -> %s\n", url, downloadURL)
			}

			// 尝试下载，支持重试
			for attempt := 1; attempt <= retries; attempt++ {
				if attempt > 1 {
					fmt.Printf("    第 %d 次重试下载...\n", attempt)
				} else {
					fmt.Printf("    尝试从 %s 下载...\n", downloadURL)
				}

				if err := downloadFile(downloadURL, filePath, client, keepOld); err != nil {
					fmt.Printf("    下载失败: %v\n", err)
					// 如果不是最后一次尝试，则等待后重试
					if attempt < retries {
						waitTime := time.Duration(attempt) * 2 * time.Second
						fmt.Printf("    等待 %v 后重试...\n", waitTime)
						time.Sleep(waitTime)
						continue
					}
					break // 所有重试都失败
				} else {
					success = true
					successCount++
					fmt.Printf("    成功下载 %s 到 %s\n", item.Name, filePath)
					break // 下载成功，不需要继续重试
				}
			}

			if success {
				break // 当前URL下载成功，不需要尝试下一个URL
			}
		}

		if !success {
			fmt.Printf("  所有下载源都失败，无法下载 %s\n", item.Name)
		}
	}
	return successCount
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

// 下载文件
func downloadFile(url, filePath string, client *http.Client, keepOld bool) error {
	// 创建HTTP请求
	req, err := http.NewRequest("GET", url, nil)
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
		return fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode)
	}

	// 获取文件大小
	fileSize := resp.ContentLength

	// 创建目标文件的目录（如果不存在）
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 创建临时文件（使用唯一名称避免冲突）
	tempFilePath := filePath + fmt.Sprintf(".%d.download", time.Now().UnixNano())
	out, err := os.Create(tempFilePath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	// 使用defer确保在函数退出时处理临时文件
	var downloadSuccess bool
	defer func() {
		out.Close()
		if !downloadSuccess {
			// 下载失败，删除临时文件
			os.Remove(tempFilePath)
		}
	}()

	// 创建进度显示
	done := make(chan struct{})
	defer close(done)

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
				info, err := out.Stat()
				if err != nil {
					continue
				}

				currentSize := info.Size()
				if currentSize == lastProgressSize {
					noProgressDuration += StallCheckInterval * time.Second
					if noProgressDuration >= 30*time.Second {
						// 30秒内没有进度，显示警告信息
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
					info, err := out.Stat()
					if err != nil {
						continue
					}

					currentTime := time.Now()
					currentSize := info.Size()

					// 计算下载速度 (bytes/second)
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
					info, err := out.Stat()
					if err != nil {
						continue
					}

					currentTime := time.Now()
					currentSize := info.Size()

					// 计算下载速度
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

	// 使用缓冲区复制内容，提高效率
	buf := make([]byte, DownloadBufferSize)
	_, err = io.CopyBuffer(out, resp.Body, buf)
	if err != nil {
		return fmt.Errorf("下载内容失败: %w", err)
	}

	// 下载完成，清除进度条行
	fmt.Print("\r                                                                                          \r")

	// 显示总下载时间和平均速度
	totalTime := time.Since(startTime)
	info, _ := out.Stat()
	avgSpeed := float64(info.Size()) / totalTime.Seconds()
	fmt.Printf("    下载完成: 总大小 %s, 用时 %s, 平均速度 %s/s\n",
		formatSize(info.Size()),
		formatDuration(totalTime),
		formatSize(int64(avgSpeed)))

	// 关闭文件，确保内容写入磁盘
	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭文件失败: %w", err)
	}

	// 标记下载成功，避免在defer中删除临时文件
	downloadSuccess = true

	// 处理旧文件（如果存在）
	if fileExists(filePath) {
		if keepOld {
			// 保留旧文件，重命名为.old
			oldFilePath := filePath + ".old"
			// 如果已经存在.old文件，先删除它
			if fileExists(oldFilePath) {
				if err := os.Remove(oldFilePath); err != nil {
					return fmt.Errorf("删除旧的备份文件失败: %w", err)
				}
			}
			// 重命名当前文件为.old
			if err := os.Rename(filePath, oldFilePath); err != nil {
				return fmt.Errorf("备份旧文件失败: %w", err)
			}
			fmt.Printf("    已备份旧文件为: %s\n", oldFilePath)
		} else {
			// 不保留旧文件，直接删除
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("删除旧文件失败: %w", err)
			}
		}
	}

	// 重命名临时文件为最终文件名
	if err := os.Rename(tempFilePath, filePath); err != nil {
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// 检查文件是否存在
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// 转换GitHub URL为原始内容URL
func convertGitHubURL(url string) string {
	// 只转换blob URL，不转换releases下载链接
	if strings.Contains(url, "/releases/") {
		return url
	}
	return strings.Replace(strings.Replace(url, "github.com", "raw.githubusercontent.com", 1), "/blob/", "/", 1)
}
