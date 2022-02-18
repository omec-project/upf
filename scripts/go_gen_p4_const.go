package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/ettle/strcase"
	"github.com/golang/protobuf/proto"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
)

const (
	P4infoPath = "conf/p4/bin/p4info.txt"

	DefaultPackageName = "p4constants"

	CopyrightHeader = "// SPDX-License-Identifier: Apache-2.0\n// Copyright 2022-present Open Networking Foundation\n\n"

	ConstOpen  = "//noinspection GoSnakeCaseUsage\nconst (\n"
	ConstClose = `)`

	Uint32String = "uint32 = "

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

func generate(p4info *p4ConfigV1.P4Info, packageName *string) string {
	builder := new(strings.Builder)

	builder.WriteString(CopyrightHeader)

	builder.WriteString(fmt.Sprintf("package %s\n", *packageName))
	builder.WriteString(ConstOpen + "\n")

	//HeaderField IDs
	builder.WriteString("// HeaderFields\n")
	for _, element := range p4info.GetTables() {
		for _, matchField := range element.MatchFields {
			tableName := element.GetPreamble().GetName()
			name, ID := matchField.GetName(), strconv.FormatUint(uint64(matchField.GetId()), 10)
			name = strcase.ToPascal(name)

			builder.WriteString(HfVarPrefix + tableName + name + "\t" + Uint32String + ID + "\n")
		}
	}
	// Tables
	builder.WriteString("// Tables\n")
	for _, element := range p4info.GetTables() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(TblVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Actions
	builder.WriteString("// Actions\n")
	for _, element := range p4info.GetActions() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(ActVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Indirect Counters
	builder.WriteString("// IndirectCounters\n")
	for _, element := range p4info.GetCounters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(CtrVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Direct Counters
	builder.WriteString("// DirectCounters\n")
	for _, element := range p4info.GetDirectCounters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(DirctrVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Action Param IDs
	builder.WriteString("// ActionParams\n")
	for _, element := range p4info.GetActions() {
		for _, actionParam := range element.GetParams() {
			actionName := element.GetPreamble().GetName()
			name, ID := actionParam.GetName(), strconv.FormatUint(uint64(actionParam.GetId()), 10)
			name = strcase.ToPascal(name)

			builder.WriteString(ActparamVarPrefix + actionName + name + "\t" + Uint32String + ID + "\n")
		}
	}
	// Action profiles
	builder.WriteString("// ActionProfiles\n")
	for _, element := range p4info.GetActionProfiles() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)

		builder.WriteString(ActprofVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Packet metadata
	builder.WriteString("// PacketMetadata\n")
	for _, element := range p4info.GetControllerPacketMetadata() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(PacketmetaVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}
	// Meters
	builder.WriteString("// Meters\n")
	for _, element := range p4info.GetMeters() {
		name, ID := element.GetPreamble().GetName(), strconv.FormatUint(uint64(element.GetPreamble().GetId()), 10)
		name = strcase.ToPascal(name)

		builder.WriteString(MtrVarPrefix + name + "\t" + Uint32String + ID + "\n")
	}

	builder.WriteString(ConstClose + "\n")

	return strings.Replace(builder.String(), ".", "_", -1)
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

	result := generate(p4config, packageName)

	if *outputPath == "-" {
		fmt.Println(result)
		os.Exit(0)
	}

	if err := os.WriteFile(*outputPath, []byte(result), 0644); err != nil {
		panic(fmt.Sprintf("Error while creating File: %v", err))
	}
}
