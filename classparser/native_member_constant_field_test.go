package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"math"
	"testing"
)

func TestNativeMemberStaticConstantRequiresTypedOriginalValue(t *testing.T) {
	files := nativeCompileClasses(t, `class NativeConstantOwner{class Child{static final long wide=7;}}`)
	for _, variant := range []string{"original", "not final", "volatile", "duplicate visibility", "synthetic", "no value", "duplicate value", "nil value", "attribute length", "zero index", "large index", "nil constant", "wrong constant type", "reference field", "boolean range", "byte range", "short range", "char range", "float payload", "double payload", "canonical float NaN", "canonical double NaN", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, e := Parse(files["NativeConstantOwner$Child.class"])
			if e != nil {
				t.Fatal(e)
			}
			owner, e := Parse(files["NativeConstantOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			var field *MemberInfo
			var constant *ConstantValueAttribute
			for _, f := range object.Fields {
				name, _ := sourceBridgeUTF8(object, f.NameIndex)
				if name == "wide" {
					field = f
					for _, a := range f.Attributes {
						if v, ok := a.(*ConstantValueAttribute); ok {
							constant = v
						}
					}
				}
			}
			if field == nil || constant == nil {
				t.Fatal("original constant")
			}
			var work *workbudget.Budget
			replace := func(descriptor string, value ConstantInfo) {
				field.DescriptorIndex = uint16(object.ConstantPoolManager.AddUtf8Info(descriptor))
				constant.ConstantValueIndex = uint16(object.ConstantPoolManager.AppendConstantInfo(value))
			}
			switch variant {
			case "not final":
				field.AccessFlags &^= 0x10
			case "volatile":
				field.AccessFlags |= 0x40
			case "duplicate visibility":
				field.AccessFlags |= 3
			case "synthetic":
				field.AccessFlags |= 0x1000
			case "no value":
				field.Attributes = nil
			case "duplicate value":
				field.Attributes = append(field.Attributes, constant)
			case "nil value":
				var nilValue *ConstantValueAttribute
				field.Attributes = append(field.Attributes, nilValue)
			case "attribute length":
				constant.AttrLen = 3
			case "zero index":
				constant.ConstantValueIndex = 0
			case "large index":
				constant.ConstantValueIndex = 65535
			case "nil constant":
				object.ConstantPool[constant.ConstantValueIndex-1] = nil
			case "wrong constant type":
				object.ConstantPool[constant.ConstantValueIndex-1] = &ConstantIntegerInfo{Value: 7}
			case "reference field":
				replace("Ljava/lang/Long;", &ConstantLongInfo{Value: 7})
			case "boolean range":
				replace("Z", &ConstantIntegerInfo{Value: 2})
			case "byte range":
				replace("B", &ConstantIntegerInfo{Value: 128})
			case "short range":
				replace("S", &ConstantIntegerInfo{Value: 32768})
			case "char range":
				replace("C", &ConstantIntegerInfo{Value: -1})
			case "float payload":
				replace("F", &ConstantFloatInfo{Value: math.Float32frombits(0x7fc00001)})
			case "double payload":
				replace("D", &ConstantDoubleInfo{Value: math.Float64frombits(0x7ff8000000000001)})
			case "canonical float NaN":
				replace("F", &ConstantFloatInfo{Value: math.Float32frombits(0x7fc00000)})
			case "canonical double NaN":
				replace("D", &ConstantDoubleInfo{Value: math.Float64frombits(0x7ff8000000000000)})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "canonical float NaN" || variant == "canonical double NaN"
			if got := nativeMemberProofWithOwner(object, owner, work) != nil; got != want {
				t.Fatalf("constant field proof %v want%v", got, want)
			}
		})
	}
}
