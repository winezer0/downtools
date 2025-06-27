package main

import (
	"downtools/downfile"
	"downtools/downutils"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "config.yaml", "配置文件路径")
	outputDir := flag.String("output", "downloads", "下载文件保存目录")
	connectTimeout := flag.Int("connect-timeout", 30, "连接超时时间（秒）")
	idleTimeout := flag.Int("idle-timeout", 60, "空闲超时时间（秒）")
	retries := flag.Int("retries", 3, "下载失败重试次数")
	keepOld := flag.Bool("keep-old", false, "保留旧文件（重命名为.old）")
	forceUpdate := flag.Bool("force", false, "强制更新，忽略缓存")
	flag.Parse()

	// 显示程序信息
	fmt.Println("自动下载工具 v1.2")
	fmt.Printf("配置文件: %s\n", *configFile)
	fmt.Printf("输出目录: %s\n", *outputDir)
	fmt.Printf("连接超时: %d秒\n", *connectTimeout)
	fmt.Printf("空闲超时: %d秒\n", *idleTimeout)
	fmt.Printf("重试次数: %d次\n", *retries)
	fmt.Printf("保留旧文件: %v\n", *keepOld)
	fmt.Printf("启用强制更新: %v\n\n", *forceUpdate)

	// 读取配置文件
	config, err := downfile.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return
	}

	// 创建下载目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("创建下载目录失败: %v\n", err)
		return
	}

	// 清理过期缓存记录
	downfile.CleanupExpiredCache(24 * 7)

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

		success := processGroup(httpClient, items, groupDir, *forceUpdate, *keepOld, *retries)
		totalItems += len(items)
		successItems += success
	}

	fmt.Printf("\n所有下载任务完成: 成功 %d/%d\n", successItems, totalItems)
}

// 处理配置组
func processGroup(client *http.Client, items []downfile.ModuleItem, downloadDir string, forceUpdate bool, keepOld bool, retries int) int {
	successCount := 0
	for _, item := range items {
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
			// 处理GitHub URL
			downloadURL := url
			if strings.Contains(url, "github.com") && strings.Contains(url, "/blob/") {
				downloadURL = downfile.ConvertGitHubURL(url)
				fmt.Printf("    转换GitHub URL: %s -> %s\n", url, downloadURL)
			}

			// 尝试下载，支持重试
			for attempt := 1; attempt <= retries; attempt++ {
				if attempt > 1 {
					fmt.Printf("    第 %d 次重试下载...\n", attempt)
				} else {
					fmt.Printf("    尝试从 %s 下载...\n", downloadURL)
				}

				if err := downfile.DownloadFile(client, downloadURL, filePath, keepOld); err != nil {
					// 检查是否是404错误
					var downloadErr downfile.DownloadError
					fmt.Printf("    下载失败: %v\n", err)

					if errors.As(err, &downloadErr) && downloadErr.Type == downfile.ErrResourceNotFound {
						fmt.Printf("    资源不存在 (404)，请检查配置中的URL是否正确:%v\n", err)
						resourceNotFound = true
						break // 404错误不需要重试
					}

					// 如果不是最后一次尝试，则等待后重试
					if attempt < retries {
						waitTime := time.Duration(attempt) * 2 * time.Second
						fmt.Printf("    等待 %v 后重试...\n", waitTime)
						time.Sleep(waitTime)
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
