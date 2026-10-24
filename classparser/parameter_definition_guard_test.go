package javaclassparser

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialFinalParameterDefinitionRefusalIsAtomic(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class GuardFinalOwner{static int read(int left,int right){try{left++;return left/right;}catch(ArithmeticException expected){return left;}}}`, "source,lines,vars")
	for _, scenario := range []string{"depth", "work", "canceled", "mixed alias", "duplicate header identity", "duplicate code", "duplicate table", "nil unparsed", "flags", "length", "count", "name cp out of range", "name cp kind", "invalid class version"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["GuardFinalOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, candidate := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, candidate.NameIndex)
				if name == "read" {
					method = candidate
					for _, attribute := range method.Attributes {
						if body, ok := attribute.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("authored original method")
			}
			// An unnamed physical table isolates flags from source naming.
			table := &UnparsedAttribute{Name: "MethodParameters", Length: 9, Info: []byte{2, 0, 0, 0, 16, 0, 0, 0, 0}}
			method.Attributes = append(method.Attributes, table)
			integer := types.NewJavaPrimer(types.JavaInteger)
			left := values.NewJavaRef(coreutils.NewRootVariableId(), values.NewJavaLiteral(0, integer), integer)
			right := values.NewJavaRef(coreutils.NewRootVariableId(), values.NewJavaLiteral(0, integer), integer)
			left.IsParam, right.IsParam = true, true
			left.MarkOriginalParameter(0)
			right.MarkOriginalParameter(1)
			tracked := []*values.JavaRef{left, right}
			body := []statements.Statement{&statements.AssignStatement{LeftValue: left, JavaValue: values.NewJavaLiteral(7, integer)}}
			c := NewClassObjectDumper(obj)
			c.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName(), Arguments: []string{"old-left", "old-right"}}
			switch scenario {
			case "depth":
				c.Work = workbudget.New(context.Background(), workbudget.Limits{MaxASTDepth: 2})
				for i := 0; i < 4; i++ {
					body = []statements.Statement{&statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: body}}
				}
			case "work":
				c.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			case "mixed alias":
				alias := values.NewJavaRef(left.Id, values.NewJavaLiteral(int64(3), types.NewJavaPrimer(types.JavaLong)), types.NewJavaPrimer(types.JavaLong))
				tracked = append(tracked, alias)
				body = append(body, &statements.ReturnStatement{JavaValue: alias})
			case "duplicate header identity":
				right.Id = left.Id
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "duplicate table":
				method.Attributes = append(method.Attributes, table)
			case "nil unparsed":
				method.Attributes = append(method.Attributes, (*UnparsedAttribute)(nil))
			case "flags":
				table.Info[3] = 0x20
			case "length":
				table.Length++
			case "count":
				table.Info[0]--
			case "invalid class version":
				obj.MajorVersion = 0
			case "name cp out of range":
				table.Info[1], table.Info[2] = 0xff, 0xff
			case "name cp kind":
				for i, constant := range obj.ConstantPool {
					if _, utf8 := constant.(*ConstantUtf8Info); !utf8 && constant != nil {
						table.Info[1], table.Info[2] = byte((i+1)>>8), byte(i+1)
						break
					}
				}
			}
			type symbolState struct {
				id      *coreutils.VariableId
				seed    values.JavaValue
				uid     string
				isParam bool
			}
			states := map[*values.JavaRef]symbolState{}
			for _, ref := range tracked {
				states[ref] = symbolState{ref.Id, ref.Val, ref.VarUid, ref.IsParam}
			}
			originalArguments := append([]string(nil), c.FuncCtx.Arguments...)
			originalMethod, err := json.Marshal(method)
			if err != nil {
				t.Fatal(err)
			}
			originalMethodAttrs := append([]AttributeInfo(nil), method.Attributes...)
			originalCodeAttrs := append([]AttributeInfo(nil), code.Attributes...)
			originalHandlers := make([]ExceptionTableEntry, len(code.ExceptionTable))
			for i, handler := range code.ExceptionTable {
				originalHandlers[i] = *handler
			}
			originalNestedInfo := map[*UnparsedAttribute][]byte{}
			for _, attribute := range code.Attributes {
				if raw, ok := attribute.(*UnparsedAttribute); ok {
					originalNestedInfo[raw] = append([]byte(nil), raw.Info...)
				}
			}
			originalCode, originalTable := append([]byte(nil), code.Code...), append([]byte(nil), table.Info...)
			originalAttrLen, originalMaxStack, originalMaxLocals := code.AttrLen, code.MaxStack, code.MaxLocals
			originalTableLength := table.Length
			if _, _, known := c.prepareFinalParameterStorage(method, "(II)I", code, []values.JavaValue{left, right}, body, false); known {
				t.Fatal("unproved definition projection accepted")
			}
			for ref, state := range states {
				if ref.Id != state.id || ref.Val != state.seed || ref.VarUid != state.uid || ref.IsParam != state.isParam {
					t.Fatal("failed projection changed a header/body symbol or original seed")
				}
			}
			if !bytes.Equal(originalCode, code.Code) || !bytes.Equal(originalTable, table.Info) || code.AttrLen != originalAttrLen || code.MaxStack != originalMaxStack || code.MaxLocals != originalMaxLocals || table.Length != originalTableLength || !reflect.DeepEqual(originalArguments, c.FuncCtx.Arguments) || !reflect.DeepEqual(originalMethodAttrs, method.Attributes) || !reflect.DeepEqual(originalCodeAttrs, code.Attributes) {
				t.Fatal("failed projection changed original symbols, Code, metadata or scope")
			}
			currentMethod, err := json.Marshal(method)
			if err != nil || !bytes.Equal(originalMethod, currentMethod) || len(code.ExceptionTable) != len(originalHandlers) {
				t.Fatal("failed projection changed original method, Code attributes or handler count")
			}
			for i, handler := range code.ExceptionTable {
				if *handler != originalHandlers[i] {
					t.Fatal("failed projection changed an exception handler")
				}
			}
			for raw, info := range originalNestedInfo {
				if !bytes.Equal(raw.Info, info) {
					t.Fatal("failed projection changed original code/debug metadata")
				}
			}
			if scenario == "depth" || scenario == "work" || scenario == "canceled" {
				if c.Work.Err() == nil {
					t.Fatal("resource refusal lost its budget error")
				}
			}
		})
	}
}
