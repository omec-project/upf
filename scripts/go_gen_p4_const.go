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
	P4infoPath = "conf/p4/bin/p4info.txt"

	DefaultPackageName = "p4constants"
	// CopyrightHeader uses raw strings to avoid issues with reuse
	CopyrightHeader = `// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation
`

	ConstOpen  = "//noinspection GoSnakeCaseUsage\nconst (\n"
	ConstClose = ")"

	IdTypeString = "uint32"

	HfVarPrefix         = "Hdr_"
	TblVarPrefix        = "Table_"
	CtrVarPrefix        = "Counter_"
	DirctrVarPrefix     = "DirectCounter_"
	ActVarPrefix        = "Action_"
	ActparamVarPrefix   = "ActionParam_"
	ActprofVarPrefix    = "ActionProfile_"
	PacketmetaVarPrefix = "PacketMeta_"
	MtrVarPrefix        = "Meter_"
)

func emitEntityConstant(p4EntityName string, id uint32) string {
	// see: https://go.dev/ref/spec#Identifiers
	p4EntityName = strings.Replace(p4EntityName, ".", "_", -1)
	p4EntityName = strcase.ToPascal(p4EntityName)
	return fmt.Sprintf("%s \t %s = %v\n", p4EntityName, IdTypeString, id)
}

func generateP4Constants(p4info *p4ConfigV1.P4Info, packageName string) string {
	builder := strings.Builder{}

	builder.WriteString(CopyrightHeader + "\n")

	builder.WriteString(fmt.Sprintf("package %s\n", packageName))
	builder.WriteString(ConstOpen + "\n")

	//HeaderField IDs
	builder.WriteString("// HeaderFields\n")
	for _, element := range p4info.GetTables() {
		for _, matchField := range element.MatchFields {
			tableName := element.GetPreamble().GetName()
			name := matchField.GetName()
			builder.WriteString(emitEntityConstant(HfVarPrefix+tableName+name, matchField.GetId()))
		}
	}
	// Tables
	builder.WriteString("// Tables\n")
	for _, element := range p4info.GetTables() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(TblVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Actions
	builder.WriteString("// Actions\n")
	for _, element := range p4info.GetActions() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(ActVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Action Param IDs
	builder.WriteString("// ActionParams\n")
	for _, element := range p4info.GetActions() {
		for _, actionParam := range element.GetParams() {
			actionName := element.GetPreamble().GetName()
			name := actionParam.GetName()
			builder.WriteString(emitEntityConstant(ActparamVarPrefix+actionName+name, actionParam.GetId()))
		}
	}
	// Indirect Counters
	builder.WriteString("// IndirectCounters\n")
	for _, element := range p4info.GetCounters() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(CtrVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Direct Counters
	builder.WriteString("// DirectCounters\n")
	for _, element := range p4info.GetDirectCounters() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(DirctrVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Action profiles
	builder.WriteString("// ActionProfiles\n")
	for _, element := range p4info.GetActionProfiles() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(ActprofVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Packet metadata
	builder.WriteString("// PacketMetadata\n")
	for _, element := range p4info.GetControllerPacketMetadata() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(PacketmetaVarPrefix+name, element.GetPreamble().GetId()))
	}
	// Meters
	builder.WriteString("// Meters\n")
	for _, element := range p4info.GetMeters() {
		name := element.GetPreamble().GetName()
		builder.WriteString(emitEntityConstant(MtrVarPrefix+name, element.GetPreamble().GetId()))
	}

	builder.WriteString(ConstClose + "\n")

	return builder.String()
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
	p4infoPath := flag.String("p4info", P4infoPath, "Path of the p4info file")
	outputPath := flag.String("output", "-", "Default will print to Stdout")
	packageName := flag.String("package", DefaultPackageName, "Set the package name")

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
