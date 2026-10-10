package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeEnumConstantInitializationRetainsOriginalResourceGuards(t *testing.T) {
	files := nativeCompileClasses(t, `enum InitializerGuardChoice{LEFT,RIGHT;}`)
	for _, variant := range []string{"original", "canceled", "memory", "budget", "duplicate initializer", "missing initializer", "wrong constant descriptor", "duplicate constant", "wrong invocation CP tag"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["InitializerGuardChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			switch variant {
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "duplicate initializer", "missing initializer":
				methods := append([]*MemberInfo(nil), obj.Methods...)
				for _, method := range methods {
					n, _ := sourceBridgeUTF8(obj, method.NameIndex)
					if n != "<clinit>" {
						continue
					}
					if variant == "duplicate initializer" {
						obj.Methods = append(obj.Methods, method)
					} else {
						obj.Methods = nil
						for _, m := range methods {
							if m != method {
								obj.Methods = append(obj.Methods, m)
							}
						}
					}
				}
			case "wrong constant descriptor":
				obj.Fields[0].DescriptorIndex = obj.Methods[0].DescriptorIndex
			case "duplicate constant":
				obj.Fields = append(obj.Fields, obj.Fields[0])
			case "wrong invocation CP tag":
				for _, method := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, method.NameIndex)
					if n != "<clinit>" {
						continue
					}
					ops, known := nativeEnumMethodOps(obj, method, nil)
					if !known || len(ops) < 6 {
						t.Fatal("initializer")
					}
					index, known := nativeEnumCPIndex(ops[4])
					if !known {
						t.Fatal("constructor memberref")
					}
					ref, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
					if !ok {
						t.Fatal("original constructor CP tag")
					}
					obj.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
				}
			}
			plans, err := d.nativeEnumConstantInitializations()
			if (err == nil) != (variant == "original") {
				t.Fatalf("initialization admitted=%v err=%v", err == nil, err)
			}
			if variant == "original" {
				if len(plans) != 2 || plans["LEFT"].ordinal != 0 || plans["RIGHT"].ordinal != 1 || plans["LEFT"].newPC == plans["RIGHT"].newPC || plans["LEFT"].invokePC == plans["RIGHT"].invokePC {
					t.Fatalf("original allocation identities:%+v", plans)
				}
			}
			if d.Work != nil && err != nil && d.Work.Err() == nil {
				t.Fatal("resource failure lost its original work-budget error")
			}
		})
	}
}
