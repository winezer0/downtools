package downutils

// ModuleItem 下载项目结构
type ModuleItem struct {
	Module       string   `yaml:"module"`
	FileName     string   `yaml:"filename"`
	DownloadURLs []string `yaml:"download-urls"`
	KeepUpdated  bool     `yaml:"keep-updated"`
}

// Config 配置文件结构
type Config map[string][]ModuleItem
