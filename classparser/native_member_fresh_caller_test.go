package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeFreshCallerIdentityDoesNotGrantPrivateScope(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"CallerScope.java": `class CallerScope {class Member{}} class ExternalCaller {}`}, "none", "8")
	for _, variant := range []string{"lexical", "external", "copy", "renamed", "wrong family", "missing certificate", "missing lexical row", "external enrolled", "external member role", "missing owner role", "missing member role", "nil member role", "missing anonymous role", "missing enum role", "missing local role", "class CP tag", "canceled bind", "canceled query", "allocation"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(append([]byte(nil), files["CallerScope.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			foreign, err := Parse(append([]byte(nil), files["ExternalCaller.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			p := &nativeMemberFamily{owner: "CallerScope", lexicalObjects: map[string]*ClassObject{"CallerScope": root}}
			obj := foreign
			if variant == "lexical" || variant == "missing lexical row" || variant == "missing owner role" {
				obj = root
			}
			name := obj.GetClassName()
			switch variant {
			case "missing owner role":
				delete(p.lexicalObjects, name)
			case "missing member role", "nil member role":
				p.children = map[string]*nativeMemberClass{name: nil}
			case "missing anonymous role":
				p.anonymousUnits = map[string]*nativeAnonymousFamily{name: nil}
			case "missing enum role":
				p.enumConstants = map[string]*nativeEnumConstantBody{name: nil}
			case "missing local role":
				p.methodLocals = map[string]*nativeMethodLocalClass{name: nil}
			}
			d := NewClassObjectDumper(obj)
			d.nativeMemberRoot = p
			if variant == "canceled bind" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "allocation" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			d.nativeFreshCaller = nativeFreshBindCaller(obj, p, d.Work)
			switch variant {
			case "copy":
				clone := *obj
				d.obj = &clone
			case "renamed":
				i := obj.ConstantPool[obj.ThisClass-1].(*ConstantClassInfo).NameIndex
				obj.ConstantPool[i-1].(*ConstantUtf8Info).Value = "OtherCaller"
			case "wrong family":
				d.nativeMemberRoot = &nativeMemberFamily{owner: p.owner, lexicalObjects: p.lexicalObjects}
			case "missing certificate":
				d.nativeFreshCaller = nil
			case "missing lexical row":
				delete(p.lexicalObjects, name)
			case "external enrolled":
				p.lexicalObjects[name] = obj
			case "external member role":
				p.children = map[string]*nativeMemberClass{name: nil}
			case "class CP tag":
				obj.ConstantPool[obj.ThisClass-1] = &ConstantIntegerInfo{Value: 1}
			case "canceled query":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.nativeFreshCallerClosed()
			if got != (variant == "lexical" || variant == "external") {
				t.Fatalf("caller identity=%v", got)
			}
			if variant == "external" {
				if _, enrolled := p.lexicalObjects[name]; enrolled || p.failed {
					t.Fatal("local restoration enrolled private scope or latched source failure")
				}
			}
		})
	}
}
