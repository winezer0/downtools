package main

import (
	"downtools/downfile"
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"path/filepath"
)

// AppConfig 应用配置结构体
type AppConfig struct {
	ConfigFile     string `short:"c" long:"config" description:"配置文件路径" default:"config.yaml"`
	OutputDir      string `short:"o" long:"output" description:"下载文件保存目录" default:"downloads"`
	ConnectTimeout int    `short:"t" long:"connect-timeout" description:"连接超时时间（秒）" default:"10"`
	IdleTimeout    int    `short:"T" long:"idle-timeout" description:"空闲超时时间（秒）" default:"60"`
	Retries        int    `short:"r" long:"retries" description:"下载失败重试次数" default:"1"`
	KeepOld        bool   `short:"k" long:"keep-old" description:"保留旧文件（重命名为.old）"`
	ForceUpdate    bool   `short:"f" long:"force" description:"强制更新，忽略缓存"`
	ProxyURL       string `short:"p" long:"proxy" description:"代理URL（支持http://和socks5://格式）" default:"socks5://127.0.0.1:10808"`
	Version        bool   `short:"v" long:"version" description:"显示版本信息"`
}

const Version = "1.3.1"

// DisplayConfig 显示应用配置信息
func (config *AppConfig) DisplayConfig() {
	fmt.Printf("自动下载工具 v%s\n", Version)
	fmt.Printf("配置文件: %s\n", config.ConfigFile)
	fmt.Printf("输出目录: %s\n", config.OutputDir)
	fmt.Printf("连接超时: %d秒\n", config.ConnectTimeout)
	fmt.Printf("空闲超时: %d秒\n", config.IdleTimeout)
	fmt.Printf("重试次数: %d次\n", config.Retries)
	fmt.Printf("保留旧文件: %v\n", config.KeepOld)
	fmt.Printf("使用代理: %s\n", config.ProxyURL)
	fmt.Printf("启用强制更新: %v\n", config.ForceUpdate)
	fmt.Println()
}

func main() {
	// 解析命令行参数
	var config AppConfig
	parser := flags.NewParser(&config, flags.Default)
	parser.Name = "downtools"
	parser.Usage = "[OPTIONS]"

	// 解析命令行参数
	_, err := parser.Parse()
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		}
	}

	// 显示版本信息后退出
	if config.Version {
		fmt.Printf("自动下载工具 v%s\n", Version)
		os.Exit(0)
	}

	// 显示程序信息
	config.DisplayConfig()

	// 读取配置文件
	downloadConfig, err := downfile.LoadConfig(config.ConfigFile)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
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

	for groupName, downItems := range downloadConfig {
		fmt.Printf("\n处理配置组: %s\n", groupName)

		// 为每个组创建子目录
		groupDir := filepath.Join(config.OutputDir, groupName)
		if err := os.MkdirAll(groupDir, 0755); err != nil {
			fmt.Printf("创建目录 %s 失败: %v\n", groupDir, err)
			continue
		}

		success := downfile.ProcessGroup(httpClient, downItems, groupDir, config.ForceUpdate, config.KeepOld, config.Retries)
		totalItems += len(downItems)
		successItems += success
	}

	fmt.Printf("\n所有下载任务完成: 成功 %d/%d\n", successItems, totalItems)
}
