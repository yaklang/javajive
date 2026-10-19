package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMovableMethodRegistrationRequiresOriginalCompletedDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, methodRegistrationScheduleFixture)
	for _, variant := range []string{"original", "missing family", "foreign dumper", "foreign object", "detached cache", "lambda role", "synthetic", "constructor role", "method local scope", "anonymous scope", "invokedynamic counter", "no compiler event", "empty code", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			object, err := Parse(files["MethodScheduleOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := archive.prepareNativeMemberFamilyUnpublished(object, map[string]string{})
			if prepared == nil {
				t.Fatal("original local family preparation")
			}
			d := prepared.reader
			d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
			if _, err := d.DumpClass(); err != nil {
				t.Fatal("completed original declaration", err)
			}
			methods, err := d.DumpMethods()
			if err != nil {
				t.Fatal(err)
			}
			var body *dumpedMethods
			for _, m := range methods {
				if m.methodName == "allocate" {
					body = m
				}
			}
			if body == nil || d.nativeMemberRoot == nil || len(d.nativeMemberRoot.getters) == 0 {
				t.Fatal("original method/constructor/getter inventory")
			}
			text := strings.TrimSpace(body.code)
			name, _ := sourceBridgeUTF8(object, body.member.NameIndex)
			desc, _ := sourceBridgeUTF8(object, body.member.DescriptorIndex)
			switch variant {
			case "missing family":
				d.nativeMemberRoot = nil
			case "foreign dumper":
				body.sourceDumper = NewClassObjectDumper(object)
			case "foreign object":
				body.sourceObject = &ClassObject{}
			case "detached cache":
				delete(d.dumpedMethodsSet, body.sourceCacheKey)
			case "lambda role":
				body.sourceRequest.role = methodSourceLambdaBody
			case "synthetic":
				body.member.AccessFlags |= 0x1000
			case "constructor role":
				body.methodName = "<init>"
			case "method local scope":
				d.nativeMemberRoot.methodLocals = map[string]*nativeMethodLocalClass{"local": {owner: &nativeMethodLocalOwner{owner: object.GetClassName(), method: name, descriptor: desc}}}
			case "anonymous scope":
				p := d.nativeMemberRoot
				p.anonymousForest.groups[p.owner].children["synthetic negative"] = &nativeAnonymousClass{method: name + desc}
			case "invokedynamic counter":
				// Denial control only: this byte sequence need not be verifier
				// valid; an unknown/new counter can never grant movement.
				for _, a := range body.member.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code = []byte{core.OP_INVOKEDYNAMIC, 0, 1, 0, 0, core.OP_RETURN}
					}
				}
			case "no compiler event":
				body.code = "int plain(){return 7;}"
			case "empty code":
				body.code = ""
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			allowed := d.nativeMemberRegistrationMethods([]*dumpedMethods{body})
			if allowed[text] != (variant == "original") {
				t.Fatalf("ordinary method scheduling authority=%v", allowed[text])
			}
		})
	}
}

func TestNativeMethodRegistrationLayoutKeepsInitializationOrder(t *testing.T) {
	one := "int first(){return Owner.first/*jdec-owned-getter:100:Owner:first*/;}"
	two := "int second(){return Owner.second/*jdec-owned-getter:0:Owner:second*/;}"
	a, b, c := "Object a=effect(1);", "Object b=effect(2);", "Object c=effect(3);"
	for _, permitted := range []bool{false, true} {
		p := &nativeMemberFamily{owner: "Owner", getters: map[string]*nativeMemberPrivateGetter{"first": {owner: "Owner", field: "first", ordinal: 100}, "second": {owner: "Owner", field: "second", ordinal: 0}}, registrationMethods: map[string]bool{two: permitted}}
		source := "class Owner{" + a + one + b + two + c + "}"
		out, known := nativeMemberRegistrationLayout(p, source, nil, nil)
		if known != permitted {
			t.Fatalf("permitted=%v known=%v", permitted, known)
		}
		if !known {
			continue
		}
		if strings.Index(out, a) >= strings.Index(out, b) || strings.Index(out, b) >= strings.Index(out, c) || strings.Count(out, one) != 1 || strings.Count(out, two) != 1 || !nativeMemberPrivateGetterSourceClosed(p, out, nil) {
			t.Fatal("declaration scheduling moved initialization, duplicated a method or discarded registration")
		}
	}
}
