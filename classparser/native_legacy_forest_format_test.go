package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeLegacyForestFormatRejectsUnprovedFeatures(t *testing.T) {
	for _, source := range []string{
		`interface VersionInterface{static int value(){return 7;}}class VersionOwner{int call(){return VersionInterface.value();}}`,
		`interface VersionInterface{default int value(){return 7;}}class VersionOwner implements VersionInterface{int call(){return VersionInterface.super.value();}}`,
	} {
		files := nativeCompileClasses(t, source)
		for _, major := range []uint16{48, 49, 50, 51, 52, 53, 54, 55} {
			t.Run(fmt.Sprint(major, source), func(t *testing.T) {
				obj, e := Parse(files["VersionOwner.class"])
				if e != nil {
					t.Fatal(e)
				}
				obj.MajorVersion = major
				if got := nativeAnonymousForestVersion(obj, nil); got != (major >= 52 && major <= 54) {
					t.Fatalf("pre52 interface static/special opcode admitted=%v", got)
				}
			})
		}
	}
	files := nativeCompileClasses(t, nativeAccessorAnonymousScopeFixture)
	for _, variant := range []string{"49 original", "50 original", "51 original", "52 original", "later CP tag", "typed nil CP", "nil method", "nil code", "minor", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["AccessScopeOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			obj.MajorVersion = 50
			var work *workbudget.Budget
			switch variant {
			case "49 original":
				obj.MajorVersion = 49
			case "51 original":
				obj.MajorVersion = 51
			case "52 original":
				obj.MajorVersion = 52
			case "later CP tag":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodTypeInfo{})
			case "typed nil CP":
				obj.ConstantPool = append(obj.ConstantPool, (*ConstantClassInfo)(nil))
			case "nil method":
				obj.Methods = append(obj.Methods, nil)
			case "nil code":
				obj.Methods[0].Attributes = append(obj.Methods[0].Attributes, (*CodeAttribute)(nil))
			case "minor":
				obj.MinorVersion = 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "49 original" || variant == "50 original" || variant == "51 original" || variant == "52 original"
			if got := nativeAnonymousForestVersion(obj, work); got != want {
				t.Fatalf("format admitted=%v want=%v", got, want)
			}
		})
	}
}
