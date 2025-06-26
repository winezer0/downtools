# 自动下载工具

这是一个基于YAML配置的自动下载工具，可以根据配置文件下载指定的资源。

## 功能特点

- 支持从多个URL源尝试下载
- 支持是否更新已存在的文件
- 自动处理GitHub URLs
- 支持多个配置组
- 支持命令行参数配置

## 使用方法

1. 确保已安装Go环境
2. 下载本项目
3. 配置`config.yaml`文件
4. 运行程序：

```bash
# 使用默认配置
go run main.go

# 指定配置文件
go run main.go -config=my-config.yaml

# 指定输出目录
go run main.go -output=data

# 指定下载超时时间
go run main.go -timeout=60
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| -config | config.yaml | 配置文件路径 |
| -output | downloads | 下载文件保存目录 |
| -timeout | 30 | 下载超时时间（秒） |

## 配置文件格式

```yaml
databases:
  - name: 资源名称
    file: 保存的文件名
    download-urls:
      - https://url1.com/file
      - https://url2.com/file
    keep-updated: true  # 如果为false，则已存在文件时不会更新
```

## 构建可执行文件

```bash
go build -o downloader
```

然后可以直接运行生成的可执行文件：

```bash
./downloader  # Linux/Mac
downloader.exe  # Windows
```
