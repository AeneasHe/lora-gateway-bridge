package forwarder

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-gateway-bridge/internal/backend"
	"github.com/brocaar/lora-gateway-bridge/internal/config"
	"github.com/brocaar/lora-gateway-bridge/internal/integration"
	"github.com/brocaar/lora-gateway-bridge/internal/metadata"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

var alwaysSubscribe []lorawan.EUI64

func Setup(conf config.Config) error {
	b := backend.GetBackend()
	i := integration.GetIntegration()

	if b == nil {
		return errors.New("backend is not set")
	}

	if i == nil {
		return errors.New("integration is not set")
	}

	for _, c := range conf.Backend.SemtechUDP.Configuration {
		var gatewayID lorawan.EUI64
		if err := gatewayID.UnmarshalText([]byte(c.GatewayID)); err != nil {
			return errors.Wrap(err, "unmarshal gateway_id error")
		}

		if err := i.SubscribeGateway(gatewayID); err != nil {
			return errors.Wrap(err, "subscribe gateway error")
		}

		alwaysSubscribe = append(alwaysSubscribe, gatewayID)
	}

	go onConnectedLoop()    //连接事件循环
	go onDisconnectedLoop() //取消连接事件循环

	go forwardUplinkFrameLoop()
	go forwardGatewayStatsLoop()
	go forwardDownlinkTxAckLoop()
	go forwardDownlinkFrameLoop()
	go forwardGatewayConfigurationLoop()

	return nil
}

// 连接事件循环：主要是监听网关
func onConnectedLoop() {
	for gatewayID := range backend.GetBackend().GetConnectChan() {
		var found bool
		for _, gwID := range alwaysSubscribe {
			if gatewayID == gwID {
				found = true
			}
		}
		if found {
			break
		}

		if err := integration.GetIntegration().SubscribeGateway(gatewayID); err != nil {
			log.WithError(err).Error("subscribe gateway error")
		}
	}
}

// 取消连接事件循环：主要是取消监听网关
func onDisconnectedLoop() {
	for gatewayID := range backend.GetBackend().GetDisconnectChan() {
		var found bool
		for _, gwID := range alwaysSubscribe {
			if gatewayID == gwID {
				found = true
			}
		}
		if found {
			break
		}

		if err := integration.GetIntegration().UnsubscribeGateway(gatewayID); err != nil {
			log.WithError(err).Error("unsubscribe gateway error")
		}
	}
}

// 推送上行数据帧循环
func forwardUplinkFrameLoop() {
	//循环从上行数据帧通道取数据
	for uplinkFrame := range backend.GetBackend().GetUplinkFrameChan() {
		go func(uplinkFrame gw.UplinkFrame) {
			//获取网关id
			var gatewayID lorawan.EUI64
			copy(gatewayID[:], uplinkFrame.RxInfo.GatewayId)

			//向mqtt发布数据帧消息
			if err := integration.GetIntegration().PublishEvent(gatewayID, integration.EventUp, &uplinkFrame); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"gateway_id": gatewayID,
					"event_type": integration.EventUp,
				}).Error("publish event error")
			}
		}(uplinkFrame)
	}
}

// 推送网关状态循环
func forwardGatewayStatsLoop() {
	for stats := range backend.GetBackend().GetGatewayStatsChan() {
		go func(stats gw.GatewayStats) {
			var gatewayID lorawan.EUI64
			copy(gatewayID[:], stats.GatewayId)

			// add meta-data to stats
			stats.MetaData = metadata.Get()

			if err := integration.GetIntegration().PublishEvent(gatewayID, integration.EventStats, &stats); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"gateway_id": gatewayID,
					"event_type": integration.EventStats,
				}).Error("publish event error")
			}
		}(stats)
	}
}

// 推送下行指令的回应循环
func forwardDownlinkTxAckLoop() {
	for txAck := range backend.GetBackend().GetDownlinkTXAckChan() {
		go func(txAck gw.DownlinkTXAck) {
			var gatewayID lorawan.EUI64
			copy(gatewayID[:], txAck.GatewayId)

			if err := integration.GetIntegration().PublishEvent(gatewayID, integration.EventAck, &txAck); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"gateway_id": gatewayID,
					"event_type": integration.EventAck,
				}).Error("publish event error")
			}
		}(txAck)
	}
}

// 推送下行指令数据帧
func forwardDownlinkFrameLoop() {
	for downlinkFrame := range integration.GetIntegration().GetDownlinkFrameChan() {
		go func(downlinkFrame gw.DownlinkFrame) {
			// 调用udp的发送下行数据帧指令
			if err := backend.GetBackend().SendDownlinkFrame(downlinkFrame); err != nil {
				log.WithError(err).Error("send downlink frame error")
			}
		}(downlinkFrame)
	}
}

// 推送网关配置
func forwardGatewayConfigurationLoop() {
	for gatewayConfig := range integration.GetIntegration().GetGatewayConfigurationChan() {
		go func(gatewayConfig gw.GatewayConfiguration) {
			// 调用udp的网关配置命令，配置网关
			if err := backend.GetBackend().ApplyConfiguration(gatewayConfig); err != nil {
				log.WithError(err).Error("apply gateway-configuration error")
			}
		}(gatewayConfig)
	}
}
