# 自动下载工具 (v0.0.9)

这是一个基于YAML配置的自动下载工具，可以根据配置文件下载指定的资源。

## 功能特点

- 支持从多个URL源尝试下载
- 支持是否更新已存在的文件
- 自动处理GitHub URLs
- 支持多个配置组
- 支持命令行参数配置
- 支持HTTP和SOCKS5代理
- 缓存控制和过期清理
- 失败重试机制

## 使用方法

1. 确保已安装Go环境
2. 下载本项目
3. 配置`config.yaml`文件
4. 运行程序：

```bash
# 使用默认配置
go run main.go

# 指定配置文件
go run main.go -c my-config.yaml

# 指定输出目录
go run main.go -o data

# 使用代理
go run main.go -p http://127.0.0.1:8080
go run main.go -p socks5://127.0.0.1:1080
```

## 命令行参数

| 短参数 | 长参数 | 默认值 | 说明 |
|------|--------|------|------|
| -c | --config | config.yaml | 配置文件路径 |
| -o | --output | downloads | 下载文件保存目录 |
| -t | --connect-timeout | 10 | 连接超时时间（秒） |
| -T | --idle-timeout | 60 | 空闲超时时间（秒） |
| -r | --retries | 1 | 下载失败重试次数 |
| -k | --keep-old | false | 保留旧文件（重命名为.old） |
| -f | --force | false | 强制更新，忽略缓存 |
| -p | --proxy | | 代理URL（支持http://和socks5://格式） |
| -E | --cache-expire | 24 | 缓存过期时间（小时） |
| -e | --enable-all | false | 下载所有项（即使enable=false） |
| -v | --version | false | 显示版本信息 |

## 配置文件格式

```yaml
databases:
  - module: 模块名称
    filename: 保存的文件名
    download-urls:
      - https://url1.com/file
      - https://url2.com/file
    keep-updated: true  # 如果为false，则已存在文件时不会更新
    enable: true  # 如果为false，则会忽略这条规则（除非使用--enable-all参数）
```

## 构建可执行文件

```bash
go build -o downtools
```

然后可以直接运行生成的可执行文件：

```bash
./downtools  # Linux/Mac
downtools.exe  # Windows
```

## 示例配置

配置文件示例（config.yaml）:

```yaml
databases:
  - module: qqwry
    filename: qqwry.dat
    download-urls:
      - https://github.com/metowolf/qqwry.dat/releases/latest/download/qqwry.dat
    keep-updated: true
    enable: true

  - module: cdn-domains
    filename: cdn.yml
    download-urls:
      - https://raw.githubusercontent.com/4ft35t/cdn/master/src/cdn.yml
    keep-updated: false
    enable: true
```
