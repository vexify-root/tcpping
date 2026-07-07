package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"
)

// dnsServers 存储用户通过 -dns 提供的 DNS 服务器地址（格式：IP:端口或udp://IP:端口）
type dnsServers []string

func (d *dnsServers) String() string {
	return fmt.Sprint(*d)
}

func (d *dnsServers) Set(value string) error {
	*d = append(*d, value)
	return nil
}

// resolveHost 解析主机名到 IP 地址
// 如果自定义了 DNS 服务器，则直接使用它们进行解析；
// 否则先尝试标准解析，失败后使用默认 DNS 8.8.8.8:53
func resolveHost(host string, dnsList []string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}

	// 如果用户指定了 DNS 服务器，则使用它们
	if len(dnsList) > 0 {
		return resolveWithCustomDNS(host, dnsList)
	}

	// 未指定时，先尝试标准解析
	ips, err := net.LookupHost(host)
	if err == nil && len(ips) > 0 {
		return ips[0], nil
	}

	// 标准解析失败，使用默认 DNS 8.8.8.8:53
	fmt.Printf("标准 DNS 解析失败，使用默认 DNS (8.8.8.8:53) 解析 %s ...\n", host)
	return resolveWithCustomDNS(host, []string{"8.8.8.8:53"})
}

// resolveWithCustomDNS 使用自定义 DNS 服务器列表进行解析（纯 Go 实现，无需系统命令）
func resolveWithCustomDNS(host string, dnsList []string) (string, error) {
	// 逐个尝试 DNS 服务器
	for _, server := range dnsList {
		// 支持 udp:// 前缀，也支持直接 IP:端口
		address := server
		network := "udp"
		if strings.Contains(server, "://") {
			u, err := url.Parse(server)
			if err != nil {
				fmt.Printf("DNS 地址格式错误: %s，跳过\n", server)
				continue
			}
			network = u.Scheme // 通常为 udp
			address = u.Host
		}

		resolver := &net.Resolver{
			PreferGo: true, // 使用 Go 实现的 DNS 客户端
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, address)
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ips, err := resolver.LookupHost(ctx, host)
		cancel()
		if err == nil && len(ips) > 0 {
			if len(dnsList) > 1 {
				fmt.Printf("通过 DNS %s 解析成功\n", address)
			}
			return ips[0], nil
		}
		fmt.Printf("DNS %s 解析失败: %v\n", address, err)
	}
	return "", fmt.Errorf("所有 DNS 服务器均解析失败")
}

func main() {
	// 自定义 -dns 参数，可多次指定，格式: -dns 8.8.8.8:53 -dns 114.114.114.114:53
	var customDNS dnsServers
	flag.Var(&customDNS, "dns", "自定义 DNS 服务器，格式: IP:端口 或 udp://IP:端口 (可多次使用)")

	count := flag.Int("c", 0, "发送次数（0 表示无限）")
	interval := flag.Duration("i", time.Second, "间隔时间（如 1s, 500ms）")
	timeout := flag.Duration("t", time.Second, "连接超时（如 2s）")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("用法: tcpping <目标地址> <端口> [-c 次数] [-i 间隔] [-t 超时] [-dns DNS服务器]")
		fmt.Println("示例: tcpping baidu.com 80 -c 4 -dns 8.8.8.8:53")
		os.Exit(1)
	}
	host := args[0]
	port := args[1]

	// 检查端口是否为数字
	if _, err := fmt.Sscanf(port, "%d", new(int)); err != nil {
		fmt.Printf("错误：端口必须为数字，得到 %q\n", port)
		os.Exit(1)
	}

	// 解析主机
	ip, err := resolveHost(host, customDNS)
	if err != nil {
		fmt.Printf("解析主机 %s 失败: %v\n", host, err)
		os.Exit(1)
	}
	if ip != host {
		fmt.Printf("解析 %s → %s\n", host, ip)
	}

	target := net.JoinHostPort(ip, port)
	fmt.Printf("TCP Ping %s (端口 %s)\n", host, port)

	var (
		sent, received int
		delays         []float64
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	done := false
	for !done {
		select {
		case <-sigCh:
			done = true
		case <-ticker.C:
			if *count > 0 && sent >= *count {
				done = true
				continue
			}

			sent++
			start := time.Now()
			conn, err := net.DialTimeout("tcp", target, *timeout)
			elapsed := time.Since(start).Seconds() * 1000

			if err != nil {
				fmt.Printf("连接 %s 失败: %v (%.2f ms)\n", target, err, elapsed)
			} else {
				conn.Close()
				received++
				delays = append(delays, elapsed)
				fmt.Printf("来自 %s 的应答: 时间=%.2f ms\n", target, elapsed)
			}
		}
	}

	// 统计
	fmt.Println("\n--- TCP Ping 统计 ---")
	if sent == 0 {
		fmt.Println("未发送任何包。")
		return
	}
	fmt.Printf("%d 个包已发送，%d 个包已接收，%.1f%% 丢包\n",
		sent, received, float64(sent-received)/float64(sent)*100)
	if len(delays) > 0 {
		sort.Float64s(delays)
		min := delays[0]
		max := delays[len(delays)-1]
		sum := 0.0
		for _, d := range delays {
			sum += d
		}
		avg := sum / float64(len(delays))
		fmt.Printf("最小/平均/最大延迟 = %.2f/%.2f/%.2f ms\n", min, avg, max)
	}
}