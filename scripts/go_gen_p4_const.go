package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/ettle/strcase"
	"github.com/golang/protobuf/proto"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	"github.com/pborman/getopt/v2"
)

const (
	P4INFO_PATH = "conf/p4/bin/p4info.txt"

	COPYRIGHT_HEADER = "/*\n* Copyright 2022-present Open Networking Foundation\n*\n* SPDX-License-Identifier: Apache-2.0\n*/\n"

	CONST_OPEN  = "//noinspection GoSnakeCaseUsage\nconst (\n"
	CONST_CLOSE = `)`

	UINT32_STRING = "uint32 = "

	HF_VAR_PREFIX         = "Hdr_"
	TBL_VAR_PREFIX        = "Table_"
	CTR_VAR_PREFIX        = "Counter_"
	DIRCTR_VAR_PREFIX     = "DirectCounter_"
	ACT_VAR_PREFIX        = "Action_"
	ACTPARAM_VAR_PREFIX   = "ActionParam_"
	ACTPROF_VAR_PREFIX    = "ActionProfile_"
	PACKETMETA_VAR_PREFIX = "PacketMeta_"
	MTR_VAR_PREFIX        = "Meter_"
)

func generate(p4info *p4ConfigV1.P4Info, builder *strings.Builder) *strings.Builder {
	//HeaderField IDs
	builder.WriteString("//HeaderFields\n")
	for _, element := range p4info.GetTables() {
		for _, matchField := range element.MatchFields {
			tableName := element.GetPreamble().GetName()
			name, ID := matchField.GetName(), strconv.FormatUint(uint64(matchField.GetId()), 10)
			name = strcase.ToPascal(name)

			builder.WriteString(HF_VAR_PREFIX + tableName + name + UINT32_STRING + ID + "\n")
		}
	}
	// Tables
	builder.WriteString("//Tables\n")
	for _, element := range p4info.GetTables() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(TBL_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}
	// Actions
	builder.WriteString("//Actions\n")
	for _, element := range p4info.GetActions() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString("\t" + ACT_VAR_PREFIX + name + "\t\t" + UINT32_STRING + ID + "\n")
	}
	// Indirect Counters
	builder.WriteString("//IndirectCounters\n")
	for _, element := range p4info.GetCounters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(CTR_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}
	// Direct Counters
	builder.WriteString("\t//DirectCounters\n")
	for _, element := range p4info.GetDirectCounters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(DIRCTR_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}
	// Action Param IDs
	builder.WriteString("\t//ActionParams\n")
	for _, element := range p4info.GetActions() {
		for _, actionParam := range element.GetParams() {
			actionName := element.GetPreamble().GetName()
			name, ID := actionParam.GetName(), strconv.FormatUint(uint64(actionParam.GetId()), 10)
			name = strcase.ToPascal(name)

			builder.WriteString(ACTPARAM_VAR_PREFIX + actionName + name + UINT32_STRING + ID + "\n")
		}
	}
	// Action profiles
	builder.WriteString("//ActionProfiles\n")
	for _, element := range p4info.GetActionProfiles() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)

		builder.WriteString(ACTPROF_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}
	// Packet metadata
	builder.WriteString("//PacketMetadata\n")
	for _, element := range p4info.GetControllerPacketMetadata() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(PACKETMETA_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}
	// Meters
	builder.WriteString("//Meters")
	for _, element := range p4info.GetMeters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(MTR_VAR_PREFIX + name + UINT32_STRING + ID + "\n")
	}

	builder.WriteString(CONST_CLOSE + "\n")

	return builder
}

func getP4Config(p4infopath string) *p4ConfigV1.P4Info {
	p4infoBytes, err := ioutil.ReadFile(p4infopath)
	if err != nil {
		panic(fmt.Sprintf("Could not read P4Info file: %v", err))
	}

	var p4info p4ConfigV1.P4Info

	err = proto.UnmarshalText(string(p4infoBytes), &p4info)
	if err != nil {
		panic("Could not retrive P4Info file")
	}

	return &p4info
}

func main() {
	p4infoPath := getopt.StringLong("p4info", 'p', P4INFO_PATH, "Path of the p4info file")
	outputPath := getopt.StringLong("output", 'o', "-", "Default will print to Stdout")

	getopt.ParseV2()

	p4config := getP4Config(*p4infoPath)

	stringBuilder := new(strings.Builder)

	stringBuilder.WriteString(COPYRIGHT_HEADER)

	packageName := ""

	if path := strings.Split(*outputPath, "/"); len(path) <= 1 {
		packageName = "undefined"
	} else {
		packageName = path[len(path)-2]
	}

	stringBuilder.WriteString(fmt.Sprintf("package %s\n", packageName))
	stringBuilder.WriteString(CONST_OPEN + "\n")

	result := generate(p4config, stringBuilder).String()
	result = strings.Replace(result, ".", "_", -1)

	if *outputPath == "-" {
		fmt.Println(result)
		os.Exit(0)
	}

	if err := os.WriteFile(*outputPath, []byte(result), 0644); err != nil {
		panic(fmt.Sprintf("Error while creating File: %v", err))
	}
}
