// Code generated by protoc-gen-go. DO NOT EDIT.
// source: ports/port_msg.proto

package bess_pb

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type PCAPPortArg struct {
	Dev                  string   `protobuf:"bytes,1,opt,name=dev,proto3" json:"dev,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PCAPPortArg) Reset()         { *m = PCAPPortArg{} }
func (m *PCAPPortArg) String() string { return proto.CompactTextString(m) }
func (*PCAPPortArg) ProtoMessage()    {}
func (*PCAPPortArg) Descriptor() ([]byte, []int) {
	return fileDescriptor_15ffb2279a2b5904, []int{0}
}

func (m *PCAPPortArg) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PCAPPortArg.Unmarshal(m, b)
}
func (m *PCAPPortArg) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PCAPPortArg.Marshal(b, m, deterministic)
}
func (m *PCAPPortArg) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PCAPPortArg.Merge(m, src)
}
func (m *PCAPPortArg) XXX_Size() int {
	return xxx_messageInfo_PCAPPortArg.Size(m)
}
func (m *PCAPPortArg) XXX_DiscardUnknown() {
	xxx_messageInfo_PCAPPortArg.DiscardUnknown(m)
}

var xxx_messageInfo_PCAPPortArg proto.InternalMessageInfo

func (m *PCAPPortArg) GetDev() string {
	if m != nil {
		return m.Dev
	}
	return ""
}

type PMDPortArg struct {
	Loopback bool `protobuf:"varint,1,opt,name=loopback,proto3" json:"loopback,omitempty"`
	// Types that are valid to be assigned to Port:
	//	*PMDPortArg_PortId
	//	*PMDPortArg_Pci
	//	*PMDPortArg_Vdev
	Port isPMDPortArg_Port `protobuf_oneof:"port"`
	// See http://dpdk.org/doc/dts/test_plans/dual_vlan_test_plan.html
	VlanOffloadRxStrip  bool `protobuf:"varint,5,opt,name=vlan_offload_rx_strip,json=vlanOffloadRxStrip,proto3" json:"vlan_offload_rx_strip,omitempty"`
	VlanOffloadRxFilter bool `protobuf:"varint,6,opt,name=vlan_offload_rx_filter,json=vlanOffloadRxFilter,proto3" json:"vlan_offload_rx_filter,omitempty"`
	VlanOffloadRxQinq   bool `protobuf:"varint,7,opt,name=vlan_offload_rx_qinq,json=vlanOffloadRxQinq,proto3" json:"vlan_offload_rx_qinq,omitempty"`
	// Types that are valid to be assigned to Socket:
	//	*PMDPortArg_SocketId
	Socket               isPMDPortArg_Socket `protobuf_oneof:"socket"`
	PromiscuousMode      bool                `protobuf:"varint,9,opt,name=promiscuous_mode,json=promiscuousMode,proto3" json:"promiscuous_mode,omitempty"`
	Hwcksum              bool                `protobuf:"varint,10,opt,name=hwcksum,proto3" json:"hwcksum,omitempty"`
	XXX_NoUnkeyedLiteral struct{}            `json:"-"`
	XXX_unrecognized     []byte              `json:"-"`
	XXX_sizecache        int32               `json:"-"`
}

func (m *PMDPortArg) Reset()         { *m = PMDPortArg{} }
func (m *PMDPortArg) String() string { return proto.CompactTextString(m) }
func (*PMDPortArg) ProtoMessage()    {}
func (*PMDPortArg) Descriptor() ([]byte, []int) {
	return fileDescriptor_15ffb2279a2b5904, []int{1}
}

func (m *PMDPortArg) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PMDPortArg.Unmarshal(m, b)
}
func (m *PMDPortArg) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PMDPortArg.Marshal(b, m, deterministic)
}
func (m *PMDPortArg) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PMDPortArg.Merge(m, src)
}
func (m *PMDPortArg) XXX_Size() int {
	return xxx_messageInfo_PMDPortArg.Size(m)
}
func (m *PMDPortArg) XXX_DiscardUnknown() {
	xxx_messageInfo_PMDPortArg.DiscardUnknown(m)
}

var xxx_messageInfo_PMDPortArg proto.InternalMessageInfo

func (m *PMDPortArg) GetLoopback() bool {
	if m != nil {
		return m.Loopback
	}
	return false
}

type isPMDPortArg_Port interface {
	isPMDPortArg_Port()
}

type PMDPortArg_PortId struct {
	PortId uint64 `protobuf:"varint,2,opt,name=port_id,json=portId,proto3,oneof"`
}

type PMDPortArg_Pci struct {
	Pci string `protobuf:"bytes,3,opt,name=pci,proto3,oneof"`
}

type PMDPortArg_Vdev struct {
	Vdev string `protobuf:"bytes,4,opt,name=vdev,proto3,oneof"`
}

func (*PMDPortArg_PortId) isPMDPortArg_Port() {}

func (*PMDPortArg_Pci) isPMDPortArg_Port() {}

func (*PMDPortArg_Vdev) isPMDPortArg_Port() {}

func (m *PMDPortArg) GetPort() isPMDPortArg_Port {
	if m != nil {
		return m.Port
	}
	return nil
}

func (m *PMDPortArg) GetPortId() uint64 {
	if x, ok := m.GetPort().(*PMDPortArg_PortId); ok {
		return x.PortId
	}
	return 0
}

func (m *PMDPortArg) GetPci() string {
	if x, ok := m.GetPort().(*PMDPortArg_Pci); ok {
		return x.Pci
	}
	return ""
}

func (m *PMDPortArg) GetVdev() string {
	if x, ok := m.GetPort().(*PMDPortArg_Vdev); ok {
		return x.Vdev
	}
	return ""
}

func (m *PMDPortArg) GetVlanOffloadRxStrip() bool {
	if m != nil {
		return m.VlanOffloadRxStrip
	}
	return false
}

func (m *PMDPortArg) GetVlanOffloadRxFilter() bool {
	if m != nil {
		return m.VlanOffloadRxFilter
	}
	return false
}

func (m *PMDPortArg) GetVlanOffloadRxQinq() bool {
	if m != nil {
		return m.VlanOffloadRxQinq
	}
	return false
}

type isPMDPortArg_Socket interface {
	isPMDPortArg_Socket()
}

type PMDPortArg_SocketId struct {
	SocketId int32 `protobuf:"varint,8,opt,name=socket_id,json=socketId,proto3,oneof"`
}

func (*PMDPortArg_SocketId) isPMDPortArg_Socket() {}

func (m *PMDPortArg) GetSocket() isPMDPortArg_Socket {
	if m != nil {
		return m.Socket
	}
	return nil
}

func (m *PMDPortArg) GetSocketId() int32 {
	if x, ok := m.GetSocket().(*PMDPortArg_SocketId); ok {
		return x.SocketId
	}
	return 0
}

func (m *PMDPortArg) GetPromiscuousMode() bool {
	if m != nil {
		return m.PromiscuousMode
	}
	return false
}

func (m *PMDPortArg) GetHwcksum() bool {
	if m != nil {
		return m.Hwcksum
	}
	return false
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*PMDPortArg) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*PMDPortArg_PortId)(nil),
		(*PMDPortArg_Pci)(nil),
		(*PMDPortArg_Vdev)(nil),
		(*PMDPortArg_SocketId)(nil),
	}
}

type UnixSocketPortArg struct {
	/// Set the first character to "@" in place of \0 for abstract path
	/// See manpage for unix(7).
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	/// Minimum RX polling interval for system calls, when *idle*.
	/// Use a negative number for unthrottled polling. If unspecified or 0,
	/// it is set to 50,000 (50 microseconds, or 20k polls per second)
	MinRxIntervalNs int64 `protobuf:"varint,2,opt,name=min_rx_interval_ns,json=minRxIntervalNs,proto3" json:"min_rx_interval_ns,omitempty"`
	/// If set, the port driver will send a confirmation once
	/// the port is connected.  This lets pybess avoid a race during
	/// testing.  See bessctl/test_utils.py for details.
	ConfirmConnect       bool     `protobuf:"varint,3,opt,name=confirm_connect,json=confirmConnect,proto3" json:"confirm_connect,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UnixSocketPortArg) Reset()         { *m = UnixSocketPortArg{} }
func (m *UnixSocketPortArg) String() string { return proto.CompactTextString(m) }
func (*UnixSocketPortArg) ProtoMessage()    {}
func (*UnixSocketPortArg) Descriptor() ([]byte, []int) {
	return fileDescriptor_15ffb2279a2b5904, []int{2}
}

func (m *UnixSocketPortArg) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UnixSocketPortArg.Unmarshal(m, b)
}
func (m *UnixSocketPortArg) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UnixSocketPortArg.Marshal(b, m, deterministic)
}
func (m *UnixSocketPortArg) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UnixSocketPortArg.Merge(m, src)
}
func (m *UnixSocketPortArg) XXX_Size() int {
	return xxx_messageInfo_UnixSocketPortArg.Size(m)
}
func (m *UnixSocketPortArg) XXX_DiscardUnknown() {
	xxx_messageInfo_UnixSocketPortArg.DiscardUnknown(m)
}

var xxx_messageInfo_UnixSocketPortArg proto.InternalMessageInfo

func (m *UnixSocketPortArg) GetPath() string {
	if m != nil {
		return m.Path
	}
	return ""
}

func (m *UnixSocketPortArg) GetMinRxIntervalNs() int64 {
	if m != nil {
		return m.MinRxIntervalNs
	}
	return 0
}

func (m *UnixSocketPortArg) GetConfirmConnect() bool {
	if m != nil {
		return m.ConfirmConnect
	}
	return false
}

type VPortArg struct {
	Ifname string `protobuf:"bytes,1,opt,name=ifname,proto3" json:"ifname,omitempty"`
	// Types that are valid to be assigned to Cpid:
	//	*VPortArg_Docker
	//	*VPortArg_ContainerPid
	//	*VPortArg_Netns
	Cpid                 isVPortArg_Cpid `protobuf_oneof:"cpid"`
	RxqCpus              []int64         `protobuf:"varint,5,rep,packed,name=rxq_cpus,json=rxqCpus,proto3" json:"rxq_cpus,omitempty"`
	TxTci                uint64          `protobuf:"varint,6,opt,name=tx_tci,json=txTci,proto3" json:"tx_tci,omitempty"`
	TxOuterTci           uint64          `protobuf:"varint,7,opt,name=tx_outer_tci,json=txOuterTci,proto3" json:"tx_outer_tci,omitempty"`
	Loopback             bool            `protobuf:"varint,8,opt,name=loopback,proto3" json:"loopback,omitempty"`
	IpAddrs              []string        `protobuf:"bytes,9,rep,name=ip_addrs,json=ipAddrs,proto3" json:"ip_addrs,omitempty"`
	XXX_NoUnkeyedLiteral struct{}        `json:"-"`
	XXX_unrecognized     []byte          `json:"-"`
	XXX_sizecache        int32           `json:"-"`
}

func (m *VPortArg) Reset()         { *m = VPortArg{} }
func (m *VPortArg) String() string { return proto.CompactTextString(m) }
func (*VPortArg) ProtoMessage()    {}
func (*VPortArg) Descriptor() ([]byte, []int) {
	return fileDescriptor_15ffb2279a2b5904, []int{3}
}

func (m *VPortArg) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_VPortArg.Unmarshal(m, b)
}
func (m *VPortArg) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_VPortArg.Marshal(b, m, deterministic)
}
func (m *VPortArg) XXX_Merge(src proto.Message) {
	xxx_messageInfo_VPortArg.Merge(m, src)
}
func (m *VPortArg) XXX_Size() int {
	return xxx_messageInfo_VPortArg.Size(m)
}
func (m *VPortArg) XXX_DiscardUnknown() {
	xxx_messageInfo_VPortArg.DiscardUnknown(m)
}

var xxx_messageInfo_VPortArg proto.InternalMessageInfo

func (m *VPortArg) GetIfname() string {
	if m != nil {
		return m.Ifname
	}
	return ""
}

type isVPortArg_Cpid interface {
	isVPortArg_Cpid()
}

type VPortArg_Docker struct {
	Docker string `protobuf:"bytes,2,opt,name=docker,proto3,oneof"`
}

type VPortArg_ContainerPid struct {
	ContainerPid int64 `protobuf:"varint,3,opt,name=container_pid,json=containerPid,proto3,oneof"`
}

type VPortArg_Netns struct {
	Netns string `protobuf:"bytes,4,opt,name=netns,proto3,oneof"`
}

func (*VPortArg_Docker) isVPortArg_Cpid() {}

func (*VPortArg_ContainerPid) isVPortArg_Cpid() {}

func (*VPortArg_Netns) isVPortArg_Cpid() {}

func (m *VPortArg) GetCpid() isVPortArg_Cpid {
	if m != nil {
		return m.Cpid
	}
	return nil
}

func (m *VPortArg) GetDocker() string {
	if x, ok := m.GetCpid().(*VPortArg_Docker); ok {
		return x.Docker
	}
	return ""
}

func (m *VPortArg) GetContainerPid() int64 {
	if x, ok := m.GetCpid().(*VPortArg_ContainerPid); ok {
		return x.ContainerPid
	}
	return 0
}

func (m *VPortArg) GetNetns() string {
	if x, ok := m.GetCpid().(*VPortArg_Netns); ok {
		return x.Netns
	}
	return ""
}

func (m *VPortArg) GetRxqCpus() []int64 {
	if m != nil {
		return m.RxqCpus
	}
	return nil
}

func (m *VPortArg) GetTxTci() uint64 {
	if m != nil {
		return m.TxTci
	}
	return 0
}

func (m *VPortArg) GetTxOuterTci() uint64 {
	if m != nil {
		return m.TxOuterTci
	}
	return 0
}

func (m *VPortArg) GetLoopback() bool {
	if m != nil {
		return m.Loopback
	}
	return false
}

func (m *VPortArg) GetIpAddrs() []string {
	if m != nil {
		return m.IpAddrs
	}
	return nil
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*VPortArg) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*VPortArg_Docker)(nil),
		(*VPortArg_ContainerPid)(nil),
		(*VPortArg_Netns)(nil),
	}
}

func init() {
	proto.RegisterType((*PCAPPortArg)(nil), "bess.pb.PCAPPortArg")
	proto.RegisterType((*PMDPortArg)(nil), "bess.pb.PMDPortArg")
	proto.RegisterType((*UnixSocketPortArg)(nil), "bess.pb.UnixSocketPortArg")
	proto.RegisterType((*VPortArg)(nil), "bess.pb.VPortArg")
}

func init() { proto.RegisterFile("ports/port_msg.proto", fileDescriptor_15ffb2279a2b5904) }

var fileDescriptor_15ffb2279a2b5904 = []byte{
	// 532 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x53, 0xd1, 0x4e, 0xdb, 0x3c,
	0x18, 0xa5, 0x24, 0x4d, 0xd2, 0xef, 0xe7, 0x1f, 0xe0, 0x01, 0x32, 0x93, 0xa6, 0x45, 0x95, 0xa6,
	0x75, 0x9a, 0x04, 0x9a, 0x78, 0x02, 0x60, 0x9a, 0xca, 0x05, 0xa3, 0x33, 0xdb, 0x6e, 0xad, 0xd4,
	0x76, 0xc1, 0xa2, 0xb1, 0x5d, 0xdb, 0xe9, 0x72, 0xb3, 0x57, 0xd8, 0x13, 0xef, 0x62, 0xb2, 0x9b,
	0x56, 0xc0, 0x6e, 0x22, 0x9f, 0x73, 0xbe, 0xe3, 0x13, 0xe7, 0x38, 0x70, 0x60, 0xb4, 0xf5, 0xee,
	0x34, 0x3c, 0x69, 0xed, 0xee, 0x4e, 0x8c, 0xd5, 0x5e, 0xa3, 0x7c, 0x2a, 0x9c, 0x3b, 0x31, 0xd3,
	0xe1, 0x1b, 0xf8, 0x6f, 0x72, 0x79, 0x3e, 0x99, 0x68, 0xeb, 0xcf, 0xed, 0x1d, 0xda, 0x83, 0x84,
	0x8b, 0x25, 0xee, 0x95, 0xbd, 0xd1, 0x80, 0x84, 0xe5, 0xf0, 0xcf, 0x36, 0xc0, 0xe4, 0xfa, 0xd3,
	0x7a, 0xe0, 0x15, 0x14, 0x73, 0xad, 0xcd, 0xb4, 0x62, 0x0f, 0x71, 0xaa, 0x20, 0x1b, 0x8c, 0x8e,
	0x21, 0x8f, 0x31, 0x92, 0xe3, 0xed, 0xb2, 0x37, 0x4a, 0xc7, 0x5b, 0x24, 0x0b, 0xc4, 0x15, 0x47,
	0x08, 0x12, 0xc3, 0x24, 0x4e, 0xc2, 0xbe, 0xe3, 0x2d, 0x12, 0x00, 0x3a, 0x80, 0x74, 0x19, 0xc2,
	0xd2, 0x8e, 0x8c, 0x08, 0x7d, 0x84, 0xc3, 0xe5, 0xbc, 0x52, 0x54, 0xcf, 0x66, 0x73, 0x5d, 0x71,
	0x6a, 0x5b, 0xea, 0xbc, 0x95, 0x06, 0xf7, 0x63, 0x1a, 0x0a, 0xe2, 0xcd, 0x4a, 0x23, 0xed, 0x6d,
	0x50, 0xd0, 0x19, 0x1c, 0x3d, 0xb7, 0xcc, 0xe4, 0xdc, 0x0b, 0x8b, 0xb3, 0xe8, 0x79, 0xf9, 0xc4,
	0xf3, 0x39, 0x4a, 0xe8, 0x14, 0x0e, 0x9e, 0x9b, 0x16, 0x52, 0x2d, 0x70, 0x1e, 0x2d, 0xfb, 0x4f,
	0x2c, 0x5f, 0xa5, 0x5a, 0xa0, 0xd7, 0x30, 0x70, 0x9a, 0x3d, 0x88, 0x78, 0xbe, 0xa2, 0xec, 0x8d,
	0xfa, 0xe3, 0x1e, 0x29, 0x56, 0xd4, 0x15, 0x47, 0xef, 0x61, 0xcf, 0x58, 0x5d, 0x4b, 0xc7, 0x1a,
	0xdd, 0x38, 0x5a, 0x6b, 0x2e, 0xf0, 0x20, 0xee, 0xb5, 0xfb, 0x88, 0xbf, 0xd6, 0x5c, 0x20, 0x0c,
	0xf9, 0xfd, 0x4f, 0xf6, 0xe0, 0x9a, 0x1a, 0x43, 0x9c, 0x58, 0xc3, 0x8b, 0x0c, 0xd2, 0xf0, 0xc1,
	0x2e, 0x0a, 0xc8, 0x56, 0x1b, 0x0f, 0x7f, 0xc1, 0xfe, 0x77, 0x25, 0xdb, 0xdb, 0x88, 0xd6, 0x25,
	0x20, 0x48, 0x4d, 0xe5, 0xef, 0xbb, 0x9a, 0xe2, 0x1a, 0x7d, 0x00, 0x54, 0x4b, 0x15, 0x8e, 0x21,
	0x95, 0x17, 0x76, 0x59, 0xcd, 0xa9, 0x72, 0xb1, 0x87, 0x84, 0xec, 0xd6, 0x52, 0x91, 0xf6, 0xaa,
	0xe3, 0xbf, 0x38, 0xf4, 0x0e, 0x76, 0x99, 0x56, 0x33, 0x69, 0x6b, 0xca, 0xb4, 0x52, 0x82, 0xf9,
	0x58, 0x4d, 0x41, 0x5e, 0x74, 0xf4, 0xe5, 0x8a, 0x1d, 0xfe, 0xde, 0x86, 0xe2, 0xc7, 0x3a, 0xf6,
	0x08, 0x32, 0x39, 0x53, 0x55, 0x2d, 0xba, 0xe0, 0x0e, 0x21, 0x0c, 0x19, 0x0f, 0xef, 0x67, 0x63,
	0x5c, 0xa8, 0xb2, 0xc3, 0xe8, 0x2d, 0xfc, 0xcf, 0xb4, 0xf2, 0x95, 0x54, 0xc2, 0x52, 0x23, 0x79,
	0x4c, 0x49, 0xc6, 0x5b, 0x64, 0x67, 0x43, 0x4f, 0x24, 0x47, 0x47, 0xd0, 0x57, 0xc2, 0x2b, 0xb7,
	0xb9, 0x0a, 0x2b, 0x88, 0x8e, 0xa1, 0xb0, 0xed, 0x82, 0x32, 0xd3, 0x38, 0xdc, 0x2f, 0x93, 0x51,
	0x42, 0x72, 0xdb, 0x2e, 0x2e, 0x4d, 0xe3, 0xd0, 0x21, 0x64, 0xbe, 0xa5, 0x9e, 0xc9, 0xd8, 0x71,
	0x4a, 0xfa, 0xbe, 0xfd, 0xc6, 0x24, 0x2a, 0x61, 0xc7, 0xb7, 0x54, 0x37, 0x5e, 0xd8, 0x28, 0xe6,
	0x51, 0x04, 0xdf, 0xde, 0x04, 0x2a, 0x4c, 0x3c, 0xbe, 0xc0, 0xc5, 0x3f, 0x17, 0xb8, 0x90, 0x86,
	0x56, 0x9c, 0x5b, 0x87, 0x07, 0x65, 0x32, 0x1a, 0x90, 0x5c, 0x9a, 0xf3, 0x00, 0x43, 0x33, 0xcc,
	0x48, 0x3e, 0xcd, 0xe2, 0xff, 0x73, 0xf6, 0x37, 0x00, 0x00, 0xff, 0xff, 0xee, 0xbb, 0x05, 0x9a,
	0x57, 0x03, 0x00, 0x00,
}
