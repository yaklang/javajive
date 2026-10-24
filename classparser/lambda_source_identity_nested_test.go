package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialNestedLambdaIdentityRequiresCompleteActualFactoryPostorder(t *testing.T) {
	raw := nestedLambdaMetadataOriginal(t, nestedLambdaParameterFixture)
	for _, variant := range []string{"original", "changed enclosing body", "missing enclosing receipt", "missing leaf receipt", "detached nested frame", "reordered nested children", "unsealed nested render", "wrong factory parent", "wrong nested ordinal", "nested arrow removed", "unowned nested arrow", "changed child header", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, source, methods := lambdaEmissionTestDumper(t, raw)
			var root *dumpedMethods
			for _, method := range methods {
				if method.methodName == "nested" {
					root = method
				}
			}
			if root == nil || len(root.lambdaEmission.children) != 1 {
				t.Fatal("original root factory")
			}
			outer := root.lambdaEmission.children[0].body
			if outer == nil || len(outer.lambdaEmission.children) != 2 {
				t.Fatal("actual enclosing lambda must emit the original map and predicate factories")
			}
			leaf := outer.lambdaEmission.children[1]
			switch variant {
			case "changed enclosing body":
				source = strings.Replace(source, root.code, strings.Replace(root.code, "{", "{/* cached body differs */", 1), 1)
			case "missing enclosing receipt":
				delete(d.lambdaMethodSourceBodies, outer.member)
			case "missing leaf receipt":
				delete(d.lambdaMethodSourceBodies, leaf.body.member)
			case "detached nested frame":
				outer.lambdaEmission = nil
			case "reordered nested children":
				children := outer.lambdaEmission.children
				children[0], children[1] = children[1], children[0]
			case "unsealed nested render":
				outer.lambdaEmission.seal = nil
			case "wrong factory parent":
				leaf.request.factory.method = root.member
			case "wrong nested ordinal":
				d.obj.ConstantPool[leaf.body.member.NameIndex-1].(*ConstantUtf8Info).Value = "lambda$null$99"
			case "nested arrow removed":
				source = strings.Replace(source, leaf.packet, "java.util.Objects::isNull", 1)
			case "unowned nested arrow":
				changed := strings.Replace(leaf.packet, "{", "{ java.util.function.Function<String,String> extra=(String hidden)->{return hidden;};", 1)
				source = strings.Replace(source, leaf.packet, changed, 1)
			case "changed child header":
				source = strings.Replace(source, leaf.packet, strings.Replace(leaf.packet, "type", "other", 1), 1)
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			events, known := d.native8LambdaSourceIdentityEvents(source, nil, methods)
			want := variant == "original" || variant == "changed enclosing body"
			if known != want || known && (len(events) != 3 || !nativeLambdaSourceIdentityOrderClosed(events)) || !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatalf("complete original postorder known=%v events=%v for %s", known, events, variant)
			}
			if known {
				for _, event := range events {
					if event.parent != root {
						t.Fatal("whole ordinary declaration is the only movable registration group")
					}
				}
			}
		})
	}
}

func TestAdversarialNestedLambdaBlockIntervalsKeepLexicalChildren(t *testing.T) {
	for _, row := range []struct {
		name, source string
		count        int
		known        bool
		stop         bool
	}{
		{"empty", "return 7;", 0, true, false},
		{"nested", `(int outer)->{return (int inner)->{return inner;};}`, 1, true, false},
		{"siblings", `(int outer)->{return (int inner)->{return inner;};};(long value)->{return value;}`, 2, true, false},
		{"commented braces", `(int x)-> /* } -> */ {return "-> }";}`, 1, true, false},
		{"line comment", "(int x)->// } ->\n{return x;}", 1, true, false},
		{"only quoted arrows", `"->";/* -> */ // ->`, 0, true, false},
		{"expression not emitted block", `(int x)->x`, 0, false, false},
		{"unclosed body", `(int x)->{return x;`, 0, false, false},
		{"unclosed comment", `(int x)->/* {`, 0, false, false},
		{"unclosed literal", `(int x)->{return "->`, 0, false, false},
		{"search exhausted", `(int x)->{return x;}`, 0, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			d := &ClassObjectDumper{}
			scopes, known := d.nativeLambdaDirectBlockScopes(row.source, func(int64) bool { return !row.stop })
			if known != row.known || known && len(scopes) != row.count {
				t.Fatalf("direct lexical scopes=%v known=%v, want %d/%v", scopes, known, row.count, row.known)
			}
		})
	}
}
