// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/omec-project/upf-epc/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestTheDeclineLineCarriesNoCallerAndNamesNothing: when the X3 shipper does not start,
// the general BESS log says that an optional subsystem did not, and no more: no LI
// wording and none of the file and line the logger otherwise records for each line.
func TestTheDeclineLineCarriesNoCallerAndNamesNothing(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	saved := logger.BessLog
	logger.BessLog = zap.New(core, zap.AddCaller()).Sugar()
	t.Cleanup(func() { logger.BessLog = saved })

	declineOptionalSubsystem()

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("%d log entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Caller.Defined {
		t.Errorf("the line carries its caller %s, which points at the Lawful Interception branch", e.Caller.TrimmedPath())
	}
	msg := strings.ToLower(e.Message)
	for _, word := range []string{"li ", "lawful", "intercept", "x3", "shipper", "mdf", "admf"} {
		if strings.Contains(msg, word) {
			t.Errorf("the line %q names %q", e.Message, word)
		}
	}
}

// TestTheShipperBranchLogsOnlyThroughTheDecline: the branch that starts the X3 shipper
// makes no log call of its own, so the only line it can produce is the one above.
func TestTheShipperBranchLogsOnlyThroughTheDecline(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bess.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok || !callsNamed(stmt.Init, "startLIShipper") {
			return true
		}
		found = true
		ast.Inspect(stmt.Body, func(m ast.Node) bool {
			sel, ok := m.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.SelectorExpr); ok {
				if id, ok := pkg.X.(*ast.Ident); ok && id.Name == "logger" {
					t.Errorf("%s: the X3 shipper branch logs through logger.%s directly; call "+
						"declineOptionalSubsystem, whose line carries no caller", fset.Position(sel.Pos()), pkg.Sel.Name)
				}
			}

			return true
		})

		return false
	})
	if !found {
		t.Fatal("no branch on startLIShipper's result in bess.go")
	}
}

func callsNamed(n ast.Node, name string) bool {
	called := false
	if n == nil {
		return false
	}
	ast.Inspect(n, func(m ast.Node) bool {
		if call, ok := m.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
				called = true
			}
		}

		return !called
	})

	return called
}
