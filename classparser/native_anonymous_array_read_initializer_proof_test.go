package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeAnonymousInitializerArrayReadRequiresJvmComponentKind(t *testing.T) {
	// JVMS array load kinds: byte and boolean share BALOAD; reference and
	// array components use AALOAD. Other primitive kinds remain distinct.
	cases := []struct {
		descriptor string
		opcode     int
	}{{"[Z", core.OP_BALOAD}, {"[B", core.OP_BALOAD}, {"[C", core.OP_CALOAD}, {"[S", core.OP_SALOAD}, {"[I", core.OP_IALOAD}, {"[J", core.OP_LALOAD}, {"[F", core.OP_FALOAD}, {"[D", core.OP_DALOAD}, {"[Ljava/lang/Object;", core.OP_AALOAD}, {"[[I", core.OP_AALOAD}}
	for _, row := range cases {
		t.Run(row.descriptor, func(t *testing.T) {
			typ, e := types.ParseDescriptor(row.descriptor)
			if e != nil {
				t.Fatal(e)
			}
			member := values.NewJavaArrayMember(values.NewJavaRef(nil, nil, typ), values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			for op := core.OP_IALOAD; op <= core.OP_SALOAD; op++ {
				actual := &core.OpCode{Instr: &core.Instruction{OpCode: op}}
				if got := nativeAnonymousInitializerArrayRead(actual, member); got != (op == row.opcode) {
					t.Fatalf("opcode=%x accepted=%v", op, got)
				}
			}
		})
	}
}
func TestNativeAnonymousInitializerArrayReadRejectsInventedShape(t *testing.T) {
	for _, variant := range []string{"original", "nil opcode", "nil instruction", "operand bytes", "store opcode", "nil value", "nil array", "nil index", "missing array type", "scalar", "zero rank", "excess rank", "missing index type", "boolean index", "long index", "float index", "double index", "byte index", "short index", "char index"} {
		t.Run(variant, func(t *testing.T) {
			array := values.NewJavaRef(nil, nil, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
			index := values.NewJavaRef(nil, nil, types.NewJavaPrimer(types.JavaInteger))
			member := values.NewJavaArrayMember(array, index)
			op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_IALOAD}}
			switch variant {
			case "nil opcode":
				op = nil
			case "nil instruction":
				op.Instr = nil
			case "operand bytes":
				op.Data = []byte{0}
			case "store opcode":
				op.Instr.OpCode = core.OP_IASTORE
			case "nil value":
				member = nil
			case "nil array":
				member.Object = nil
			case "nil index":
				member.Index = nil
			case "missing array type":
				array.ResetVarType(nil)
			case "scalar":
				array.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "zero rank":
				array.Type().RawType().(*types.JavaArrayType).Dimension = 0
			case "excess rank":
				array.Type().RawType().(*types.JavaArrayType).Dimension = 256
			case "missing index type":
				index.ResetVarType(nil)
			case "boolean index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "long index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "float index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaFloat))
			case "double index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaDouble))
			case "byte index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaByte))
			case "short index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaShort))
			case "char index":
				index.ResetVarType(types.NewJavaPrimer(types.JavaChar))
			}
			want := variant == "original" || variant == "byte index" || variant == "short index" || variant == "char index"
			if got := nativeAnonymousInitializerArrayRead(op, member); got != want {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestNativeAnonymousInitializerArrayReadKeepsOriginalAccessPC(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousArrayReadFixture)
	obj, e := Parse(files["ArrayReadOwner$1.class"])
	if e != nil {
		t.Fatal(e)
	}
	child := nativeAnonymousConstructor(obj, "ArrayReadOwner", "make", nil)
	if child == nil || child.expressionInitializer == nil {
		t.Fatal("original expression constructor")
	}
	plan := child.expressionInitializer
	fields := map[string]*values.RefMember{}
	call := (*values.FunctionCallExpression)(nil)
	loadPC := -1
	for _, op := range plan.ops[plan.start:] {
		if f := constructorMotionMember(obj, op, core.OP_GETFIELD); f != nil {
			typ, e := types.ParseDescriptor(f.Description)
			if e != nil {
				t.Fatal(e)
			}
			fields[f.Member] = &values.RefMember{Object: &values.JavaRef{IsThis: true}, Member: f.Member, JavaType: typ, OriginPC: int(op.CurrentOffset), HasOriginPC: true}
		}
		if f := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); f != nil && f.Member == "index" {
			call = &values.FunctionCallExpression{ClassName: f.Name, FunctionName: f.Member, Descriptor: f.Description, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{types.NewJavaPrimer(types.JavaInteger)}, ReturnType: types.NewJavaPrimer(types.JavaInteger)}, Kind: values.InvokeStatic, OriginPC: int(op.CurrentOffset), HasOriginPC: true}
		}
		if op.Instr.OpCode == core.OP_BALOAD {
			loadPC = int(op.CurrentOffset)
		}
	}
	if fields["val$elements"] == nil || fields["val$position"] == nil || call == nil || loadPC < 0 {
		t.Fatal("original typed expression operands")
	}
	call.Arguments = []values.JavaValue{fields["val$position"]}
	for _, v := range []string{"original", "missing origin", "store origin", "unknown origin", "wrong array component", "wrong index call descriptor"} {
		t.Run(v, func(t *testing.T) {
			array := *fields["val$elements"]
			index := *call
			value := values.NewJavaArrayMember(&array, &index)
			value.OriginPC = loadPC
			value.HasOriginPC = true
			switch v {
			case "missing origin":
				value.HasOriginPC = false
			case "store origin":
				for p := range plan.stores {
					value.OriginPC = p
					break
				}
			case "unknown origin":
				value.OriginPC = 65535
			case "wrong array component":
				array.JavaType = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong))
			case "wrong index call descriptor":
				index.Descriptor = "(J)I"
			}
			events := []int{}
			got := nativeAnonymousInitializerExpressionEvents(child, plan, value, &events, nil, nil)
			if got != (v == "original") {
				t.Fatalf("accepted=%v", got)
			}
			if got {
				want := []int{array.OriginPC, fields["val$position"].OriginPC, call.OriginPC, loadPC}
				if len(events) != len(want) {
					t.Fatalf("events=%v", events)
				}
				for i, p := range want {
					if events[i] != p {
						t.Fatalf("events=%v want=%v", events, want)
					}
				}
			}
		})
	}
}
