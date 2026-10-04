package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The JVM-valid original keeps a constructor parameter unchanged even after
// SUPER reflectively changes the captured field. Java lexical capture syntax
// would read the changed field, so this source capability must refuse it.
func TestNativeAnonymousExpressionInitializerRefusesPostSuperParameterAlias(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static final Object replacement=new Object();static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	f = strings.Replace(f, `AnonymousInitEffects.trace+="P"+first();`, `AnonymousInitEffects.trace+="P"+first();try{java.lang.reflect.Field capture=getClass().getDeclaredField("val$captured");capture.setAccessible(true);capture.set(this,AnonymousInitEffects.replacement);}catch(ReflectiveOperationException e){throw new AssertionError(e);}`, 1)
	f = strings.Replace(f, `!AnonymousInitEffects.trace.equals("P0")||`, `!AnonymousInitEffects.trace.equals("P0I")||`, 1)
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, f, debug)
			obj, e := Parse(append([]byte(nil), files["AnonymousInitOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			changed := 0
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "<init>" {
					continue
				}
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				params, _, e := callbinding.Descriptor(desc)
				if e != nil {
					t.Fatal(e)
				}
				slots := constructorParameterSlots(params)
				for _, a := range m.Attributes {
					code, ok := a.(*CodeAttribute)
					if !ok {
						continue
					}
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if d.ParseOpcode() != nil {
						t.Fatal("decode")
					}
					ops := constructorMotionOps(d)
					var parameter int = -1
					for i, op := range ops {
						field := constructorMotionMember(obj, op, core.OP_PUTFIELD)
						if field != nil && field.Member == "val$captured" {
							parameter = slots[core.GetRetrieveIdx(ops[i-1])]
						}
					}
					slot := -1
					for s, p := range slots {
						if p == parameter {
							slot = s
						}
					}
					if parameter < 0 || slot < 0 || slot > 255 {
						t.Fatal("capture slot")
					}
					for i, op := range ops {
						field := constructorMotionMember(obj, op, core.OP_GETFIELD)
						if field == nil || field.Member != "val$captured" {
							continue
						}
						pc := int(ops[i-1].CurrentOffset)
						if op.CurrentOffset != uint16(pc+1) {
							t.Fatal("original read packet")
						}
						copy(code.Code[pc:pc+4], []byte{byte(core.OP_ALOAD), byte(slot), byte(core.OP_NOP), byte(core.OP_NOP)})
						changed++
					}
				}
			}
			if changed != 1 {
				t.Fatal("one exact post-super capture read")
			}
			files["AnonymousInitOwner$1.class"] = obj.Bytes()
			out := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(out, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, out, "AnonymousInitDriver"); got != "6:anonymous:initializer:callback:identity:owner\n" {
				t.Fatalf("valid original parameter identity=%q", got)
			}
			root, e := Parse(files["AnonymousInitOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(root)
			d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok }
			if d.planNativeAnonymousFamily() != nil {
				t.Fatal("mutable field substituted for original parameter")
			}
		})
	}
}

func TestNativeAnonymousExpressionInitializerRequiresOriginalTypedPackets(t *testing.T) {
	f := strings.Replace(nativeAnonymousInitializerFixture, `static String trace="";`, `static String trace="";static Object observe(Object value){trace+="I";return value;}`, 1)
	f = strings.Replace(f, `Object ref=captured;`, `Object ref=AnonymousInitEffects.observe(captured);`, 1)
	files := nativeCompileClasses(t, f)
	for _, variant := range []string{"original", "small stack", "small locals", "oversize frames", "handler", "duplicate constructor", "duplicate field", "empty signature", "duplicate signature", "malformed signature", "nonvoid signature", "constructor formal", "foreign target", "wrong field descriptor", "captured target", "missing final return", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["AnonymousInitOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var code *CodeAttribute
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			desc, _ := sourceBridgeUTF8(obj, ctor.DescriptorIndex)
			child := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make", nil)
			if child == nil || child.expressionInitializer == nil {
				t.Fatal("original expression proof absent")
			}
			switch variant {
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "oversize frames":
				code.MaxStack = 65535
				code.MaxLocals = 65535
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code)), HandlerPc: 0}}
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, ctor)
			case "duplicate field":
				obj.Fields = append(obj.Fields, obj.Fields[0])
			case "empty signature", "duplicate signature", "malformed signature", "nonvoid signature", "constructor formal":
				raw := map[string]string{"empty signature": "", "duplicate signature": "()V", "malformed signature": "(", "nonvoid signature": "()I", "constructor formal": "<T:Ljava/lang/Object;>()V"}[variant]
				signature := &SignatureAttribute{SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info(raw))}
				ctor.Attributes = append(ctor.Attributes, signature)
				if variant == "duplicate signature" {
					ctor.Attributes = append(ctor.Attributes, signature)
				}
			case "missing final return":
				code.Code[len(code.Code)-1] = byte(core.OP_NOP)
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if d.ParseOpcode() != nil {
				t.Fatal("original decode")
			}
			ops := constructorMotionOps(d)
			start := -1
			for i, op := range ops {
				if int(op.CurrentOffset) == child.superPC {
					start = i + 1
				}
			}
			if start < 0 {
				t.Fatal("super identity")
			}
			if variant == "foreign target" || variant == "wrong field descriptor" || variant == "captured target" {
				for _, op := range ops[start:] {
					field := constructorMotionMember(obj, op, core.OP_PUTFIELD)
					if field == nil || field.Member != "ref" {
						continue
					}
					index := core.Convert2bytesToInt(op.Data)
					ref := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
					if variant == "foreign target" {
						ref.ClassIndex = obj.SuperClass
					}
					if variant == "wrong field descriptor" {
						for _, field := range obj.Fields {
							n, _ := sourceBridgeUTF8(obj, field.NameIndex)
							if n == "ref" {
								field.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("J"))
							}
						}
					}
					if variant == "captured target" {
						nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						nt.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("val$captured"))
					}
				}
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			child.descriptor = desc
			got := nativeAnonymousExpressionInitializerProof(obj, code, ops, start, child, work)
			if (got != nil) != (variant == "original") {
				t.Fatalf("expression initialization accepted=%v in %s", got != nil, variant)
			}
		})
	}
}
