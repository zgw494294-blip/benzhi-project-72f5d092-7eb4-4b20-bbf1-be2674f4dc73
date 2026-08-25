package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	address          string
	dataDirectory    string
	selfcheck        bool
	selfcheckTimeout time.Duration
}

func parseConfig(arguments []string) (config, error) {
	flags := flag.NewFlagSet("acoustic-annotation-release", flag.ContinueOnError)
	address := flags.String("addr", "", "监听地址，必须为 127.0.0.1:<port>")
	dataDirectory := flags.String("data-dir", "./data", "本地事件日志和快照目录")
	selfcheck := flags.Bool("selfcheck", false, "运行真实 HTTP 完整链路自检后退出")
	selfcheckTimeout := flags.Duration("selfcheck-timeout", 10*time.Second, "自检总超时")
	if err := flags.Parse(arguments); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("存在未识别的参数")
	}
	resolved, err := resolveAddress(*address, os.Getenv("PORT"))
	if err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*dataDirectory) == "" {
		return config{}, fmt.Errorf("data-dir 不能为空")
	}
	if *selfcheckTimeout <= 0 || *selfcheckTimeout > time.Minute {
		return config{}, fmt.Errorf("selfcheck-timeout 必须在 0 到 1 分钟之间")
	}
	return config{address: resolved, dataDirectory: *dataDirectory, selfcheck: *selfcheck, selfcheckTimeout: *selfcheckTimeout}, nil
}

func resolveAddress(flagAddress, environmentPort string) (string, error) {
	address := strings.TrimSpace(flagAddress)
	if address == "" {
		port := 19081
		if strings.TrimSpace(environmentPort) != "" {
			parsed, err := strconv.Atoi(environmentPort)
			if err != nil {
				return "", fmt.Errorf("PORT 必须是端口号")
			}
			port = parsed
		}
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return "", fmt.Errorf("监听地址必须为 127.0.0.1:<port>，不接受裸端口或非回环地址")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 || port == 3000 || port == 8080 {
		return "", fmt.Errorf("监听端口必须是非通用高位端口")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
