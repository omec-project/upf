// SPDX-License-Identifier: Apache-2.0
// Copyright 2020-present Open Networking Foundation
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// List of methods to target
var targetMethods = map[string]bool{
	"ProtoMessage":                           true,
	"isTrafficClass_Arg":                     true,
	"isGateHookInfo_Gate":                    true,
	"isMeasureArg_Type":                      true,
	"isSplitArg_Type":                        true,
	"isTimestampArg_Type":                    true,
	"isQosCommandAddArg_OptionalDeductLen":   true,
	"isGenericEncapArg_EncapField_Insertion": true,
	"isSetMetadataArg_Attribute_Value":       true,
	"isPMDPortArg_Port":                      true,
	"isPMDPortArg_Socket":                    true,
	"isVPortArg_Cpid":                        true,
	"isField_Position":                       true,
	"isFieldData_Encoding":                   true,
}

func main() {
	root := "pfcpiface/bess_pb"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".pb.go") {
			fmt.Println("Processing:", path)
			return processFile(path)
		}

		return nil
	})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func processFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var output []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		// Match any empty function: func (...) methodName() {}
		if strings.HasPrefix(trim, "func (") && strings.HasSuffix(trim, "{}") {
			methodName := extractMethodName(trim)
			if targetMethods[methodName] {
				// Skip if already has SONARQB
				if i > 0 && strings.Contains(lines[i-1], "SONARQB") {
					output = append(output, line)
					continue
				}
				fmt.Println("Adding comment above:", methodName)
				output = append(output, "// SONARQB: Empty protobuf generated method is safe to ignore")
			}
		}
		output = append(output, line)
	}
	return os.WriteFile(path, []byte(strings.Join(output, "\n")), 0o644)
}

// Extract method name from line
func extractMethodName(line string) string {
	// Example:
	// func (*Type) ProtoMessage() {}
	// func (*Type) isSomething() {}

	parts := strings.Split(line, ")")
	if len(parts) < 2 {
		return ""
	}
	right := strings.TrimSpace(parts[1]) // ProtoMessage() {}
	name := strings.Split(right, "(")[0]
	return strings.TrimSpace(name)
}
