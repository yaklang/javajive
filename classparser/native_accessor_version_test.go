package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAccessorVersionRequiresLegalFeatureNamespace(t *testing.T) {
	constants := []struct {
		name    string
		value   ConstantInfo
		minimum uint16
	}{
		{"Utf8", &ConstantUtf8Info{}, 49}, {"Class", &ConstantClassInfo{}, 49}, {"Fieldref", &ConstantFieldrefInfo{}, 49},
		{"InterfaceMethodref", &ConstantInterfaceMethodrefInfo{}, 49},
		{"MethodHandle", &ConstantMethodHandleInfo{}, 51}, {"MethodType", &ConstantMethodTypeInfo{}, 51}, {"InvokeDynamic", &ConstantInvokeDynamicInfo{}, 51},
		{"Dynamic", &ConstantDynamicInfo{}, 55}, {"Module", &ConstantModuleInfo{}, 53}, {"Package", &ConstantPackageInfo{}, 53},
	}
	for _, major := range []uint16{48, 49, 50, 51, 52, 53, 55} {
		for _, c := range constants {
			t.Run(fmt.Sprintf("%d/%s", major, c.name), func(t *testing.T) {
				obj := &ClassObject{MajorVersion: major, ConstantPool: []ConstantInfo{c.value}}
				want := major >= 49 && major <= 52 && major >= c.minimum
				if got := nativeAccessorVersion(obj, nil); got != want {
					t.Fatalf("format feature admitted=%v want=%v", got, want)
				}
			})
		}
	}
	for _, variant := range []string{"minor", "typed nil", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := &ClassObject{MajorVersion: 50, ConstantPool: []ConstantInfo{&ConstantUtf8Info{}, &ConstantClassInfo{}}}
			var work *workbudget.Budget
			switch variant {
			case "typed nil":
				obj.ConstantPool = []ConstantInfo{(*ConstantClassInfo)(nil)}
			case "minor":
				obj.MinorVersion = 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if nativeAccessorVersion(obj, work) {
				t.Fatal("unproved format namespace accepted")
			}
		})
	}
}
