package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

const ordinaryConstructorLifecycleFixture = `class ObserveBase {<E extends Throwable>ObserveBase(boolean fail,Throwable value)throws E{if(fail)throw(E)value;}}
class ObserveFactory {static int field=3;static Object make(boolean fail,Throwable value){return new ObserveBase(fail,value);}static String unbound(){return "visible";}}
class ObserveDriver {public static void main(String[]args){Throwable[]values={new java.io.IOException("same"),new IllegalStateException("same"),null};int rows=0;for(boolean fail:new boolean[]{false,true})for(Throwable value:values){Throwable caught=null;try{if(ObserveFactory.make(fail,value)==null)throw new AssertionError("new");}catch(Throwable e){caught=e;}if(fail?(value==null?!(caught instanceof NullPointerException):caught!=value):caught!=null)throw new AssertionError("identity",caught);rows++;}System.out.println(rows+":same");}}`

func ordinaryConstructorLifecycleSetup(t *testing.T, files map[string][]byte) (*ordinaryConstructorThrowsScope, *values.NewExpression, []values.JavaValue) {
	t.Helper()
	obj, err := Parse(bytes.Clone(files["ObserveFactory.class"]))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	d.options.TargetSourceVersion = 8
	d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }
	d.FuncCtx = &class_context.ClassContext{ClassName: "ObserveFactory", FunctionName: "make", CurrentMethodDesc: "(ZLjava/lang/Throwable;)Ljava/lang/Object;"}
	d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
	var method *MemberInfo
	var code *CodeAttribute
	for _, m := range obj.Methods {
		if name, _ := obj.getUtf8(m.NameIndex); name == "make" {
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
	d.CurrentMethod = method
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	var member *values.JavaClassMember
	newPC, initPC := -1, -1
	for _, op := range decoder.Opcodes() {
		if op.Instr.OpCode == core.OP_NEW {
			newPC = int(op.CurrentOffset)
		}
		if m := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "ObserveBase" {
			member = m
			initPC = int(op.CurrentOffset)
		}
	}
	if member == nil || newPC < 0 || initPC < 0 {
		t.Fatal("original NEW/init")
	}
	flag := constructorThrowsTestParameter(0, types.NewJavaPrimer(types.JavaBoolean), "fail")
	value := constructorThrowsTestParameter(1, types.NewJavaClass("java.lang.Throwable"), "value")
	args := []values.JavaValue{flag, value}
	ft, err := types.ParseMethodDescriptor(member.Description)
	if err != nil {
		t.Fatal(err)
	}
	n := values.NewNewExpression(types.NewJavaClass("ObserveBase"))
	n.HasOriginPC, n.OriginPC = true, newPC
	n.ConstructorCall = &values.FunctionCallExpression{ClassName: "ObserveBase", FunctionName: "<init>", Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: initPC, Object: n, FuncType: ft.FunctionType(), Arguments: args}
	body := []statements.Statement{&statements.ReturnStatement{JavaValue: n}}
	s := d.ordinaryConstructorThrowsScope(code, body, method)
	if s == nil {
		t.Fatal("original scope unavailable")
	}
	r, state := s.admit(member, initPC)
	if state != constructorThrowsProved || r == nil || !r.fresh() {
		t.Fatalf("original proof not proved/fresh: %v", state)
	}
	return s, n, args
}

func TestAdversarialOrdinaryConstructorThrowsFinalEmissionLifecycle(t *testing.T) {
	_, java := t04Tools(t)
	files := nativeCompileDebugClasses(t, ordinaryConstructorLifecycleFixture, "none")
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "ObserveDriver"); got != "6:same\n" {
		t.Fatal("original JVM", got)
	}
	t.Run("debug and alternatives do not consume", func(t *testing.T) {
		s, n, args := ordinaryConstructorLifecycleSetup(t, files)
		if text := n.String(s.c.FuncCtx); strings.Contains(text, "jdec-ordinary-new") {
			t.Fatal("debug before final phase published marker")
		}
		restore, err := s.begin(s.c.FuncCtx, args)
		if err != nil {
			t.Fatal(err)
		}
		defer restore()
		_ = n.String(s.c.FuncCtx)
		_ = n.String(s.c.FuncCtx)
		if len(s.renders) != 0 {
			t.Fatal("ordinary debug call manufactured final authority")
		}
		text, err := s.renderStatement(s.returnStatement, func() string {
			_ = n.String(s.c.FuncCtx)
			_ = n.String(s.c.FuncCtx)
			return "return " + n.String(s.c.FuncCtx) + ";"
		})
		if err != nil {
			t.Fatal("original typed return writer", err)
		}
		clean, err := s.finish(s.c.FuncCtx, text)
		if err != nil || !s.finished || s.failed || strings.Contains(clean, "jdec-ordinary-new") || !strings.HasPrefix(clean, "return new ObserveBase(") {
			t.Fatalf("original actual final invocation did not consume cleanly %v\n%s", err, clean)
		}
		if _, err := s.finish(s.c.FuncCtx, text); err == nil {
			t.Fatal("repeat final publication accepted")
		}
	})
	for _, variant := range []string{"unused", "drop begin", "drop end", "literal", "line comment", "block comment", "replace expression", "duplicate", "alternate duplicate", "wrong end", "nested markers", "outside return", "unicode escape", "stale init CP", "stale max stack", "stale handler", "source parameter rename", "other allocation"} {
		t.Run(variant, func(t *testing.T) {
			s, n, args := ordinaryConstructorLifecycleSetup(t, files)
			restore, err := s.begin(s.c.FuncCtx, args)
			if err != nil {
				t.Fatal(err)
			}
			defer restore()
			text := ""
			source, renderErr := s.renderStatement(s.returnStatement, func() string {
				text = n.String(s.c.FuncCtx)
				if variant == "alternate duplicate" || variant == "wrong end" {
					_ = n.String(s.c.FuncCtx)
				}
				return "return " + text + ";"
			})
			if renderErr != nil || s.failed || len(s.renders) < 1 {
				t.Fatal("original render control failed", renderErr)
			}
			r := s.renders[0]
			switch variant {
			case "unused":
				source = "return null;"
			case "drop begin":
				source = strings.Replace(source, r.begin, "", 1)
			case "drop end":
				source = strings.Replace(source, r.end, "", 1)
			case "literal":
				source = "return " + strconv.Quote(text) + ";"
			case "line comment":
				source = "// " + source + "\nreturn null;"
			case "block comment":
				source = "/* " + source + " */ return null;"
			case "replace expression":
				source = "return " + r.begin + "new Object()" + r.end + ";"
			case "duplicate":
				source += source
			case "alternate duplicate":
				other := s.renders[1]
				source += "return " + other.begin + other.source + other.end + ";"
			case "wrong end":
				source = strings.Replace(source, r.end, s.renders[1].end, 1)
			case "nested markers":
				source = "return " + r.begin + text + r.end + ";"
			case "outside return":
				source = "Object value=" + text + ";return value;"
			case "unicode escape":
				source = `\u0072eturn ` + text + ";"
			case "stale init CP":
				for _, cp := range s.object.ConstantPool {
					if c, ok := cp.(*ConstantUtf8Info); ok && c.Value == "ObserveBase" {
						c.Value = "OtherBase"
						break
					}
				}
			case "stale max stack":
				s.code.MaxStack++
			case "stale handler":
				s.code.ExceptionTable = append(s.code.ExceptionTable, &ExceptionTableEntry{})
			case "source parameter rename":
				args[1].(*values.JavaRef).Id.SetName("otherValue")
			case "other allocation":
				copy := *n
				n.ConstructorCall.Object = &copy
			}
			if _, err := s.finish(s.c.FuncCtx, source); err == nil || s.finished || !s.failed {
				t.Fatalf("invalid final consumption accepted: %s", variant)
			}
		})
	}
	for _, variant := range []string{"missing actual view", "other invocation pointer", "other NEW pointer", "other plan", "other original NEW PC", "other original init PC", "other descriptor", "other owner", "budget before render", "cancel before final"} {
		t.Run(variant, func(t *testing.T) {
			s, n, args := ordinaryConstructorLifecycleSetup(t, files)
			restore, err := s.begin(s.c.FuncCtx, args)
			if err != nil {
				t.Fatal(err)
			}
			defer restore()
			observe := s.c.FuncCtx.SourceConstructedInvocation
			if variant == "missing actual view" {
				s.c.FuncCtx.SourceConstructedInvocation = nil
			} else if variant == "budget before render" {
				s.c.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			} else {
				s.c.FuncCtx.SourceConstructedInvocation = func(view class_context.ConstructedInvocationView) {
					switch variant {
					case "other invocation pointer":
						copy := *n.ConstructorCall
						view.Invocation = &copy
					case "other NEW pointer":
						copy := *n
						view.Allocation = &copy
					case "other plan":
						view.SourcePlan = &nativeMemberAllocation{}
					case "other original NEW PC":
						view.NewPC++
					case "other original init PC":
						view.InitPC++
					case "other descriptor":
						view.PhysicalDescriptor = "()V"
					case "other owner":
						view.Owner = "OtherBase"
					}
					observe(view)
				}
			}
			source, renderErr := s.renderStatement(s.returnStatement, func() string { return "return " + n.String(s.c.FuncCtx) + ";" })
			if renderErr != nil {
				if !s.failed {
					t.Fatal("failed render did not revoke capability", variant)
				}
				return
			}
			if variant == "cancel before final" {
				ctx, cancel := context.WithCancel(context.Background())
				s.c.Work = workbudget.New(ctx, workbudget.Limits{})
				cancel()
			}
			if _, err := s.finish(s.c.FuncCtx, source); err == nil || s.finished || !s.failed {
				t.Fatal("typed/failed observation approved", variant)
			}
		})
	}
}
