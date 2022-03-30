// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"sort"
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

	uint32TypeString = "uint32"
	int32TypeString  = "int32"
	int64TypeString  = "uint64"

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
	enumVarPrefix       = "Enum_"
	bitwidthMFVarPrefix = "BitwidthMf_"
	bitwidthAPVarPrefix = "BitwidthAp_"
)

func emitEntityConstantUint32(prefix string, p4EntityName string, id uint32) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = prefix + "_" + p4EntityName
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, uint32TypeString, id)
}

func emitEntityConstantInt32(prefix string, p4EntityName string, value int32) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = prefix + "_" + p4EntityName
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, int32TypeString, value)
}

// TODO: collapse with emitEntityConstantUint32
func emitEntitySizeConstant(prefix string, p4EntityName string, id int64) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = prefix + "_" + p4EntityName
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, int64TypeString, id)
}

func getPreambles(info *p4ConfigV1.P4Info, p4Type string) (preambles []*p4ConfigV1.Preamble) {
	switch p4Type {
	case "Table":
		for _, e := range info.GetTables() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "Action":
		for _, e := range info.GetActions() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "ActionProfile":
		for _, e := range info.GetActionProfiles() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "Counter":
		for _, e := range info.GetCounters() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "DirectCounter":
		for _, e := range info.GetDirectCounters() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "Meter":
		for _, e := range info.GetMeters() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "DirectMeter":
		for _, e := range info.GetDirectMeters() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "ControllerPacketMetadata":
		for _, e := range info.GetControllerPacketMetadata() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "ValueSet":
		for _, e := range info.GetValueSets() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "Register":
		for _, e := range info.GetRegisters() {
			preambles = append(preambles, e.GetPreamble())
		}
	case "Digest":
		for _, e := range info.GetDigests() {
			preambles = append(preambles, e.GetPreamble())
		}
	default:
		panic("unknown p4 type " + p4Type)
	}

	return
}

func generateP4DataFunctions(info *p4ConfigV1.P4Info, p4Type string) string {
	const mapFuncTemplate = "func Get%sIDToNameMap() map[%s]string {\n return map[%s]string {\n"
	const listFuncTemplate = "func Get%sIDList() []%s {\n return []%s {\n"

	mapBuilder, listBuilder := strings.Builder{}, strings.Builder{}
	mapBuilder.WriteString(fmt.Sprintf(mapFuncTemplate, p4Type, uint32TypeString, uint32TypeString))
	listBuilder.WriteString(fmt.Sprintf(listFuncTemplate, p4Type, uint32TypeString, uint32TypeString))

	preambles := getPreambles(info, p4Type)

	for _, element := range preambles {
		name, ID := element.GetName(), element.GetId()

		mapBuilder.WriteString(fmt.Sprintf(mapFormatString, ID, name))
		listBuilder.WriteString(fmt.Sprintf(listFormatString, ID))
	}
	mapBuilder.WriteString("}\n}\n\n")
	listBuilder.WriteString("}\n}\n\n") //Close declarations

	return mapBuilder.String() + listBuilder.String()
}

func generateConstants(p4info *p4ConfigV1.P4Info) string {
	constBuilder := strings.Builder{}
	matchFieldBitwidth := map[string]int32{}
	actionParamBitwidth := map[string]int32{}

	constBuilder.WriteString(constOpen)

	//HeaderField IDs
	constBuilder.WriteString("// HeaderFields\n")
	for _, element := range p4info.GetTables() {
		for _, matchField := range element.MatchFields {
			tableName, name := element.GetPreamble().GetName(), matchField.GetName()

			constBuilder.WriteString(emitEntityConstantUint32(hfVarPrefix+tableName, name, matchField.GetId()))
		}
	}

	// Tables
	constBuilder.WriteString("// Tables\n")
	for _, element := range p4info.GetTables() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(tblVarPrefix, name, ID))

		for _, mf := range element.GetMatchFields() {
			matchFieldBitwidth[mf.GetName()] = mf.GetBitwidth()
		}
	}

	// Actions
	constBuilder.WriteString("// Actions\n")
	for _, element := range p4info.GetActions() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(actVarPrefix, name, ID))

		for _, ap := range element.GetParams() {
			actionParamBitwidth[ap.GetName()] = ap.GetBitwidth()
		}
	}

	// Action Param IDs
	constBuilder.WriteString("// ActionParams\n")
	for _, element := range p4info.GetActions() {
		for _, actionParam := range element.GetParams() {
			actionName, name := element.GetPreamble().GetName(), actionParam.GetName()

			constBuilder.WriteString(emitEntityConstantUint32(actparamVarPrefix+actionName, name, actionParam.GetId()))
		}
	}

	// Indirect Counters
	constBuilder.WriteString("// IndirectCounters\n")
	for _, element := range p4info.GetCounters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(ctrVarPrefix, name, ID))
		constBuilder.WriteString(emitEntitySizeConstant(ctrSizeVarPrefix, name, element.GetSize()))
	}

	// Direct Counters
	constBuilder.WriteString("// DirectCounters\n")
	for _, element := range p4info.GetDirectCounters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(dirCtrVarPrefix, name, ID))
	}

	// Action profiles
	constBuilder.WriteString("// ActionProfiles\n")
	for _, element := range p4info.GetActionProfiles() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(actprofVarPrefix, name, ID))
	}

	// Packet metadata
	constBuilder.WriteString("// PacketMetadata\n")
	for _, element := range p4info.GetControllerPacketMetadata() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(packetmetaVarPrefix, name, ID))
	}

	// Meters
	constBuilder.WriteString("// Meters\n")
	for _, element := range p4info.GetMeters() {
		name, ID := element.GetPreamble().GetName(), element.GetPreamble().GetId()

		constBuilder.WriteString(emitEntityConstantUint32(mtrVarPrefix, name, ID))
		constBuilder.WriteString(emitEntitySizeConstant(mtrSizeVarPrefix, name, element.GetSize()))
	}

	// Enums
	constBuilder.WriteString("// Enumerators\n")
	serializableEnums := p4info.GetTypeInfo().GetSerializableEnums()
	orderedEnumNames := make([]string, 0, len(serializableEnums))
	for k := range serializableEnums {
		orderedEnumNames = append(orderedEnumNames, k)
	}

	sort.Strings(orderedEnumNames)

	for _, eName := range orderedEnumNames {
		for _, member := range serializableEnums[eName].GetMembers() {
			name := eName + "_" + member.GetName()
			enumVal, err := getUint32FromByteArray(member.GetValue())
			if err != nil {
				log.Errorln(name, err)
			} else {
				constBuilder.WriteString(emitEntityConstantUint32(enumVarPrefix, name, enumVal))
			}
		}
	}

	// match field bitwidth
	orderedMfBitwidth := *getOrderedMapKeys(matchFieldBitwidth)
	for _, name := range orderedMfBitwidth {
		constBuilder.WriteString(emitEntityConstantInt32(bitwidthMFVarPrefix, name, matchFieldBitwidth[name]))
	}

	// action param bitwidth
	orderedApBitwidth := *getOrderedMapKeys(actionParamBitwidth)
	for _, name := range orderedApBitwidth {
		constBuilder.WriteString(emitEntityConstantInt32(bitwidthAPVarPrefix, name, actionParamBitwidth[name]))
	}

	constBuilder.WriteString(constOrVarClose + "\n")

	return constBuilder.String()
}

func getUint32FromByteArray(s []byte) (uint32, error) {
	if len(s) > 4 {
		return 0, fmt.Errorf("getUint32FromByteArray failed due to: cannot fit in Uint32")
	}
	var b [4]byte
	copy(b[4-len(s):], s)
	return binary.BigEndian.Uint32(b[:]), nil
}

func mustGetP4Config(p4infopath string) *p4ConfigV1.P4Info {
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

func getOrderedMapKeys(fieldMap map[string]int32) *[]string {
	ordered := make([]string, 0, len(fieldMap))
	for k := range fieldMap {
		ordered = append(ordered, k)
	}

	sort.Strings(ordered)
	return &ordered
}

func main() {
	p4infoPath := flag.String("p4info", p4infoPath, "Path of the p4info file")
	outputPath := flag.String("output", "-", "Default will print to Stdout")
	packageName := flag.String("package", defaultPackageName, "Set the package name")

	flag.Parse()

	p4info := mustGetP4Config(*p4infoPath)

	sb := strings.Builder{}

	sb.WriteString(copyrightHeader + "\n")
	sb.WriteString(fmt.Sprintf("package %s\n", *packageName))

	sb.WriteString(generateConstants(p4info))
	sb.WriteString(generateP4DataFunctions(p4info, "Table"))
	sb.WriteString(generateP4DataFunctions(p4info, "Action"))
	sb.WriteString(generateP4DataFunctions(p4info, "ActionProfile"))
	sb.WriteString(generateP4DataFunctions(p4info, "Counter"))
	sb.WriteString(generateP4DataFunctions(p4info, "DirectCounter"))
	sb.WriteString(generateP4DataFunctions(p4info, "Meter"))
	sb.WriteString(generateP4DataFunctions(p4info, "DirectMeter"))
	sb.WriteString(generateP4DataFunctions(p4info, "ControllerPacketMetadata"))
	sb.WriteString(generateP4DataFunctions(p4info, "Register"))

	result := sb.String()

	if *outputPath == "-" {
		fmt.Println(result)
	} else {
		if err := os.WriteFile(*outputPath, []byte(result), 0644); err != nil {
			panic(fmt.Sprintf("Error while creating File: %v", err))
		}
	}
}
