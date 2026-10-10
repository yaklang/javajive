package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerOwnReadRequiresExactOriginalField(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int mirror=this.initial;`, 1)
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "static field", "synthetic ordinary field", "descriptor mismatch", "instance constant", "foreign owner"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["AnonymousInitOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			child := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make", nil)
			if child == nil || child.expressionInitializer == nil {
				t.Fatal("original owned field proof absent")
			}
			plan := child.expressionInitializer
			var field *MemberInfo
			for _, f := range obj.Fields {
				n, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if n == "initial" {
					field = f
				}
			}
			if field == nil {
				t.Fatal("original own field")
			}
			switch variant {
			case "static field":
				field.AccessFlags |= 8
			case "synthetic ordinary field":
				field.AccessFlags |= 0x1000
			case "descriptor mismatch":
				field.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("J"))
			case "instance constant":
				field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
			case "foreign owner":
				for _, op := range plan.ops[plan.start:] {
					f := constructorMotionMember(obj, op, core.OP_GETFIELD)
					if f != nil && f.Member == "initial" {
						obj.ConstantPool[core.Convert2bytesToInt(op.Data)-1].(*ConstantFieldrefInfo).ClassIndex = obj.SuperClass
					}
				}
			}
			got := nativeAnonymousExpressionInitializerProof(obj, plan.code, plan.ops, plan.start, child, nil)
			if (got != nil) != (variant == "original") {
				t.Fatalf("accepted=%v", got != nil)
			}
		})
	}
}

func TestNativeAnonymousInitializerOwnReadRequiresActualThisAndPC(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;int mirror=this.initial;`, 1)
	files := nativeCompileClasses(t, fixture)
	obj, e := Parse(files["AnonymousInitOwner$1.class"])
	if e != nil {
		t.Fatal(e)
	}
	child := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make", nil)
	if child == nil || child.expressionInitializer == nil {
		t.Fatal("original owned field proof absent")
	}
	plan := child.expressionInitializer
	pc := -1
	for _, op := range plan.ops[plan.start:] {
		f := constructorMotionMember(obj, op, core.OP_GETFIELD)
		if f != nil && f.Member == "initial" {
			pc = int(op.CurrentOffset)
		}
	}
	if pc < 0 {
		t.Fatal("original read identity")
	}
	for _, variant := range []string{"original", "missing origin", "store origin", "wrong field", "foreign receiver", "custom this", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			receiver := &values.JavaRef{IsThis: true}
			field := &values.RefMember{Object: receiver, Member: "initial", OriginPC: pc, HasOriginPC: true}
			switch variant {
			case "missing origin":
				field.HasOriginPC = false
			case "store origin":
				for p := range plan.stores {
					field.OriginPC = p
					break
				}
			case "wrong field":
				field.Member = "mirror"
			case "foreign receiver":
				receiver.IsThis = false
			case "custom this":
				receiver.CustomValue = &values.CustomValue{}
			case "cycle":
				field.Object = field
			}
			events := []int{}
			known := nativeAnonymousInitializerExpressionEvents(child, plan, field, &events, nil, nil)
			if known != (variant == "original") {
				t.Fatalf("accepted=%v", known)
			}
			if known && (len(events) != 1 || events[0] != pc) {
				t.Fatalf("actual read count/order=%v", events)
			}
		})
	}
}
