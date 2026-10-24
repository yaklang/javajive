package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestMultiCatchLocalAssignCollapses(t *testing.T) {
	id := utils.NewRootVariableId()
	id.SetName("var6")
	union := types.NewMultiCatchType([]types.JavaType{
		types.NewJavaClass("java.lang.IllegalArgumentException"),
		types.NewJavaClass("java.lang.SecurityException"),
	})
	ref := values.NewJavaRef(id, nil, union)
	ref.WebDeclType = union
	ctx := &class_context.ClassContext{}
	got := NewAssignStatement(ref, values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")), true).String(ctx)
	if strings.Contains(got, " | ") {
		t.Fatalf("union leaked: %s", got)
	}
	if !strings.Contains(got, "Exception") {
		t.Fatalf("missing collapsed type: %s", got)
	}
}

func TestSwitchCaseDeclarationIsBlocked(t *testing.T) {
	id := utils.NewRootVariableId()
	id.SetName("flag")
	decl := NewDeclareStatement(values.NewJavaRef(id, nil, types.NewJavaPrimer(types.JavaBoolean)))
	sw := &SwitchStatement{
		Value: values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)),
		Cases: []*CaseItem{
			{IntValue: 3, Body: []Statement{decl}},
			{IsDefault: true},
		},
	}
	src := sw.String(nil)
	if !strings.Contains(src, "case 3:\n{\nboolean flag") {
		t.Fatalf("declaration is not inside the case block:\n%s", src)
	}
	if !strings.Contains(src, "default:\n{}") {
		t.Fatalf("empty case was not a block:\n%s", src)
	}
}

func TestSourceEnumSwitchProjectionIsAtomicForSelectorAndEveryLabel(t *testing.T) {
	sw := &SwitchStatement{Value: values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)), Cases: []*CaseItem{{IntValue: 1}, {IntValue: 2}, {IsDefault: true}}}
	for _, tc := range []struct {
		name          string
		mapping       map[int]string
		selector      string
		ok, projected bool
	}{
		{"complete", map[int]string{1: "RED", 2: "BLUE"}, "choice", true, true},
		{"partial", map[int]string{1: "RED"}, "choice", true, false},
		{"extra", map[int]string{1: "RED", 2: "BLUE", 3: "GREEN"}, "choice", true, false},
		{"empty label", map[int]string{1: "RED", 2: ""}, "choice", true, false},
		{"empty selector", map[int]string{1: "RED", 2: "BLUE"}, "", true, false},
		{"refused", map[int]string{1: "RED", 2: "BLUE"}, "choice", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ctx := &class_context.ClassContext{SourceEnumSwitch: func(selector any, keys []int) (string, map[int]string, bool) {
				calls++
				if selector != sw.Value || len(keys) != 2 || keys[0] != 1 || keys[1] != 2 {
					t.Fatal("original selector/case set not supplied")
				}
				return tc.selector, tc.mapping, tc.ok
			}}
			src := sw.String(ctx)
			if calls != 1 {
				t.Fatal("selector was evaluated by multiple proof attempts")
			}
			if tc.projected {
				if !strings.Contains(src, "switch(choice)") || !strings.Contains(src, "case RED:") || !strings.Contains(src, "case BLUE:") {
					t.Fatal(src)
				}
			} else {
				if !strings.Contains(src, "switch(7)") || !strings.Contains(src, "case 1:") || !strings.Contains(src, "case 2:") || strings.Contains(src, "RED") {
					t.Fatal(src)
				}
			}
			if !strings.Contains(src, "default:") {
				t.Fatal("default arm dropped")
			}
		})
	}
}
