// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestStrictLiBlockDoesNotFailTheConfigurationLoad is a source-level assertion that the strict
// `li` decode in `LoadConfigFile` does not fail the load.
//
// The LI block is checked in `startLIShipper`, not in validateConf, because a refusal there would
// take the user plane down and name the offending LI field in the general operator log. The strict
// decode is one layer further up and has the same hazard: `LoadConfigFile`'s only caller answers
// an error with `Fatalln`, so returning it would let one mistyped LI key crash-loop the element
// carrying every subscriber's traffic, and echo the offending key into the operator log.
//
// This is the worst of the three network functions to get wrong. The AMF and SMF stop signalling;
// this one stops forwarding. And a user plane that will not start is the loudest possible
// disclosure that the element is LI-provisioned — visible to every operator, every peer and every
// monitoring system, where a log line is visible only to whoever reads logs.
//
// The rule: whatever statement carries the `strictLiBlock` call must not return from it. The
// verdict is recorded on the LI object and acted on by the LI subsystem, where the ADMF can be
// told. See LiConfig.blockErr and startLIShipper.
func TestStrictLiBlockDoesNotFailTheConfigurationLoad(t *testing.T) {
	const file = "config.go"

	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	// The statement that carries the call, whatever shape it takes.
	var carrier ast.Node

	ast.Inspect(parsed, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.AssignStmt, *ast.ExprStmt, *ast.ReturnStmt:
		default:
			return true
		}

		ast.Inspect(n, func(inner ast.Node) bool {
			call, isCall := inner.(*ast.CallExpr)
			if !isCall {
				return true
			}

			if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == "strictLiBlock" {
				if carrier == nil {
					carrier = n
				}
			}

			return true
		})

		return true
	})

	if carrier == nil {
		t.Fatalf("no call to strictLiBlock in %s; if the strict LI decode moved, move this guard "+
			"with it rather than deleting it — the property it holds is that a refused LI block "+
			"never stops the user plane", file)
	}

	if _, isReturn := carrier.(*ast.ReturnStmt); isReturn {
		t.Fatalf("%s: strictLiBlock's verdict is returned directly from the configuration load, "+
			"which reaches cmd/pfcpiface's Fatalln and crash-loops the user plane over an "+
			"optional subsystem", file)
	}

	var (
		returns  bool
		recorded bool
	)

	ast.Inspect(carrier, func(n ast.Node) bool {
		// A nested function literal has its own return, which is its own business.
		if _, isFunc := n.(*ast.FuncLit); isFunc {
			return false
		}

		if _, isReturn := n.(*ast.ReturnStmt); isReturn {
			returns = true
		}

		if sel, isSel := n.(*ast.SelectorExpr); isSel && sel.Sel != nil && sel.Sel.Name == "blockErr" {
			recorded = true
		}

		return true
	})

	if returns {
		t.Errorf("%s: the statement carrying strictLiBlock returns. A refused `li` object must "+
			"stop interception and nothing else — returning here fails LoadConfigFile, which "+
			"cmd/pfcpiface answers with Fatalln, taking every subscriber's traffic down over a "+
			"typo in an optional subsystem.", file)
	}

	if !recorded {
		t.Errorf("%s: strictLiBlock's verdict is not recorded on the LI object's blockErr, so "+
			"nothing downstream can act on it. Record it and let startLIShipper refuse "+
			"interception on it, where the ADMF can be told.", file)
	}

	// The other way the load can fail on the `li` object: the decode of the whole document
	// reading it. That decode returns on a value of the wrong type, before strictLiBlock is
	// reached, so the li object must not be visible to it at all.
	if tag := confFieldTag(t, parsed, "Li"); tag != "`json:\"-\"`" {
		t.Errorf("%s: Conf.Li carries the tag %s, so the decode of the whole document reads the "+
			"`li` object and a value of the wrong type there fails the load — and reaches "+
			"Fatalln — before strictLiBlock can contain it. Tag it `json:\"-\"` and leave the "+
			"decoding to strictLiBlock.", file, tag)
	}
}

// confFieldTag returns the raw struct tag of one field of Conf.
func confFieldTag(t *testing.T, parsed *ast.File, field string) string {
	t.Helper()

	var tag string

	ast.Inspect(parsed, func(n ast.Node) bool {
		spec, isSpec := n.(*ast.TypeSpec)
		if !isSpec || spec.Name.Name != "Conf" {
			return true
		}

		st, isStruct := spec.Type.(*ast.StructType)
		if !isStruct {
			return false
		}

		for _, f := range st.Fields.List {
			for _, name := range f.Names {
				if name.Name == field && f.Tag != nil {
					tag = f.Tag.Value
				}
			}
		}

		return false
	})

	if tag == "" {
		t.Fatalf("Conf has no tagged field %s; if it moved, move this guard with it", field)
	}

	return tag
}

// TestLiTypeErrorIsContainedNotFatal: a value of the wrong type in the `li` object is refused
// the way a misspelled key is — on the LI object, where startLIShipper acts on it — and not by
// failing the configuration load, which cmd/pfcpiface answers with Fatalln.
//
// A quoted number or a quoted boolean is the likeliest mistake in a hand-written file, and a
// load that fails on it stops the user plane and prints the LI field's name in the general
// operator log.
func TestLiTypeErrorIsContainedNotFatal(t *testing.T) {
	load := func(t *testing.T, body string) (Conf, error) {
		t.Helper()

		path := t.TempDir() + "/conf.jsonc"
		mustWriteStringToDisk(body, path)

		return LoadConfigFile(path)
	}

	for _, value := range []string{`"x3_rcvbuf": "65536"`, `"keepalive_disabled": "false"`} {
		t.Run(value, func(t *testing.T) {
			conf, err := load(t, `{"mode": "dpdk", "li": {"x3_sockaddr": "/x3", "ne_id": "u", `+value+`}}`)
			if err != nil {
				t.Fatalf("a value of the wrong type in the li object failed the configuration "+
					"load, which crash-loops the user plane and names the field in the general "+
					"log: %v", err)
			}
			if conf.Li == nil {
				t.Fatal("the li object went missing, so the refusal has nowhere to be carried " +
					"and interception reads as off")
			}
			if conf.Li.blockErr == nil {
				t.Error("a value of the wrong type was accepted rather than refused")
			}
			if conf.Li.NEID != "u" {
				t.Errorf("the fields that did decode were lost: ne_id = %q, want \"u\"", conf.Li.NEID)
			}
		})
	}

	t.Run("an li that is not an object", func(t *testing.T) {
		conf, err := load(t, `{"mode": "dpdk", "li": 5}`)
		if err != nil {
			t.Fatalf("an li that is not an object failed the configuration load: %v", err)
		}
		if conf.Li == nil || conf.Li.blockErr == nil {
			t.Error("an li that is not an object was read as interception being off, so the " +
				"refusal is never acted on")
		}
	})

	t.Run("a null li is no li", func(t *testing.T) {
		conf, err := load(t, `{"mode": "dpdk", "li": null}`)
		if err != nil {
			t.Fatalf("a null li failed the configuration load: %v", err)
		}
		if conf.Li != nil {
			t.Error("a null li started the interception subsystem")
		}
	})

	t.Run("a type error outside li still fails the load", func(t *testing.T) {
		if _, err := load(t, `{"mode": "dpdk", "max_req_retries": "x"}`); err == nil {
			t.Error("a value of the wrong type outside the li object was accepted; containment " +
				"is for the li object only")
		}
	})
}

// TestTheStrictLiDecodeIsStillStrict guards the other direction. The containment fix is a narrow
// one — who acts on the refusal — and it must not be read, or implemented, as backing out the
// strictness that produced it.
func TestTheStrictLiDecodeIsStillStrict(t *testing.T) {
	const body = `{
		"mode": "dpdk",
		"li": {
			"x3_sockaddr": "/pod-share/x3",
			"ne_id": "upf-1",
			"trigger_keepalve": "5m"
		}
	}`

	if _, err := strictLiBlock([]byte(body)); err == nil {
		t.Error("a misspelled LI key was accepted by the strict decode; the containment fix is " +
			"about who acts on the refusal, not about whether there is one")
	}
}
