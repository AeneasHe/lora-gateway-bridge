package semtechudp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-gateway-bridge/internal/backend/semtechudp/packets"
	"github.com/brocaar/lora-gateway-bridge/internal/config"
	"github.com/brocaar/lora-gateway-bridge/internal/filters"
	"github.com/brocaar/loraserver/api/gw" //loraserver的gateway api
	"github.com/brocaar/lorawan"           //lorawan协议
)

// udpPacket represents a raw UDP packet.
type udpPacket struct {
	addr *net.UDPAddr
	data []byte
}

// packet-forwarder配置
type pfConfiguration struct {
	gatewayID      lorawan.EUI64
	baseFile       string
	outputFile     string
	restartCommand string
	currentVersion string
}

// Backend implements a Semtech packet-forwarder (UDP) gateway backend.
type Backend struct {
	sync.RWMutex

	downlinkTXAckChan chan gw.DownlinkTXAck // 下传指令
	uplinkFrameChan   chan gw.UplinkFrame   // 上传数据帧
	gatewayStatsChan  chan gw.GatewayStats  // 网关状态
	udpSendChan       chan udpPacket        // udp数据包通道

	wg             sync.WaitGroup // 通道阻塞
	conn           *net.UDPConn   // udp连接
	closed         bool           // 关闭
	gateways       gateways       // 网关实例
	fakeRxTime     bool
	configurations []pfConfiguration
	skipCRCCheck   bool
}

// NewBackend creates a new backend.
func NewBackend(conf config.Config) (*Backend, error) {
	addr, err := net.ResolveUDPAddr("udp", conf.Backend.SemtechUDP.UDPBind)
	if err != nil {
		return nil, errors.Wrap(err, "resolve udp addr error")
	}

	log.WithField("addr", addr).Info("backend/semtechudp: starting gateway udp listener")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "listen udp error")
	}

	//实例化后端
	b := &Backend{
		conn:              conn,
		downlinkTXAckChan: make(chan gw.DownlinkTXAck),
		uplinkFrameChan:   make(chan gw.UplinkFrame),
		gatewayStatsChan:  make(chan gw.GatewayStats),
		udpSendChan:       make(chan udpPacket),
		gateways: gateways{
			gateways:       make(map[lorawan.EUI64]gateway),
			connectChan:    make(chan lorawan.EUI64),
			disconnectChan: make(chan lorawan.EUI64),
		},
		fakeRxTime:   conf.Backend.SemtechUDP.FakeRxTime,
		skipCRCCheck: conf.Backend.SemtechUDP.SkipCRCCheck,
	}

	//配置pf
	for _, pfConf := range conf.Backend.SemtechUDP.Configuration {
		c := pfConfiguration{
			baseFile:       pfConf.BaseFile,
			outputFile:     pfConf.OutputFile,
			restartCommand: pfConf.RestartCommand,
		}
		if err := c.gatewayID.UnmarshalText([]byte(pfConf.GatewayID)); err != nil {
			return nil, errors.Wrap(err, "unmarshal gateway id error")
		}
		b.configurations = append(b.configurations, c)
	}

	//清除旧配置
	go func() {
		for {
			log.Debug("backend/semtechudp: cleanup gateway registry")
			if err := b.gateways.cleanup(); err != nil {
				log.WithError(err).Error("backend/semtechudp: gateway registry cleanup failed")
			}
			time.Sleep(time.Minute)
		}
	}()

	//循环读取数据包
	go func() {
		b.wg.Add(1)
		err := b.readPackets()
		if !b.isClosed() {
			log.WithError(err).Error("backend/semtechudp: read udp packets error")
		}
		b.wg.Done()
	}()

	//循环发送数据包
	go func() {
		b.wg.Add(1)
		err := b.sendPackets()
		if !b.isClosed() {
			log.WithError(err).Error("backend/semtechudp: send udp packets error")
		}
		b.wg.Done()
	}()

	return b, nil
}

// Close closes the backend.
func (b *Backend) Close() error {
	b.Lock()
	b.closed = true

	log.Info("backend/semtechudp: closing gateway backend")

	//关闭udp连接
	if err := b.conn.Close(); err != nil {
		return errors.Wrap(err, "close udp listener error")
	}

	//处理最后的数据包
	log.Info("backend/semtechudp: handling last packets")
	close(b.udpSendChan) //关闭udp发送
	b.Unlock()
	b.wg.Wait()
	return nil
}

// GetDownlinkTXAckChan returns the downlink tx ack channel.
func (b *Backend) GetDownlinkTXAckChan() chan gw.DownlinkTXAck {
	return b.downlinkTXAckChan
}

// GetGatewayStatsChan returns the gateway stats channel.
func (b *Backend) GetGatewayStatsChan() chan gw.GatewayStats {
	return b.gatewayStatsChan
}

// GetUplinkFrameChan returns the uplink frame channel.
func (b *Backend) GetUplinkFrameChan() chan gw.UplinkFrame {
	return b.uplinkFrameChan
}

// GetConnectChan returns the channel for received gateway connections.
func (b *Backend) GetConnectChan() chan lorawan.EUI64 {
	return b.gateways.connectChan
}

// GetDisconnectChan returns the channel for disconnected gateway connections.
func (b *Backend) GetDisconnectChan() chan lorawan.EUI64 {
	return b.gateways.disconnectChan
}

// 发送下传数据帧
// SendDownlinkFrame sends the given downlink frame to the gateway.
func (b *Backend) SendDownlinkFrame(frame gw.DownlinkFrame) error {
	var gatewayID lorawan.EUI64
	copy(gatewayID[:], frame.TxInfo.GatewayId)

	//获取网关
	gw, err := b.gateways.get(gatewayID)
	if err != nil {
		return errors.Wrap(err, "get gateway error")
	}

	//拉取响应
	pullResp, err := packets.GetPullRespPacket(gw.protocolVersion, uint16(frame.Token), frame)
	if err != nil {
		return errors.Wrap(err, "get PullRespPacket error")
	}

	bytes, err := pullResp.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "backend/semtechudp: marshal PullRespPacket error")
	}

	//udp发送响应
	b.udpSendChan <- udpPacket{
		data: bytes,   //数据内容
		addr: gw.addr, //网关地址
	}
	return nil
}

// ApplyConfiguration applies the given configuration to the gateway
// (packet-forwarder).
func (b *Backend) ApplyConfiguration(config gw.GatewayConfiguration) error {
	var gatewayID lorawan.EUI64
	copy(gatewayID[:], config.GatewayId)

	b.Lock()
	var pfConfig *pfConfiguration
	for i := range b.configurations {
		if b.configurations[i].gatewayID == gatewayID {
			pfConfig = &b.configurations[i]
		}
	}
	b.Unlock()

	if pfConfig == nil {
		return nil
	}

	return b.applyConfiguration(*pfConfig, config)
}

func (b *Backend) applyConfiguration(pfConfig pfConfiguration, config gw.GatewayConfiguration) error {
	gwConfig, err := getGatewayConfig(config)
	if err != nil {
		return errors.Wrap(err, "get gateway config error")
	}

	baseConfig, err := loadConfigFile(pfConfig.baseFile)
	if err != nil {
		return errors.Wrap(err, "load config file error")
	}

	if err = mergeConfig(pfConfig.gatewayID, baseConfig, gwConfig); err != nil {
		return errors.Wrap(err, "merge config error")
	}

	// generate config json
	bb, err := json.Marshal(baseConfig)
	if err != nil {
		return errors.Wrap(err, "marshal json error")
	}

	// write new config file to disk
	if err = ioutil.WriteFile(pfConfig.outputFile, bb, 0644); err != nil {
		return errors.Wrap(err, "write config file error")
	}
	log.WithFields(log.Fields{
		"gateway_id": pfConfig.gatewayID,
		"file":       pfConfig.outputFile,
	}).Info("backend/semtechudp: new configuration file written")

	// invoke restart command
	if err = invokePFRestart(pfConfig.restartCommand); err != nil {
		return errors.Wrap(err, "invoke packet-forwarder restart error")
	}
	log.WithFields(log.Fields{
		"gateway_id": pfConfig.gatewayID,
		"cmd":        pfConfig.restartCommand,
	}).Info("backend/semtechudp: packet-forwarder restart command invoked")

	b.Lock()
	defer b.Unlock()

	for i := range b.configurations {
		if b.configurations[i].gatewayID == pfConfig.gatewayID {
			b.configurations[i].currentVersion = config.Version
		}
	}

	return nil
}

func (b *Backend) isClosed() bool {
	b.RLock()
	defer b.RUnlock()
	return b.closed
}

// 读取数据包
func (b *Backend) readPackets() error {
	buf := make([]byte, 65507) // max udp data size
	// 循环读取udp连接的buf
	for {
		i, addr, err := b.conn.ReadFromUDP(buf)
		if err != nil {
			if b.isClosed() {
				return nil
			}

			log.WithError(err).Error("gateway: read from udp error")
			continue
		}
		data := make([]byte, i)
		copy(data, buf[:i])
		// 生成udp数据包
		up := udpPacket{data: data, addr: addr}

		// handle packet async
		// 异步处理数据包
		go func(up udpPacket) {
			if err := b.handlePacket(up); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"data_base64": base64.StdEncoding.EncodeToString(up.data),
					"addr":        up.addr,
				}).Error("backend/semtechudp: could not handle packet")
			}
		}(up)
	}
}

//发送数据包
func (b *Backend) sendPackets() error {
	//从数据包通道循环读取数据并发送
	for p := range b.udpSendChan {
		//数据包类型
		pt, err := packets.GetPacketType(p.data)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"addr":        p.addr,
				"data_base64": base64.StdEncoding.EncodeToString(p.data),
			}).Error("backend/semtechudp: get packet-type error")
			continue
		}

		log.WithFields(log.Fields{
			"addr":             p.addr,
			"type":             pt,
			"protocol_version": p.data[0],
		}).Debug("backend/semtechudp: sending udp packet to gateway")

		//向udp连接写入数据包
		_, err = b.conn.WriteToUDP(p.data, p.addr)
		if err != nil {
			log.WithFields(log.Fields{
				"addr":             p.addr,
				"type":             pt,
				"protocol_version": p.data[0],
			}).WithError(err).Error("backend/semtechudp: write to udp error")
		}
		//udp写入计数器计数
		udpWriteCounter(pt.String()).Inc()
	}
	return nil
}

//处理数据包
func (b *Backend) handlePacket(up udpPacket) error {
	//取锁
	b.RLock()
	defer b.RUnlock()

	if b.closed {
		return nil
	}

	//获取数据包类型
	pt, err := packets.GetPacketType(up.data)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"addr":             up.addr,
		"type":             pt,
		"protocol_version": up.data[0],
	}).Debug("backend/semtechudp: received udp packet from gateway")

	//udp读取计数器计数
	udpReadCounter(pt.String()).Inc()

	//根据数据包类型做相应处理
	switch pt {
	case packets.PushData:
		return b.handlePushData(up) //推送数据
	case packets.PullData:
		return b.handlePullData(up) //拉取数据
	case packets.TXACK:
		return b.handleTXACK(up)
	default:
		return fmt.Errorf("backend/semtechudp: unknown packet type: %s", pt)
	}
}

func (b *Backend) handlePullData(up udpPacket) error {
	var p packets.PullDataPacket
	if err := p.UnmarshalBinary(up.data); err != nil {
		return err
	}
	ack := packets.PullACKPacket{
		ProtocolVersion: p.ProtocolVersion,
		RandomToken:     p.RandomToken,
	}
	bytes, err := ack.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal pull ack packet error")
	}

	err = b.gateways.set(p.GatewayMAC, gateway{
		addr:            up.addr,
		lastSeen:        time.Now().UTC(),
		protocolVersion: p.ProtocolVersion,
	})
	if err != nil {
		return errors.Wrap(err, "set gateway error")
	}

	b.udpSendChan <- udpPacket{
		addr: up.addr,
		data: bytes,
	}
	return nil
}

func (b *Backend) handleTXACK(up udpPacket) error {
	var p packets.TXACKPacket
	if err := p.UnmarshalBinary(up.data); err != nil {
		return err
	}

	if p.Payload != nil && p.Payload.TXPKACK.Error != "" && p.Payload.TXPKACK.Error != "NONE" {
		b.downlinkTXAckChan <- gw.DownlinkTXAck{
			GatewayId: p.GatewayMAC[:],
			Token:     uint32(p.RandomToken),
			Error:     p.Payload.TXPKACK.Error,
		}
	} else {
		b.downlinkTXAckChan <- gw.DownlinkTXAck{
			GatewayId: p.GatewayMAC[:],
			Token:     uint32(p.RandomToken),
		}
	}

	return nil
}

//处理数据推送，指将从网关接收的上传数据推送到mqtt broker
func (b *Backend) handlePushData(up udpPacket) error {
	var p packets.PushDataPacket
	if err := p.UnmarshalBinary(up.data); err != nil {
		return err
	}

	// ack the packet
	ack := packets.PushACKPacket{
		ProtocolVersion: p.ProtocolVersion,
		RandomToken:     p.RandomToken,
	}
	bytes, err := ack.MarshalBinary()
	if err != nil {
		return err
	}
	b.udpSendChan <- udpPacket{
		addr: up.addr,
		data: bytes,
	}

	// gateway stats
	// 获取网关状态
	stats, err := p.GetGatewayStats()
	if err != nil {
		return errors.Wrap(err, "get stats error")
	}
	if stats != nil {
		// set gateway ip
		if up.addr.IP.IsLoopback() {
			ip, err := getOutboundIP()
			if err != nil {
				log.WithError(err).Error("backend/semtechudp: get outbound ip error")
			} else {
				stats.Ip = ip.String()
			}
		} else {
			stats.Ip = up.addr.IP.String()
		}

		b.handleStats(p.GatewayMAC, *stats)
	}

	// uplink frames
	// 获取上传数据帧
	uplinkFrames, err := p.GetUplinkFrames(b.skipCRCCheck, b.fakeRxTime)
	if err != nil {
		return errors.Wrap(err, "get uplink frames error")
	}
	// 处理上传数据帧
	b.handleUplinkFrames(uplinkFrames)

	return nil
}

func (b *Backend) handleStats(gatewayID lorawan.EUI64, stats gw.GatewayStats) {
	// set configuration version, if available

	for _, c := range b.configurations {
		if gatewayID == c.gatewayID {
			stats.ConfigVersion = c.currentVersion
		}
	}

	b.gatewayStatsChan <- stats
}

//处理上传数据帧
func (b *Backend) handleUplinkFrames(uplinkFrames []gw.UplinkFrame) error {
	for i := range uplinkFrames {
		if filters.MatchFilters(uplinkFrames[i].PhyPayload) {
			b.uplinkFrameChan <- uplinkFrames[i] //向上传数据帧通道写入数据帧即可
		} else {
			log.WithFields(log.Fields{
				"data_base64": base64.StdEncoding.EncodeToString(uplinkFrames[i].PhyPayload),
			}).Debug("backend/semtechudp: frame dropped because of configured filters")
		}
	}

	return nil
}

func getOutboundIP() (net.IP, error) {
	// this does not actually connect to 8.8.8.8, unless the connection is
	// used to send UDP frames
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}
