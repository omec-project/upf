// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/ettle/strcase"
	"github.com/golang/protobuf/proto"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
)

const (
	p4infoPath = "conf/p4/bin/p4info.txt"

	defaultPackageName = "p4constants"
	// copyrightHeader uses raw strings to avoid issues with reuse
	copyrightHeader = `// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation
`

	constOpen        = "const (\n"
	mapFormatString  = "%v:\"%v\",\n"
	listFormatString = "%v,\n"
	constOrVarClose  = ")\n"

	idTypeString   = "uint32"
	sizeTypeString = "uint64"

	hfVarPrefix         = "Hdr_"
	tblVarPrefix        = "Table_"
	ctrVarPrefix        = "Counter_"
	ctrSizeVarPrefix    = "CounterSize_"
	dirCtrVarPrefix     = "DirectCounter_"
	actVarPrefix        = "Action_"
	actparamVarPrefix   = "ActionParam_"
	actprofVarPrefix    = "ActionProfile_"
	packetmetaVarPrefix = "PacketMeta_"
	mtrVarPrefix        = "Meter_"
	mtrSizeVarPrefix    = "MeterSize_"

	tableMapFunc          = "func GetTableIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	tableListFunc         = "func GetTableIDList() []uint32 {\n return []uint32 {\n"
	actionMapFunc         = "func GetActionIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	actionListFunc        = "func GetActionIDList() []uint32 {\n return []uint32 {\n"
	counterMapFunc        = "func GetCounterIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	counterListFunc       = "func GetCounterIDList() []uint32 {\n return []uint32 {\n"
	directCounterMapFunc  = "func GetDirectCounterIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	directCounterListFunc = "func GetDirectCounterIDList() []uint32 {\n return []uint32 {\n"
	actionProfileMapFunc  = "func GetActionProfileIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	actionProfileListFunc = "func GetActionProfileIDList() []uint32 {\n return []uint32 {\n"
	pktMetadataMapFunc    = "func GetPacketMetadataIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	pktMetadataListFunc   = "func GetPacketMetadataIDList() []uint32 {\n return []uint32 {\n"
	metersMapFunc         = "func GetMeterIDToNameMap() map[uint32]string {\n return map[uint32]string {\n"
	metersListFunc        = "func GetMeterIDList() []uint32 {\n return []uint32 {\n"
)

func emitEntityConstant(p4EntityName string, id uint32) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, idTypeString, id)
}

func emitEntitySizeConstant(p4EntityName string, id int64) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, sizeTypeString, id)
}

func generateP4Constants(p4info *p4ConfigV1.P4Info, packageName string) string {
	constBuilder, mapBuilder, listBuilder := strings.Builder{}, strings.Builder{}, strings.Builder{}

	constBuilder.WriteString(copyrightHeader + "\n")

	constBuilder.WriteString(fmt.Sprintf("package %s\n", packageName))
	constBuilder.WriteString(constOpen)

	//HeaderField IDs
	constBuilder.WriteString("// HeaderFields\n")
	for _, element := range p4info.GetTables() {
		for _, matchField := range element.MatchFields {
			tableName, name := element.GetPreamble().GetName(), matchField.GetName()

			constBuilder.WriteString(emitEntityConstant(hfVarPrefix+tableName+name, matchField.GetId()))
		}
	}
	// Tables
	constBuilder.WriteString("// Tables\n")
	mapBuilder.WriteString(tableMapFunc)
	listBuilder.WriteString(tableListFunc)
	for _, element := range p4info.GetTables() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(tblVarPrefix+name, ID))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close func declaration

	// Actions
	constBuilder.WriteString("// Actions\n")
	mapBuilder.WriteString(actionMapFunc)
	listBuilder.WriteString(actionListFunc)
	for _, element := range p4info.GetActions() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(actVarPrefix+name, ID))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close func declarations

	// Action Param IDs
	constBuilder.WriteString("// ActionParams\n")
	for _, element := range p4info.GetActions() {
		for _, actionParam := range element.GetParams() {
			actionName, name := element.GetPreamble().GetName(), actionParam.GetName()

			constBuilder.WriteString(emitEntityConstant(actparamVarPrefix+actionName+name, actionParam.GetId()))
		}
	}

	// Indirect Counters
	constBuilder.WriteString("// IndirectCounters\n")
	mapBuilder.WriteString(counterMapFunc)
	listBuilder.WriteString(counterListFunc)
	for _, element := range p4info.GetCounters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(ctrVarPrefix+name, ID))
		constBuilder.WriteString(emitEntitySizeConstant(ctrSizeVarPrefix+name, element.GetSize()))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close func declarations

	// Direct Counters
	constBuilder.WriteString("// DirectCounters\n")
	mapBuilder.WriteString(directCounterMapFunc)
	listBuilder.WriteString(directCounterListFunc)
	for _, element := range p4info.GetDirectCounters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(dirCtrVarPrefix+name, element.GetPreamble().GetId()))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close declarations

	// Action profiles
	constBuilder.WriteString("// ActionProfiles\n")
	mapBuilder.WriteString(actionProfileMapFunc)
	listBuilder.WriteString(actionProfileListFunc)
	for _, element := range p4info.GetActionProfiles() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(actprofVarPrefix+name, ID))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close declarations

	// Packet metadata
	constBuilder.WriteString("// PacketMetadata\n")
	mapBuilder.WriteString(pktMetadataMapFunc)
	listBuilder.WriteString(pktMetadataListFunc)
	for _, element := range p4info.GetControllerPacketMetadata() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(packetmetaVarPrefix+name, ID))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close declarations

	// Meters
	constBuilder.WriteString("// Meters\n")
	mapBuilder.WriteString(metersMapFunc)
	listBuilder.WriteString(metersListFunc)
	for _, element := range p4info.GetMeters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstant(mtrVarPrefix+name, ID))
		constBuilder.WriteString(emitEntitySizeConstant(mtrSizeVarPrefix+name, element.GetSize()))
		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close declarations

	constBuilder.WriteString(constOrVarClose + "\n")

	return constBuilder.String() + mapBuilder.String() + listBuilder.String()
}

func getP4Config(p4infopath string) *p4ConfigV1.P4Info {
	p4infoBytes, err := ioutil.ReadFile(p4infopath)
	if err != nil {
		panic(fmt.Sprintf("Could not read P4Info file: %v", err))
	}

	var p4info p4ConfigV1.P4Info

	err = proto.UnmarshalText(string(p4infoBytes), &p4info)
	if err != nil {
		panic("Could not parse P4Info file")
	}

	return &p4info
}

func main() {
	p4infoPath := flag.String("p4info", p4infoPath, "Path of the p4info file")
	outputPath := flag.String("output", "-", "Default will print to Stdout")
	packageName := flag.String("package", defaultPackageName, "Set the package name")

	flag.Parse()

	p4config := getP4Config(*p4infoPath)

	result := generateP4Constants(p4config, *packageName)

	if *outputPath == "-" {
		fmt.Println(result)
	} else {
		if err := os.WriteFile(*outputPath, []byte(result), 0644); err != nil {
			panic(fmt.Sprintf("Error while creating File: %v", err))
		}
	}
}
