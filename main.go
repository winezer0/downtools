package main

import (
	"context"
	"downtools/downfile"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppConfig 应用配置结构体
type AppConfig struct {
	ConfigFile     string // 配置文件路径
	OutputDir      string // 下载文件保存目录
	ConnectTimeout int    // 连接超时时间（秒）
	IdleTimeout    int    // 空闲超时时间（秒）
	MaxTotalTime   int    // 最大总下载时间（分钟）
	Retries        int    // 下载失败重试次数
	KeepOld        bool   // 保留旧文件（重命名为.old）
	ForceUpdate    bool   // 强制更新，忽略缓存
	ProxyURL       string // 代理URL（支持http和socks5）
}

// ParseFlags 解析命令行参数
func ParseFlags() *AppConfig {
	config := &AppConfig{}

	flag.StringVar(&config.ConfigFile, "config", "config.yaml", "配置文件路径")
	flag.StringVar(&config.OutputDir, "output", "downloads", "下载文件保存目录")
	flag.IntVar(&config.ConnectTimeout, "connect-timeout", 30, "连接超时时间（秒）")
	flag.IntVar(&config.IdleTimeout, "idle-timeout", 60, "空闲超时时间（秒）")
	flag.IntVar(&config.Retries, "retries", 3, "下载失败重试次数")
	flag.BoolVar(&config.KeepOld, "keep-old", false, "保留旧文件（重命名为.old）")
	flag.BoolVar(&config.ForceUpdate, "force", false, "强制更新，忽略缓存")
	flag.StringVar(&config.ProxyURL, "proxy", "socks5://127.0.0.1:10808", "代理URL（支持http://和socks5://格式）")

	flag.Parse()
	return config
}

// DisplayConfig 显示应用配置信息
func (config *AppConfig) DisplayConfig() {
	fmt.Println("自动下载工具 v1.3")
	fmt.Printf("配置文件: %s\n", config.ConfigFile)
	fmt.Printf("输出目录: %s\n", config.OutputDir)
	fmt.Printf("连接超时: %d秒\n", config.ConnectTimeout)
	fmt.Printf("空闲超时: %d秒\n", config.IdleTimeout)
	fmt.Printf("最大总时间: %d分钟\n", config.MaxTotalTime)
	fmt.Printf("重试次数: %d次\n", config.Retries)
	fmt.Printf("保留旧文件: %v\n", config.KeepOld)
	fmt.Printf("启用强制更新: %v\n", config.ForceUpdate)
	fmt.Printf("HTTP代理: %s\n", config.ProxyURL)
	fmt.Println()
}

func main() {
	// 解析命令行参数
	config := ParseFlags()

	// 创建带超时的上下文，如果设置了最大时间
	var ctx context.Context
	var cancel context.CancelFunc

	if config.MaxTotalTime > 0 {
		ctx, cancel = context.WithTimeout(
			context.Background(),
			time.Duration(config.MaxTotalTime)*time.Minute,
		)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel() // 确保在函数结束时取消上下文

	// 显示程序信息
	config.DisplayConfig()

	// 读取配置文件
	downloadConfig, err := downfile.LoadConfig(config.ConfigFile)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return
	}

	// 创建下载目录
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		fmt.Printf("创建下载目录失败: %v\n", err)
		return
	}

	// 清理过期缓存记录
	downfile.CleanupExpiredCache(24 * 7)

	// 创建HTTP客户端配置
	clientConfig := &downfile.ClientConfig{
		ConnectTimeout: config.ConnectTimeout,
		IdleTimeout:    config.IdleTimeout,
		ProxyURL:       config.ProxyURL,
	}

	// 创建HTTP客户端
	httpClient, err := downfile.CreateHTTPClient(clientConfig)
	if err != nil {
		fmt.Printf("创建HTTP客户端失败: %v\n", err)
		return
	}

	// 处理所有配置组
	totalItems := 0
	successItems := 0

	for groupName, items := range downloadConfig {
		fmt.Printf("\n处理配置组: %s\n", groupName)

		// 检查上下文是否已取消
		if err := ctx.Err(); err != nil {
			fmt.Printf("下载已取消: 超过最大允许时间 %d 分钟\n", config.MaxTotalTime)
			break
		}

		// 为每个组创建子目录
		groupDir := filepath.Join(config.OutputDir, groupName)
		if err := os.MkdirAll(groupDir, 0755); err != nil {
			fmt.Printf("创建目录 %s 失败: %v\n", groupDir, err)
			continue
		}

		success := processGroup(ctx, httpClient, items, groupDir, config.ForceUpdate, config.KeepOld, config.Retries)
		totalItems += len(items)
		successItems += success
	}

	fmt.Printf("\n所有下载任务完成: 成功 %d/%d\n", successItems, totalItems)
}

// 处理配置组
func processGroup(ctx context.Context, client *http.Client, items []downfile.ModuleItem, downloadDir string, forceUpdate bool, keepOld bool, retries int) int {
	successCount := 0
	for _, item := range items {
		// 检查上下文是否已取消
		if err := ctx.Err(); err != nil {
			fmt.Printf("  下载已取消: %v\n", err)
			return successCount
		}

		filePath := filepath.Join(downloadDir, item.FileName)

		// 检查文件是否存在以及是否需要更新
		fileExists := downfile.FileExists(filePath)
		needsUpdate := forceUpdate || !fileExists || (item.KeepUpdated && downfile.NeedsUpdate(filePath, downfile.CacheExpireHours))

		if fileExists && !needsUpdate {
			fmt.Printf("  文件 %s 已存在且不需要更新，跳过下载\n", item.FileName)
			successCount++
			continue
		}

		fmt.Printf("  开始下载 %s...\n", item.Module)
		success := false
		resourceNotFound := false

		// 尝试从每个URL下载
		for _, url := range item.DownloadURLs {
			// 检查上下文是否已取消
			if err := ctx.Err(); err != nil {
				fmt.Printf("    下载已取消: %v\n", err)
				return successCount
			}

			// 处理GitHub URL
			downloadURL := url
			if strings.Contains(url, "github.com") && strings.Contains(url, "/blob/") {
				downloadURL = downfile.ConvertGitHubURL(url)
				fmt.Printf("    转换GitHub URL: %s -> %s\n", url, downloadURL)
			}

			// 尝试下载，支持重试
			for attempt := 1; attempt <= retries; attempt++ {
				// 检查上下文是否已取消
				if err := ctx.Err(); err != nil {
					fmt.Printf("    下载已取消: %v\n", err)
					return successCount
				}

				if attempt > 1 {
					fmt.Printf("    第 %d 次重试下载...\n", attempt)
				} else {
					fmt.Printf("    尝试从 %s 下载...\n", downloadURL)
				}

				// 创建带上下文的请求
				req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
				if err != nil {
					fmt.Printf("    创建HTTP请求失败: %v\n", err)
					break
				}

				// 设置User-Agent
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

				// 使用普通的HTTP请求
				if err := downfile.DownloadFile(client, downloadURL, filePath, keepOld); err != nil {
					// 检查是否是404错误
					var downloadErr downfile.DownloadError
					fmt.Printf("    下载失败: %v\n", err)

					if errors.As(err, &downloadErr) && downloadErr.Type == downfile.ErrResourceNotFound {
						fmt.Printf("    资源不存在 (404)，请检查配置中的URL是否正确\n")
						resourceNotFound = true
						break // 404错误不需要重试
					}

					// 检查是否是上下文取消
					if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
						fmt.Printf("    下载已取消: 超过最大允许时间\n")
						return successCount
					}

					// 如果不是最后一次尝试，则等待后重试
					if attempt < retries {
						waitTime := time.Duration(attempt) * 2 * time.Second
						fmt.Printf("    等待 %v 后重试...\n", waitTime)

						// 使用带上下文的睡眠
						select {
						case <-time.After(waitTime):
							// 继续重试
						case <-ctx.Done():
							fmt.Printf("    下载已取消: %v\n", ctx.Err())
							return successCount
						}

						continue
					}
					break // 所有重试都失败
				} else {
					fmt.Printf("    成功下载 %s 到 %s\n", item.Module, filePath)
					successCount++
					success = true
					break // 下载成功，不需要继续重试
				}
			}

			if success || resourceNotFound {
				break // 当前URL下载成功或资源不存在，不需要尝试下一个URL
			}
		}

		if !success {
			if resourceNotFound {
				fmt.Printf("  警告: %s 的资源不存在，请检查配置文件中的URL\n", item.Module)
			} else {
				fmt.Printf("  错误: 所有下载源都失败，无法下载 %s\n", item.Module)
			}
		}
	}
	return successCount
}
