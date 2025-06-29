package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/winezer0/downtools/downfile"
)

// AppConfig 应用配置结构体
type AppConfig struct {
	ConfigFile     string  `short:"c" long:"config" description:"配置文件路径" default:"config.yaml"`
	OutputDir      string  `short:"o" long:"output" description:"下载文件保存目录" default:"downloads"`
	ConnectTimeout int     `short:"t" long:"connect-timeout" description:"连接超时时间（秒）" default:"10"`
	IdleTimeout    int     `short:"T" long:"idle-timeout" description:"空闲超时时间（秒）" default:"60"`
	Retries        int     `short:"r" long:"retries" description:"下载失败重试次数" default:"1"`
	KeepOld        bool    `short:"k" long:"keep-old" description:"保留旧文件（重命名为.old）"`
	ForceUpdate    bool    `short:"f" long:"force" description:"强制更新，忽略缓存"`
	ProxyURL       string  `short:"p" long:"proxy" description:"代理URL（支持http://和socks5://格式）" default:""`
	CacheExpire    float64 `short:"E" long:"cache-expire" description:"缓存过期时间（小时）" default:"24"`
	EnableAll      bool    `short:"e" long:"enable-all" description:"下载所有项 即使enable=false"`
	Version        bool    `short:"v" long:"version" description:"显示版本信息"`
}

const Version = "v0.0.6"

// DisplayConfig 显示应用配置信息
func (config *AppConfig) DisplayConfig() {
	fmt.Printf("自动下载工具 %s\n", Version)
	fmt.Printf("配置文件: %s\n", config.ConfigFile)
	fmt.Printf("输出目录: %s\n", config.OutputDir)
	fmt.Printf("连接超时: %d秒\n", config.ConnectTimeout)
	fmt.Printf("空闲超时: %d秒\n", config.IdleTimeout)
	fmt.Printf("重试次数: %d次\n", config.Retries)
	fmt.Printf("保留旧文件: %v\n", config.KeepOld)
	fmt.Printf("使用代理: %s\n", config.ProxyURL)
	fmt.Printf("启用强制更新: %v\n", config.ForceUpdate)
	fmt.Printf("缓存过期时间: %v小时\n", config.CacheExpire)
	fmt.Printf("下载未启用项: %v\n", config.EnableAll)
	fmt.Println()
}

func main() {
	// 解析命令行参数
	var appConfig AppConfig
	parser := flags.NewParser(&appConfig, flags.Default)
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
	if appConfig.Version {
		fmt.Printf("自动下载工具 v%s\n", Version)
		os.Exit(0)
	}

	// 显示程序信息
	appConfig.DisplayConfig()

	// 读取配置文件
	downloadConfig, err := downfile.LoadConfig(appConfig.ConfigFile)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return
	}

	// 清理过期缓存记录
	downfile.CacheExpireHours = appConfig.CacheExpire
	downfile.CleanupExpiredCache()

	// 创建HTTP客户端配置
	clientConfig := &downfile.ClientConfig{
		ConnectTimeout: appConfig.ConnectTimeout,
		IdleTimeout:    appConfig.IdleTimeout,
		ProxyURL:       appConfig.ProxyURL,
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
		// 如果未启用enable过滤，则只处理enable=true的项
		if !appConfig.EnableAll {
			downItems = downfile.FilterEnableItems(downItems)
		}
		if len(downItems) > 0 {
			fmt.Printf("\n处理配置组: %s\n", groupName)
			success := downfile.ProcessDownItems(httpClient, downItems, appConfig.OutputDir, appConfig.ForceUpdate, appConfig.KeepOld, appConfig.Retries)
			totalItems += len(downItems)
			successItems += success
		}
	}

	// 清理未完成的下载文件
	if err := downfile.CleanupIncompleteDownloads(appConfig.OutputDir); err != nil {
		fmt.Printf("清理未完成下载文件失败: %v\n", err)
	} else {
		fmt.Println("未完成下载文件清理完成")
	}
}
