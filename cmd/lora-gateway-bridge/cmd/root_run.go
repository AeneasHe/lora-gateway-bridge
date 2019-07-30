package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/brocaar/lora-gateway-bridge/internal/backend"
	"github.com/brocaar/lora-gateway-bridge/internal/config"
	"github.com/brocaar/lora-gateway-bridge/internal/filters"
	"github.com/brocaar/lora-gateway-bridge/internal/forwarder"
	"github.com/brocaar/lora-gateway-bridge/internal/integration"
	"github.com/brocaar/lora-gateway-bridge/internal/metadata"
	"github.com/brocaar/lora-gateway-bridge/internal/metrics"
)

//程序入口
func run(cmd *cobra.Command, args []string) error {

	//任务切片
	tasks := []func() error{
		setLogLevel,
		printStartMessage,
		setupFilters,
		setupBackend,
		setupIntegration,
		setupForwarder,
		setupMetrics,
		setupMetaData,
	}

	//启动任务
	for _, t := range tasks {
		if err := t(); err != nil {
			log.Fatal(err)
		}
	}

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)      //监听信号
	log.WithField("signal", <-sigChan).Info("signal received") //取出信号
	log.Warning("shutting down server")

	return nil
}

//配置日志
func setLogLevel() error {
	log.SetLevel(log.Level(uint8(config.C.General.LogLevel)))
	return nil
}

//打印开始信息
func printStartMessage() error {
	log.WithFields(log.Fields{
		"version": version,
		"docs":    "https://www.loraserver.io/lora-gateway-bridge/",
	}).Info("starting LoRa Gateway Bridge")
	return nil
}

//设置后端（主要的程序）
//packet-forwarder后端选择semtech_udp 或 basic_station
func setupBackend() error {
	if err := backend.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup backend error")
	}
	return nil
}

//设置集成（主要指集成mqtt，把数据按mqtt协议发布）
func setupIntegration() error {
	if err := integration.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup integration error")
	}
	return nil
}

//设置转发 （程序顶部入口，调用Backend和Integration实现程序主要功能，将lora网关消息转发到mqtt代理上）
func setupForwarder() error {
	if err := forwarder.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup forwarder error")
	}
	return nil
}

//设置监控指标（主要是监控软件运行状态）
func setupMetrics() error {
	if err := metrics.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup metrics error")
	}
	return nil
}

//设置元数据
func setupMetaData() error {
	if err := metadata.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup meta-data error")
	}
	return nil
}

//设置过滤器（用于过滤LoraWan数据帧,只转发有效数据帧）
func setupFilters() error {
	if err := filters.Setup(config.C); err != nil {
		return errors.Wrap(err, "setup filters error")
	}
	return nil
}
