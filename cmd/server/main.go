package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	configuration, err := parseConfig(arguments)
	if err != nil {
		return err
	}
	logger := defaultLogger()
	if configuration.selfcheck {
		return runSelfcheck(configuration, logger)
	}
	application, err := buildApplication(configuration, logger)
	if err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}
	errorsChannel := make(chan error, 1)
	go application.serve(errorsChannel)
	logger.Info("服务已启动", "address", application.listener.Addr().String(), "dataDirectory", configuration.dataDirectory)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signal := <-signals:
		logger.Info("收到关闭信号", "signal", signal.String())
	case serveError := <-errorsChannel:
		if serveError != nil {
			application.repository.Close()
			return fmt.Errorf("HTTP 服务异常退出: %w", serveError)
		}
		return application.repository.Close()
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = application.shutdown(shutdownContext); err != nil {
		return fmt.Errorf("优雅关闭失败: %w", err)
	}
	if serveError := <-errorsChannel; serveError != nil {
		return serveError
	}
	return nil
}
