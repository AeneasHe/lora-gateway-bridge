# LoRa Gateway Bridge

[![CircleCI](https://circleci.com/gh/brocaar/lora-gateway-bridge.svg?style=svg)](https://circleci.com/gh/brocaar/lora-gateway-bridge)

LoRa Gateway Bridge is a service which converts LoRa packet-forwarder protocols
into a LoRa Server [common protocol](https://github.com/brocaar/loraserver/blob/master/api/gw/gw.proto) (JSON and Protobuf).
This project is part of the [LoRa Server](https://github.com/brocaar/loraserver)
project.

LoRa网关桥接器，桥接网关和MQTT Broker,进而连接Lora Network Server：  
* 将网关推送的上行数据包（LGMP协议）转换成通用的JSON和Protobuf数据，并转发到MQTT Broker上；  
* 同时监听MQTT Broker消息，将下行数据并将其用UDP协议发送到网关上。  

## Backends

The following packet-forwarder backends are provided:

* [Semtech UDP packet-forwarder](https://github.com/Lora-net/packet_forwarder)
* [Basic Station packet-forwarder](https://github.com/lorabasics/basicstation)

该组件主要实现LoRa网关消息协议(LoRa Gateway Message Protocol)。  
在internal/backend文件夹下面。  

## Integrations

The following integrations are provided:

* Generic MQTT broker
* [GCP Cloud IoT Core MQTT Bridge](https://cloud.google.com/iot-core/)

该组件主要实现将Backends组件解析出来的消息发送到mqtt broker上。   
在internal/integration文件夹下面。 

## Forwarder

该组件是本软件的主要入口，将Backends和Integrations组合到一起，实现网关桥接的主要功能，即将网关的消息（LGMP协议)转换成mqtt消息（MQTT协议）。  
在internal/forwarder文件夹下面。  

软件的实际启动入口:  
cmd/lora-gateway-bridge/main.go  
cmd/lora-gateway-bridge/cmd/root_run.go  

## Architecture

![architecture](https://docs.loraserver.io/img/architecture.png)

### Component links

* [LoRa Gateway Bridge](https://www.loraserver.io/lora-gateway-bridge)
* [LoRa Gateway Bridge Config](https://www.loraserver.io/lora-gateway-bridge/install/config)
* [LoRa Server](https://www.loraserver.io/loraserver)
* [LoRa App Server](https://www.loraserver.io/lora-app-server)

## Links
****
* [Downloads](https://www.loraserver.io/lora-gateway-bridge/overview/downloads/)
* [Docker image](https://hub.docker.com/r/loraserver/lora-gateway-bridge/)
* [Documentation](https://www.loraserver.io/lora-gateway-bridge/)
* [Building from source](https://www.loraserver.io/lora-gateway-bridge/community/source/)
* [Contributing](https://www.loraserver.io/lora-gateway-bridge/community/contribute/)
* Support
  * [Support forum](https://forum.loraserver.io)
  * [Bug or feature requests](https://github.com/brocaar/lora-gateway-bridge/issues)

## Sponsors

[![CableLabs](https://www.loraserver.io/img/sponsors/cablelabs.png)](https://www.cablelabs.com/)
[![SIDNFonds](https://www.loraserver.io/img/sponsors/sidn_fonds.png)](https://www.sidnfonds.nl/)
[![acklio](https://www.loraserver.io/img/sponsors/acklio.png)](http://www.ackl.io/)

## License

LoRa Gateway Bridge is distributed under the MIT license. See 
[LICENSE](https://github.com/brocaar/lora-gateway-bridge/blob/master/LICENSE).
