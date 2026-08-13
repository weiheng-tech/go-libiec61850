package iec61850

import "net"

// GooseMessage 是一次经过libIEC61850校验和解码的GOOSE报文快照。
type GooseMessage struct {
	GoCBRef           string
	GoID              string
	DataSetRef        string
	APPID             uint16
	SourceMAC         net.HardwareAddr
	DestinationMAC    net.HardwareAddr
	StNum             uint32
	SqNum             uint32
	ConfRev           uint32
	TimeAllowedToLive uint32
	Timestamp         uint64
	Valid             bool
	Test              bool
	NeedsCommission   bool
	VLANSet           bool
	VLANID            uint16
	VLANPriority      uint8
	ParseError        int
	Values            []GoMmsValue
}

type GooseHandler func(GooseMessage)

type GooseSubscription struct {
	GoCBRef     string
	APPID       uint16
	Destination net.HardwareAddr
}

type GoosePublisherConfig struct {
	InterfaceName     string
	Destination       net.HardwareAddr
	APPID             uint16
	VLANSet           bool
	VLANID            uint16
	VLANPriority      uint8
	GoCBRef           string
	GoID              string
	DataSetRef        string
	ConfRev           uint32
	TimeAllowedToLive uint32
}
