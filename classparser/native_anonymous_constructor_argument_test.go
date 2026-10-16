package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeAnonymousConstructorArgumentRequiresOriginalFrames(t *testing.T) {
	source, owner := nativeConstructorArgumentFixture("direct-super")
	files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, "none", "8")
	bodyFiles := nativeCompileSourceReleaseClasses(t, map[string]string{"ReceiverScope.java": nativeEnclosingParameterCaptureFixture}, "none", "8")
	wrapped := source
	wrapped = strings.Replace(wrapped, "ReceiverHolder(ReceiverBox<Number> b){box=b;}", "ReceiverHolder(ReceiverBox<Number> b){box=b;}ReceiverHolder(ReceiverHolder other){box=other.box;}", 1)
	wrapped = strings.Replace(wrapped, "super(new ReceiverBox<Number>(capacity())", "super(new ReceiverHolder(new ReceiverBox<Number>(capacity())", 1)
	wrapped = strings.Replace(wrapped, "return x<y?-1:x==y?0:1;}});}}", "return x<y?-1:x==y?0:1;}}));}}", 1)
	wrappedFiles := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": wrapped}, "none", "8")
	for _, variant := range []string{"original", "nil reader", "nil owner", "nil child", "wrong lexical method", "foreign child owner", "missing enclosing", "missing constructor", "duplicate constructor", "nil method", "static constructor", "abstract constructor", "native constructor", "missing code", "duplicate code", "nil code", "empty code", "oversized code", "small stack", "small locals", "large frame allocation", "missing NEW", "wrong NEW class", "wrong child constructor descriptor", "duplicate child constructor", "wrong THIS receiver", "wrong THIS opcode", "prefix handler", "late allocation", "wrapped allocation", "resolver forbidden", "graph budget", "memory budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			caller, e := Parse(append([]byte(nil), files[owner+"$Cell.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			child, e := Parse(append([]byte(nil), files[owner+"$Cell$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			reader := NewClassObjectDumper(caller)
			lexical := "<init>(L" + owner + ";)V"
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range caller.Methods {
				n, _ := sourceBridgeUTF8(caller, m.NameIndex)
				if n == "<init>" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original method")
			}
			dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
			if e := dec.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			newPC, delegatePC := -1, -1
			for _, op := range constructorMotionOps(dec) {
				if op.Instr.OpCode == core.OP_NEW {
					n, _ := sourceBridgeClassName(caller, core.Convert2bytesToInt(op.Data))
					if n == child.GetClassName() {
						newPC = int(op.CurrentOffset)
					}
				}
				if ref := constructorMotionMember(caller, op, core.OP_INVOKESPECIAL); ref != nil && ref.Name == caller.GetSupperClassName() && ref.Member == "<init>" {
					delegatePC = int(op.CurrentOffset)
				}
			}
			if newPC < 0 || delegatePC < 0 {
				t.Fatal("original sites")
			}
			switch variant {
			case "nil reader":
				reader = nil
			case "nil owner":
				reader.obj = nil
			case "nil child":
				child = nil
			case "wrong lexical method":
				lexical = "make()V"
			case "foreign child owner":
				child.ThisClass = child.SuperClass
			case "missing enclosing":
				for i, a := range child.Attributes {
					if x, ok := a.(*UnparsedAttribute); ok && x.Name == "EnclosingMethod" {
						child.Attributes = append(child.Attributes[:i], child.Attributes[i+1:]...)
						break
					}
				}
			case "missing constructor":
				caller.Methods = nil
			case "duplicate constructor":
				caller.Methods = append(caller.Methods, method)
			case "nil method":
				caller.Methods = append(caller.Methods, nil)
			case "static constructor":
				method.AccessFlags |= 8
			case "abstract constructor":
				method.AccessFlags |= 0x400
			case "native constructor":
				method.AccessFlags |= 0x100
			case "missing code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "nil code":
				method.Attributes = []AttributeInfo{(*CodeAttribute)(nil)}
			case "empty code":
				code.Code = nil
			case "oversized code":
				code.Code = make([]byte, 65536)
			case "small stack":
				code.MaxStack = 0
			case "small locals":
				code.MaxLocals = 0
			case "large frame allocation":
				code.MaxStack, code.MaxLocals = 65535, 65535
			case "missing NEW":
				code.Code[newPC], code.Code[newPC+1], code.Code[newPC+2] = core.OP_NOP, core.OP_NOP, core.OP_ACONST_NULL
			case "wrong NEW class":
				index := caller.ConstantPoolManager.AddNewClassInfo("UnrelatedAllocation")
				code.Code[newPC+1], code.Code[newPC+2] = byte(index>>8), byte(index)
			case "wrong child constructor descriptor":
				for _, m := range child.Methods {
					if n, _ := sourceBridgeUTF8(child, m.NameIndex); n == "<init>" {
						m.DescriptorIndex = uint16(child.ConstantPoolManager.AddUtf8Info("()V"))
					}
				}
			case "duplicate child constructor":
				for _, m := range child.Methods {
					if n, _ := sourceBridgeUTF8(child, m.NameIndex); n == "<init>" {
						child.Methods = append(child.Methods, m)
						break
					}
				}
			case "wrong THIS receiver":
				code.Code[5] = core.OP_ALOAD_1
			case "wrong THIS opcode":
				code.Code[delegatePC] = core.OP_INVOKEVIRTUAL
			case "prefix handler":
				end := len(code.Code)
				code.Code = append(code.Code, core.OP_ATHROW)
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: uint16(newPC), EndPc: uint16(delegatePC), HandlerPc: uint16(end), CatchType: uint16(caller.ConstantPoolManager.AddNewClassInfo("java/lang/Throwable"))})
			case "late allocation":
				caller, e = Parse(append([]byte(nil), bodyFiles[owner+"$Cell.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				child, e = Parse(append([]byte(nil), bodyFiles[owner+"$Cell$1.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				reader.obj = caller
			case "wrapped allocation":
				caller, e = Parse(append([]byte(nil), wrappedFiles[owner+"$Cell.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				child, e = Parse(append([]byte(nil), wrappedFiles[owner+"$Cell$1.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				reader.obj = caller
			case "resolver forbidden":
				reader.foldSiblingResolver = func(string) ([]byte, bool) { panic("context must not read declarations") }
				reader.declarationResolver = reader.foldSiblingResolver
			case "graph budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := reader.nativeAnonymousConstructorArgument(child, lexical); got != (variant == "original" || variant == "resolver forbidden") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
