package integration

import (
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/brocaar/lora-gateway-bridge/internal/config"
	"github.com/brocaar/lora-gateway-bridge/internal/integration/mqtt"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
)

// Event types.
const (
	EventUp    = "up"    //上行数据
	EventStats = "stats" //网关状态
	EventAck   = "ack"   //回应数据
)

var integration Integration

func Setup(conf config.Config) error {
	var err error
	integration, err = mqtt.NewBackend(conf)
	if err != nil {
		return errors.Wrap(err, "setup mqtt integration error")
	}

	return nil
}

// GetIntegration returns the integration.
func GetIntegration() Integration {
	return integration
}

//集成接口，mqtt/backend实现该接口
type Integration interface {
	//监听
	// SubscribeGateway creates a subscription for the given gateway ID.
	SubscribeGateway(lorawan.EUI64) error

	//取消监听
	// UnsubscribeGateway removes the subscription for the given gateway ID.
	UnsubscribeGateway(lorawan.EUI64) error

	//发布
	// PublishEvent publishes the given event.
	PublishEvent(lorawan.EUI64, string, proto.Message) error

	//获取下行数据帧通道
	// GetDownlinkFrameChan returns the channel for downlink frames.
	GetDownlinkFrameChan() chan gw.DownlinkFrame

	//获取网关的配置通道
	// GetGatewayConfigurationChan returns the channel for gateway configuration.
	GetGatewayConfigurationChan() chan gw.GatewayConfiguration

	// Close closes the integration.
	Close() error
}
