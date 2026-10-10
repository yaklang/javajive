package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This is a reference-use grammar, not a program-equivalence model. Each
// retained edge has an independent JVMS syntactic witness; real access and
// behavior are covered by the cross-package original-first roundtrip.
func TestNativeProtectedTypeSourceUseKeepsEveryConsumerKind(t *testing.T) {
	files := nativeCompileClasses(t, `class SourceUseRoot{static void unused(){}}`)
	const target = "foreign/Declaring$Entry"
	for _, scenario := range []string{"metadata only", "unrelated utf8", "unrelated class", "array class", "field", "method parameter", "method return", "generic field", "generic class", "unused name-and-type", "unused method type", "unused member owner", "superclass", "interface", "bootstrap class", "enclosing method", "throws", "catch", "annotation", "code type annotation", "local descriptor", "local generic", "stack map conservative", "stack map class", "ldc", "ldc_w", "new", "anewarray", "checkcast", "instanceof", "unreachable new", "multianewarray", "malformed local", "wrong local utf8", "wrong class operand", "zero class operand", "wrong wide ldc", "truncated instruction", "nil code", "nil bootstrap", "unknown annotation", "unknown structural attribute", "unknown code attribute", "budget", "memory", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(files["SourceUseRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&object.ConstantPool)
			class := uint16(pool.AddNewClassInfo(target))
			utf := func(s string) uint16 { return uint16(pool.AddUtf8Info(s)) }
			method := object.Methods[1]
			code := &CodeAttribute{MaxStack: 4, MaxLocals: 1, Code: []byte{core.OP_RETURN}}
			method.Attributes = []AttributeInfo{code}
			// The redundant row is deliberately outside this unrelated class's
			// lexical family. It keeps the original index/dependency edge.
			object.Attributes = []AttributeInfo{&InnerClassesAttribute{Classes: []*InnerClassInfo{{InnerClassInfoIndex: class, OuterClassInfoIndex: uint16(pool.AddNewClassInfo("foreign/Declaring")), InnerNameIndex: utf("Entry"), InnerClassAccessFlags: 4}}}}
			operand := func(op byte, index uint16) []byte { return []byte{op, byte(index >> 8), byte(index), core.OP_RETURN} }
			var work *workbudget.Budget
			wantUsed, wantKnown := true, true
			switch scenario {
			case "metadata only", "unrelated utf8", "unrelated class":
				wantUsed = false
				if scenario == "unrelated utf8" {
					utf("L" + target + ";")
				}
				if scenario == "unrelated class" {
					pool.AddNewClassInfo("foreign/Other")
				}
			case "array class":
				pool.AddNewClassInfo("[[L" + target + ";")
			case "field":
				object.Fields = []*MemberInfo{{NameIndex: utf("field"), DescriptorIndex: utf("L" + target + ";")}}
			case "method parameter":
				method.DescriptorIndex = utf("(L" + target + ";)V")
			case "method return":
				method.DescriptorIndex = utf("()L" + target + ";")
			case "generic field":
				object.Fields = []*MemberInfo{{NameIndex: utf("field"), DescriptorIndex: utf("Ljava/util/List;"), Attributes: []AttributeInfo{&SignatureAttribute{SignatureIndex: utf("Ljava/util/List<L" + target + ";>;")}}}}
			case "generic class":
				object.Attributes = append(object.Attributes, &SignatureAttribute{SignatureIndex: utf("<T:L" + target + ";>Ljava/lang/Object;")})
			case "unused name-and-type":
				pool.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: utf("unused"), DescriptorIndex: utf("()L" + target + ";")})
			case "unused method type":
				pool.AppendConstantInfo(&ConstantMethodTypeInfo{DescriptorIndex: utf("(L" + target + ";)V")})
			case "unused member owner":
				pool.AddNewMethodInfo(target, "unused", "()V")
			case "superclass":
				object.SuperClass = class
			case "interface":
				object.Interfaces = []uint16{class}
			case "bootstrap class":
				ref := uint16(pool.AddNewMethodInfo("foreign/Bootstrap", "bootstrap", "()Ljava/lang/Object;"))
				handle := uint16(pool.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: ref}))
				object.Attributes = append(object.Attributes, &BootstrapMethodsAttribute{BootstrapMethods: []*BootstrapMethod{{BootstrapMethodRef: handle, BootstrapArguments: []uint16{class}}}})
			case "enclosing method":
				object.Attributes = append(object.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Info: []byte{byte(class >> 8), byte(class), 0, 0}})
			case "throws":
				method.Attributes = append(method.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{class}})
			case "catch":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0, CatchType: class}}
			case "annotation":
				object.Attributes = append(object.Attributes, &RuntimeVisibleAnnotationsAttribute{Annotations: []*AnnotationAttribute{{TypeName: "Lforeign/Marker;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "value", Tag: 'c', Value: "L" + target + ";"}}}}})
			case "code type annotation":
				code.Attributes = []AttributeInfo{&TypeAnnotationsAttribute{Annotations: []*TypeAnnotation{{Annotation: &AnnotationAttribute{TypeName: "L" + target + ";"}}}}}
			case "local descriptor", "local generic", "malformed local", "wrong local utf8":
				name, descriptor := "LocalVariableTable", "L"+target+";"
				if scenario == "local generic" {
					name, descriptor = "LocalVariableTypeTable", "Ljava/util/List<L"+target+";>;"
				}
				info := make([]byte, 12)
				binary.BigEndian.PutUint16(info, 1)
				binary.BigEndian.PutUint16(info[4:], 1)
				binary.BigEndian.PutUint16(info[6:], utf("v"))
				binary.BigEndian.PutUint16(info[8:], utf(descriptor))
				if scenario == "malformed local" {
					info = info[:11]
					wantUsed, wantKnown = false, false
				}
				if scenario == "wrong local utf8" {
					binary.BigEndian.PutUint16(info[8:], class)
					wantUsed, wantKnown = false, false
				}
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: name, Info: info}}
			case "stack map conservative":
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Info: []byte{0, 0}}}
			case "stack map class":
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "StackMapTable", Info: []byte{0, 1, 64, 7, byte(class >> 8), byte(class)}}}
			case "ldc":
				code.Code = []byte{core.OP_LDC, byte(class), core.OP_POP, core.OP_RETURN}
			case "ldc_w":
				code.Code = operand(core.OP_LDC_W, class)
			case "new":
				code.Code = operand(core.OP_NEW, class)
			case "anewarray":
				code.Code = operand(core.OP_ANEWARRAY, class)
			case "checkcast":
				code.Code = operand(core.OP_CHECKCAST, class)
			case "instanceof":
				code.Code = operand(core.OP_INSTANCEOF, class)
			case "unreachable new":
				code.Code = append([]byte{core.OP_RETURN}, operand(core.OP_NEW, class)...)
			case "multianewarray":
				code.Code = []byte{core.OP_MULTIANEWARRAY, byte(class >> 8), byte(class), 1, core.OP_RETURN}
			case "wrong class operand", "zero class operand", "wrong wide ldc", "truncated instruction":
				wantUsed, wantKnown = false, false
				code.Code = operand(core.OP_NEW, utf("unrelated"))
				if scenario == "zero class operand" {
					code.Code = operand(core.OP_NEW, 0)
				}
				if scenario == "wrong wide ldc" {
					code.Code = operand(core.OP_LDC2_W, class)
				}
				if scenario == "truncated instruction" {
					code.Code = []byte{core.OP_NEW, 0}
				}
			case "nil code":
				method.Attributes = []AttributeInfo{(*CodeAttribute)(nil)}
				wantUsed, wantKnown = false, false
			case "nil bootstrap":
				object.Attributes = append(object.Attributes, (*BootstrapMethodsAttribute)(nil))
				wantUsed, wantKnown = false, false
			case "unknown annotation":
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "UnknownAnnotation"}}
				wantUsed, wantKnown = false, false
			case "unknown structural attribute":
				object.Attributes = append(object.Attributes, &UnparsedAttribute{Name: "PermittedSubclasses", Info: []byte{0, 1, byte(class >> 8), byte(class)}})
				wantUsed, wantKnown = false, false
			case "unknown code attribute":
				code.Attributes = []AttributeInfo{&UnparsedAttribute{Name: "OpaqueCodeTable", Info: []byte{byte(class >> 8), byte(class)}}}
				wantUsed, wantKnown = false, false
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				wantUsed, wantKnown = false, false
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				wantUsed, wantKnown = false, false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				wantUsed, wantKnown = false, false
			}
			used, known := nativeProtectedTypeRequiresSourceAccess(object, target, work)
			if used != wantUsed || known != wantKnown {
				t.Fatalf("source obligation=(%v,%v), want (%v,%v)", used, known, wantUsed, wantKnown)
			}
			// The same independent JVMS consumer witnesses constrain the bulk
			// source-transaction closure. Opaque metadata is retained in full
			// there; a protected-access proof must instead refuse to admit it.
			var sourceWork *workbudget.Budget
			switch scenario {
			case "budget":
				sourceWork = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				sourceWork = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				sourceWork = workbudget.New(ctx, workbudget.Limits{})
			}
			sourceUsed, sourceKnown := wantUsed, wantKnown
			if scenario == "unknown structural attribute" || scenario == "unknown code attribute" {
				sourceUsed, sourceKnown = true, true
			}
			if scenario == "stack map conservative" {
				// The bulk scanner can prove this zero-entry frame table has
				// no class operand; the older access query stays conservative.
				sourceUsed = false
			}
			if scenario == "unused name-and-type" || scenario == "unused method type" || scenario == "unused member owner" {
				// The original index/access query keeps these symbolic words.
				// An unused symbol is not an actual source-expression root.
				sourceUsed = false
			}
			names, closed := nativeMemberSourceBindingNames(object, sourceWork)
			found := false
			for _, name := range names {
				found = found || name == target
			}
			if closed != sourceKnown || found != sourceUsed {
				t.Fatalf("bulk source obligation=(%v,%v), want (%v,%v)", found, closed, sourceUsed, sourceKnown)
			}
			if scenario == "metadata only" {
				names, known := nativeMemberDependencyNames(object, nil)
				found := false
				for _, name := range names {
					found = found || name == target
				}
				if !known || !found {
					t.Fatal("source-use query removed an original dependency edge")
				}
			}
		})
	}
}
