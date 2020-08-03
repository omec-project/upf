package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/golang/protobuf/proto"
	p4_config_v1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
)

type P4DeviceConfig []byte

const invalidID = 0
const (
	FUNCTION_TYPE_INSERT uint8 = 1
	FUNCTION_TYPE_UPDATE uint8 = 2
	FUNCTION_TYPE_DELETE uint8 = 3
)

type Intf_Table_Entry struct {
	Ip         []byte
	Prefix_Len int
	Src_Intf   string
	Direction  string
}

type Action_Param struct {
	Len   uint32
	Name  string
	Value []byte
}

type Match_Field struct {
	Len        uint32
	Prefix_Len uint32
	Name       string
	Value      []byte
	Mask       []byte
}

type AppTableEntry struct {
	Field_Size  uint32
	Param_Size  uint32
	Table_Name  string
	Action_Name string
	Fields      []Match_Field
	Params      []Action_Param
}
type P4rtClient struct {
	Client     p4.P4RuntimeClient
	P4Info     p4_config_v1.P4Info
	Stream     p4.P4Runtime_StreamChannelClient
	DeviceID   uint64
	ElectionID p4.Uint128
}

func (c *P4rtClient) tableId(name string) uint32 {
	for _, table := range c.P4Info.Tables {
		if table.Preamble.Name == name {
			return table.Preamble.Id
		}
	}
	return invalidID
}

func (c *P4rtClient) actionId(name string) uint32 {
	for _, action := range c.P4Info.Actions {
		if action.Preamble.Name == name {
			return action.Preamble.Id
		}
	}
	return invalidID
}

func (c *P4rtClient) get_enum_val(enum_name string,
	val_name string) ([]byte, error) {
	enumVal, ok := c.P4Info.TypeInfo.SerializableEnums[enum_name]
	if ok == false {
		err := fmt.Errorf("Enum Not found with name %s", enum_name)
		return nil, err
	}

	for _, enums := range enumVal.Members {
		if enums.Name == val_name {
			return enums.Value, nil
		}
	}

	err := fmt.Errorf("EnumVal not found.\n")
	return nil, err
}

func (c *P4rtClient) SetMastership(electionID p4.Uint128) (err error) {
	c.ElectionID = electionID
	mastershipReq := &p4.StreamMessageRequest{
		Update: &p4.StreamMessageRequest_Arbitration{
			Arbitration: &p4.MasterArbitrationUpdate{
				DeviceId:   1,
				ElectionId: &electionID,
			},
		},
	}
	err = c.Stream.Send(mastershipReq)
	return
}

func (c *P4rtClient) Init() (err error) {
	// Initialize stream for mastership and packet I/O
	c.Stream, err = c.Client.StreamChannel(context.Background())
	if err != nil {
		fmt.Printf("stream channel error: %v\n", err)
		return
	}
	go func() {
		for {
			res, err := c.Stream.Recv()
			if err != nil {
				fmt.Printf("stream recv error: %v\n", err)
			} else if arb := res.GetArbitration(); arb != nil {
				if code.Code(arb.Status.Code) == code.Code_OK {
					fmt.Println("client is master")
				} else {
					fmt.Println("client is not master")
				}
			} else {
				fmt.Printf("stream recv: %v\n", res)
			}

		}
	}()

	fmt.Println("exited from recv thread.")
	return
}

func (c *P4rtClient) WriteInterfaceTable(
	intf_entry Intf_Table_Entry,
	func_type uint8) error {

	fmt.Println("WriteInterfaceTable. \n")
	te := AppTableEntry{
		Table_Name:  "PreQosPipe.source_iface_lookup",
		Action_Name: "PreQosPipe.set_source_iface",
	}

	te.Field_Size = 1
	te.Fields = make([]Match_Field, 1)
	te.Fields[0].Name = "ipv4_dst_prefix"
	te.Fields[0].Value = intf_entry.Ip
	te.Fields[0].Prefix_Len = uint32(intf_entry.Prefix_Len)

	te.Param_Size = 2
	te.Params = make([]Action_Param, 2)
	te.Params[0].Name = "src_iface"
	enum_name := "InterfaceType"
	val, err := c.get_enum_val(enum_name, intf_entry.Src_Intf)
	if err != nil {
		fmt.Printf("Could not find enum val %v", err)
		return err
	}
	te.Params[0].Value = val

	te.Params[1].Name = "direction"
	enum_name = "Direction"
	val, err = c.get_enum_val(enum_name, intf_entry.Direction)
	if err != nil {
		fmt.Printf("Could not find enum val %v", err)
		return err
	}
	te.Params[1].Value = val

	var prio int32 = 0
	return c.InsertTableEntry(te, func_type, prio)
}

func (c *P4rtClient) addFieldValue(entry *p4.TableEntry, field Match_Field,
	tableId uint32) error {
	fmt.Println("add Match field\n")
	fieldVal := &p4.FieldMatch{
		FieldId: 0,
	}

	for _, tables := range c.P4Info.Tables {
		if tables.Preamble.Id == tableId {
			for _, fields := range tables.MatchFields {
				if fields.Name == field.Name {
					fmt.Println("field name match found.\n")
					fieldVal.FieldId = fields.Id
					switch fields.GetMatchType() {
					case p4_config_v1.MatchField_EXACT:
						{
							exact := &p4.FieldMatch_Exact{
								Value: field.Value,
							}
							fieldVal.FieldMatchType = &p4.FieldMatch_Exact_{exact}
						}
					case p4_config_v1.MatchField_LPM:
						{
							lpm := &p4.FieldMatch_LPM{
								Value:     field.Value,
								PrefixLen: int32(field.Prefix_Len),
							}
							fieldVal.FieldMatchType = &p4.FieldMatch_Lpm{lpm}
						}
					case p4_config_v1.MatchField_TERNARY:
						{
							tern := &p4.FieldMatch_Ternary{
								Value: field.Value,
								Mask:  field.Mask,
							}
							fieldVal.FieldMatchType =
								&p4.FieldMatch_Ternary_{tern}
						}
					case p4_config_v1.MatchField_RANGE:
						{
							rangeVal := &p4.FieldMatch_Range{
								Low:  field.Value,
								High: field.Mask,
							}
							fieldVal.FieldMatchType =
								&p4.FieldMatch_Range_{rangeVal}
						}
					default:
						fmt.Printf("Unknown MatchType.\n")
						err := fmt.Errorf("Unknown MatchType for FieldMatch")
						return err
					}

					entry.Match = append(entry.Match, fieldVal)
					return nil
				}
			}
		}
	}

	err := fmt.Errorf("addField Value failed")
	return err
}

func (c *P4rtClient) addActionValue(action *p4.Action, param Action_Param,
	actionId uint32) error {
	fmt.Println("add action param value")

	for _, actions := range c.P4Info.Actions {
		if actions.Preamble.Id == actionId {
			for _, params := range actions.Params {
				if params.Name == param.Name {
					paramVal := &p4.Action_Param{
						ParamId: params.Id,
						Value:   param.Value,
					}
					action.Params = append(action.Params, paramVal)
					return nil
				}
			}
		}
	}

	err := fmt.Errorf("addAction Value failed")
	return err
}

func (c *P4rtClient) InsertTableEntry(
	tableEntry AppTableEntry,
	func_type uint8, prio int32) error {

	fmt.Printf("Insert Table Entry for Table %s\n", tableEntry.Table_Name)
	tableID := c.tableId(tableEntry.Table_Name)
	actionID := c.actionId(tableEntry.Action_Name)
	directAction := &p4.Action{
		ActionId: actionID,
	}

	fmt.Printf("adding action params \n")
	for _, p := range tableEntry.Params {
		err := c.addActionValue(directAction, p, actionID)
		if err != nil {
			fmt.Printf("AddActionValue failed  %v\n", err)
			return err
		}
	}

	tableAction := &p4.TableAction{
		Type: &p4.TableAction_Action{directAction},
	}

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: prio,
		Action:   tableAction,
	}

	fmt.Printf("adding fields\n")
	for _, mf := range tableEntry.Fields {
		err := c.addFieldValue(entry, mf, tableID)
		if err != nil {
			fmt.Printf("AddFieldValue failed  %v\n", err)
			return err
		}
	}

	var updateType p4.Update_Type
	if func_type == FUNCTION_TYPE_UPDATE {
		updateType = p4.Update_MODIFY
	} else if func_type == FUNCTION_TYPE_INSERT {
		updateType = p4.Update_INSERT
	} else if func_type == FUNCTION_TYPE_DELETE {
		updateType = p4.Update_DELETE
	}

	update := &p4.Update{
		Type: updateType,
		Entity: &p4.Entity{
			Entity: &p4.Entity_TableEntry{entry},
		},
	}

    fmt.Println(proto.MarshalTextString(update))
	return c.WriteReq(update)
}

func (c *P4rtClient) WriteReq(update *p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.DeviceID,
		ElectionId: &c.ElectionID,
		Updates:    []*p4.Update{update},
	}
	_, err := c.Client.Write(context.Background(), req)
	return err
}

func (c *P4rtClient) SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath string) (err error) {
	fmt.Printf("P4 Info: %s\n", p4InfoPath)

	p4infoBytes, err := ioutil.ReadFile(p4InfoPath)
	if err != nil {
		fmt.Printf("Read p4info file error %v\n", err)
		return
	}

	var p4info p4_config_v1.P4Info
	err = proto.UnmarshalText(string(p4infoBytes), &p4info)
	if err != nil {
		fmt.Printf("Unmarshal test failed for p4info %v", err)
		return
	}

	c.P4Info = p4info
	deviceConfig, err := LoadDeviceConfig(deviceConfigPath)
	if err != nil {
		fmt.Printf("bmv2 json read failed %v", err)
		return
	}

	var pipeline p4.ForwardingPipelineConfig
	pipeline.P4Info = &p4info
	pipeline.P4DeviceConfig = deviceConfig

	err = SetPipelineConfig(c.Client, c.DeviceID, &c.ElectionID, &pipeline)
	if err != nil {
		fmt.Printf("set pipeline config error %v", err)
		return
	}
	return
}

func SetPipelineConfig(client p4.P4RuntimeClient, deviceID uint64, electionID *p4.Uint128, config *p4.ForwardingPipelineConfig) error {
	req := &p4.SetForwardingPipelineConfigRequest{
		DeviceId:   deviceID,
		RoleId:     0,
		ElectionId: electionID,
		Action:     p4.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config:     config,
	}
	_, err := client.SetForwardingPipelineConfig(context.Background(), req)
	if err != nil {
		fmt.Printf("set forwarding pipeline returned error %v", err)
	}
	return err
}

func GetConnection(host string) (conn *grpc.ClientConn, err error) {
	/* get connection */
	log.Println("Get connection.")
	conn, err = grpc.Dial(host, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("grpc dial err: %v\n", err)
		return nil, err
	}
	return
}

// LoadDeviceConfig : Load Device config
func LoadDeviceConfig(deviceConfigPath string) (P4DeviceConfig, error) {
	fmt.Printf("BMv2 JSON: %s\n", deviceConfigPath)

	deviceConfig, err := os.Open(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %v", deviceConfigPath, err)
	}
	defer deviceConfig.Close()
	bmv2Info, err := deviceConfig.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %v", deviceConfigPath, err)
	}

	bin := make([]byte, int(bmv2Info.Size()))
	if b, err := deviceConfig.Read(bin); err != nil {
		return nil, fmt.Errorf("read %s: %v", deviceConfigPath, err)
	} else if b != int(bmv2Info.Size()) {
		return nil, errors.New("bmv2 bin copy failed")
	}

	return bin, nil
}

func CreateChannel(host string, deviceID uint64) (*P4rtClient, error) {
	log.Println("create channel")
	// Second, check to see if we can reuse the gRPC connection for a new P4RT client
	conn, err := GetConnection(host)
	if err != nil {
		log.Println("grpc connection failed")
		return nil, err
	}

	client := &P4rtClient{
		Client:   p4.NewP4RuntimeClient(conn),
		DeviceID: deviceID,
	}

	err = client.Init()
	if err != nil {
		fmt.Printf("Client Init error: %v\n", err)
		return nil, err
	}

	err = client.SetMastership(p4.Uint128{High: 0, Low: 1})
	if err != nil {
		fmt.Printf("Set Mastership error: %v\n", err)
		return nil, err
	}

	return client, nil
}
