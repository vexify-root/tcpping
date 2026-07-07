# tcpping

通过 TCP 三次握手探测目标端口可达性并测量连接延迟的轻量工具，类似 ICMP ping，但工作在 TCP 层。

## 特性

- 探测指定 IP/域名 + 端口的可达性
- 测量 TCP 连接延迟（毫秒）
- 支持无限发送（`-c 0`），Ctrl+C 优雅退出
- 输出丢包率、最小/平均/最大延迟统计
- 纯 Go 实现，无第三方依赖
- 跨平台：Linux / macOS / Windows / FreeBSD / OpenBSD

## 快速开始

```bash
git clone https://github.com/li63050a/tcpping.git
cd tcpping
go build -o tcpping main.go
./tcpping google.com 443 -c 4
```

## 用法

```
tcpping <目标地址> <端口> [-c 次数] [-i 间隔] [-t 超时] [-dns DNS服务器]
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-c` | 0 | 发送次数，0 表示无限发送 |
| `-i` | 1s | 发送间隔，如 `500ms`、`2s` |
| `-t` | 1s | 单次连接超时，如 `2s`、`500ms` |
| `-dns` | — | 自定义 DNS 服务器，格式 `IP:端口` 或 `udp://IP:端口`，可多次指定 |

### 示例

```bash
# 发送 4 次探测
tcpping 10.0.0.1 80 -c 4

# 每 500ms 探测一次，无限发送，Ctrl+C 停止
tcpping 10.0.0.1 80 -i 500ms

# 自定义超时 2s，探测 10 次
tcpping google.com 443 -c 10 -t 2s

# 使用自定义 DNS 服务器解析域名
tcpping baidu.com 80 -c 4 -dns 8.8.8.8:53

# 指定多个 DNS 服务器，逐个尝试
tcpping baidu.com 80 -c 4 -dns 8.8.8.8:53 -dns 114.114.114.114:53
```

### 输出示例

```
TCP Ping 10.0.0.1 (端口 80)
来自 10.0.0.1:80 的应答: 时间=1.23 ms
来自 10.0.0.1:80 的应答: 时间=1.15 ms
连接 10.0.0.1:80 失败: dial tcp 10.0.0.1:80: connect: connection refused (2.00 ms)
来自 10.0.0.1:80 的应答: 时间=1.20 ms

--- TCP Ping 统计 ---
4 个包已发送，3 个包已接收，25.0% 丢包
最小/平均/最大延迟 = 1.15/1.19/1.23 ms
```

## 跨平台编译

```bash
# 当前平台
go build -o tcpping main.go

# 交叉编译所有平台
GOOS=linux   GOARCH=amd64 go build -o tcpping-linux-amd64   main.go
GOOS=linux   GOARCH=arm64 go build -o tcpping-linux-arm64   main.go
GOOS=darwin  GOARCH=amd64 go build -o tcpping-darwin-amd64  main.go
GOOS=darwin  GOARCH=arm64 go build -o tcpping-darwin-arm64  main.go
GOOS=windows GOARCH=amd64 go build -o tcpping-windows-amd64.exe main.go
GOOS=windows GOARCH=arm64 go build -o tcpping-windows-arm64.exe main.go
GOOS=freebsd GOARCH=amd64 go build -o tcpping-freebsd-amd64 main.go
GOOS=freebsd GOARCH=arm64 go build -o tcpping-freebsd-arm64 main.go
GOOS=openbsd GOARCH=amd64 go build -o tcpping-openbsd-amd64 main.go
GOOS=openbsd GOARCH=arm64 go build -o tcpping-openbsd-arm64 main.go
```
