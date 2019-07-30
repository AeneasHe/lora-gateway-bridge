//go:generate stringer -type=PacketType

package packets

//数据包
import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// PacketType defines the packet type.
type PacketType byte

// 参见http://www.loraapp.com/lora-university-case/201701211128/
// LoRa Gateway Message Protocol
// Available packet types
const (
	PushData PacketType = iota //主要的上行数据包
	PushACK
	PullData
	PullResp //主要的下行数据包
	PullACK
	TXACK
)

//Push上行
//Pull下行
//ACK回应
//DATA数据

//PUSH_DATA PUSH_ACK：GW向NS提交上行RF数据包；
//PULL_RESP TX_ACK：NS向GW提交下行RF数据包；
//PULL_DATA PULL_ACK：GW向NS发送“心跳”以打开防火墙；

//PUSH_DATA：GW向NS发送上行RF数据包，EUI用于区分不同的GW（一个NS可以连接多个GW），tocken用于区分不同的数据包（一般为自加一）。
//PUSH_ACK：NS回应GW—成功接收该DATA数据包。
//PULL_RESP：NS向GW发送下行RF数据包，tocken用于区分不同的数据包（一般为自加一）。
//TX_ACK：GW回应NS—成功接收该RESP数据包。
//PULL_DATA：GW向NS发送“心跳”数据包，EUI用于区分不同的GW（一个NS可以连接多个GW），tocken用于区分不同的数据包（一般为自加一）。
//PULL_ACK：NS回应GW—成功接收该“心跳”数据包。

// Protocol versions
const (
	ProtocolVersion1 uint8 = 0x01
	ProtocolVersion2 uint8 = 0x02
)

// Errors
var (
	ErrInvalidProtocolVersion = errors.New("gateway: invalid protocol version")
)

// GetPacketType returns the packet type for the given packet data.
func GetPacketType(data []byte) (PacketType, error) {
	if len(data) < 4 {
		return PacketType(0), errors.New("gateway: at least 4 bytes of data are expected")
	}

	if !protocolSupported(data[0]) {
		return PacketType(0), ErrInvalidProtocolVersion
	}

	return PacketType(data[3]), nil
}

//是否支持的协议
func protocolSupported(p uint8) bool {
	if p == ProtocolVersion1 || p == ProtocolVersion2 {
		return true
	}
	return false
}

// ExpandedTime implements time.Time but (un)marshals to and from
// ISO 8601 'expanded' format.
type ExpandedTime time.Time

// MarshalJSON implements the json.Marshaler interface.
func (t ExpandedTime) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(`"2006-01-02 15:04:05 MST"`)), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *ExpandedTime) UnmarshalJSON(data []byte) error {
	t2, err := time.Parse(`"2006-01-02 15:04:05 MST"`, string(data))
	if err != nil {
		return err
	}
	*t = ExpandedTime(t2)
	return nil
}

// CompactTime implements time.Time but (un)marshals to and from
// ISO 8601 'compact' format.
type CompactTime time.Time

// MarshalJSON implements the json.Marshaler interface.
func (t CompactTime) MarshalJSON() ([]byte, error) {
	t2 := time.Time(t)
	if t2.IsZero() {
		return []byte("null"), nil
	}
	return []byte(t2.UTC().Format(`"` + time.RFC3339Nano + `"`)), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *CompactTime) UnmarshalJSON(data []byte) error {
	if string(data) == `""` {
		return nil
	}

	t2, err := time.Parse(`"`+time.RFC3339Nano+`"`, string(data))
	if err != nil {
		return err
	}
	*t = CompactTime(t2)
	return nil
}

// DatR implements the data rate which can be either a string (LoRa identifier)
// or an unsigned integer in case of FSK (bits per second).
type DatR struct {
	LoRa string
	FSK  uint32
}

// MarshalJSON implements the json.Marshaler interface.
func (d DatR) MarshalJSON() ([]byte, error) {
	if d.LoRa != "" {
		return []byte(`"` + d.LoRa + `"`), nil
	}
	return []byte(strconv.FormatUint(uint64(d.FSK), 10)), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *DatR) UnmarshalJSON(data []byte) error {
	i, err := strconv.ParseUint(string(data), 10, 32)
	if err != nil {
		d.LoRa = strings.Trim(string(data), `"`)
		return nil
	}
	d.FSK = uint32(i)
	return nil
}
