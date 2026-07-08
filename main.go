package main

import (
	"context"
	"flag"
	"fmt"
	"math"
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

// percentile 返回已排序切片中第 p 百分位的值（线性插值法）
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}
	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// stddev 返回样本标准差（与 ping 的 mdev 计算方式一致）
func stddev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sumSq float64
	for _, v := range values {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)))
}

// jitter 返回 RFC 3550 中定义的 RTP 抖动值：
// 相邻样本差的绝对值经过低通滤波后的平均值，等价于相邻往返时间差的平均绝对偏差。
func jitter(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(values); i++ {
		sum += math.Abs(values[i] - values[i-1])
	}
	return sum / float64(len(values)-1)
}

func main() {
	// 自定义 -dns 参数，可多次指定，格式: -dns 8.8.8.8:53 -dns 114.114.114.114:53
	var customDNS dnsServers
	flag.Var(&customDNS, "dns", "自定义 DNS 服务器，格式: IP:端口 或 udp://IP:端口 (可多次使用)")

	count := flag.Int("c", 0, "发送次数（0 表示无限）")
	interval := flag.Duration("i", time.Second, "间隔时间（如 1s, 500ms）")
	timeout := flag.Duration("t", time.Second, "连接超时（如 2s）")
	warmup := flag.Int("w", 0, "预热次数（统计前丢弃的前若干次结果，避免冷启动影响）")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("用法: tcpping <目标地址> <端口> [-c 次数] [-i 间隔] [-t 超时] [-w 预热] [-dns DNS服务器]")
		fmt.Println("示例: tcpping baidu.com 80 -c 10 -i 500ms -w 2 -dns 8.8.8.8:53")
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
		sent, received, failed int
		delays                []float64
		totalStart            = time.Now()
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
			seq := sent
			start := time.Now()
			conn, err := net.DialTimeout("tcp", target, *timeout)
			elapsed := time.Since(start).Seconds() * 1000

			// 预热阶段只做连接，不计入统计
			if seq <= *warmup {
				if err != nil {
					fmt.Printf("[预热 %d/%d] 连接失败: %v (%.2f ms)\n", seq, *warmup, err, elapsed)
				} else {
					conn.Close()
					fmt.Printf("[预热 %d/%d] %.2f ms\n", seq, *warmup, elapsed)
				}
				continue
			}

			if err != nil {
				failed++
				fmt.Printf("来自 %s 的回复: 失败 (%.2f ms) seq=%d %v\n", target, elapsed, seq, err)
			} else {
				conn.Close()
				received++
				delays = append(delays, elapsed)
				fmt.Printf("来自 %s 的回复: 时间=%.2f ms seq=%d\n", target, elapsed, seq)
			}
		}
	}

	totalElapsed := time.Since(totalStart)

	// 统计
	fmt.Println("\n--- TCP Ping 统计 ---")
	if sent == 0 {
		fmt.Println("未发送任何包。")
		return
	}
	fmt.Printf("目标地址:        %s (端口 %s)\n", host, port)
	if ip != host {
		fmt.Printf("解析后 IP:       %s\n", ip)
	}
	fmt.Printf("总耗时:          %s\n", totalElapsed.Round(time.Millisecond))
	fmt.Printf("已发送 / 已接收 / 失败:  %d / %d / %d\n", sent, received, failed)
	if *warmup > 0 {
		fmt.Printf("(其中预热 %d 次未计入统计)\n", *warmup)
	}
	if sent > 0 {
		loss := float64(sent-received-failed) / float64(sent) * 100
		if loss < 0 {
			loss = 0
		}
		fmt.Printf("丢包率:          %.1f%%\n", loss)
	}

	if len(delays) == 0 {
		return
	}

	sort.Float64s(delays)
	min := delays[0]
	max := delays[len(delays)-1]
	var sum float64
	for _, d := range delays {
		sum += d
	}
	avg := sum / float64(len(delays))
	sd := stddev(delays, avg)
	jt := jitter(delays)
	med := percentile(delays, 50)
	p75 := percentile(delays, 75)
	p90 := percentile(delays, 90)
	p95 := percentile(delays, 95)
	p99 := percentile(delays, 99)

	fmt.Println("\n往返时间 (ms):")
	fmt.Printf("  min / avg / max / stddev = %.2f / %.2f / %.2f / %.2f\n", min, avg, max, sd)
	fmt.Printf("  median (P50) = %.2f\n", med)
	fmt.Printf("  P75 / P90 / P95 / P99    = %.2f / %.2f / %.2f / %.2f\n", p75, p90, p95, p99)
	fmt.Printf("  抖动 (Jitter)            = %.2f ms\n", jt)
}
