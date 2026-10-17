package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"testing"
)

func TestNativeCheckedHandlerLoopClausesKeepPhysicalExceptionCoverage(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"LoopCoverage.java": `class LoopCoverageOps{static void begin()throws java.io.IOException{}static void init()throws java.io.IOException{}static boolean condition()throws java.io.IOException{return false;}static void update()throws java.io.IOException{}static void body()throws java.io.IOException{}}
 class LoopCoverage{static void run()throws java.io.IOException{try{LoopCoverageOps.begin();}catch(java.io.IOException caught){for(LoopCoverageOps.init();LoopCoverageOps.condition();LoopCoverageOps.update()){LoopCoverageOps.body();}}}}`}, "none", "8")
	for _, variant := range []string{"contained", "checked init", "checked condition", "checked update", "checked body", "catchall", "wrong handler", "range miss"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["LoopCoverage.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			base := d.buildInvocationMetadata()
			provider := func(n string) (callbinding.Class, bool) {
				if n != "LoopCoverageOps" {
					return base(n)
				}
				c := callbinding.Class{Name: n, MembersComplete: true, ParentsComplete: true}
				for _, name := range []string{"begin", "init", "condition", "update", "body"} {
					desc := "()V"
					if name == "condition" {
						desc = "()Z"
					}
					m := callbinding.Method{Name: name, Desc: desc, Static: true, Public: true, ExceptionsKnown: true}
					if name == "begin" || variant == "checked "+name {
						m.Exceptions = []string{"java/io/IOException"}
					}
					c.Methods = append(c.Methods, m)
				}
				return c, true
			}
			d.FuncCtx = &class_context.ClassContext{InvocationMetadata: provider}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "run" {
					method = m
					attrs := []AttributeInfo{}
					for _, a := range m.Attributes {
						if ca, ok := a.(*CodeAttribute); ok {
							code = ca
						}
						if _, isExceptions := a.(*ExceptionsAttribute); !isExceptions {
							attrs = append(attrs, a)
						}
					}
					m.Attributes = attrs
				}
			}
			if code == nil || len(code.ExceptionTable) != 1 {
				t.Fatal("original exception range")
			}
			entry := code.ExceptionTable[0]
			loop := &statements.ForStatement{InitVar: &statements.ExpressionStatement{}, Condition: &statements.ConditionStatement{}, EndExp: &statements.ExpressionStatement{}}
			handler := &statements.TryCatchStatement{Handlers: []statements.CatchHandler{{EntryPC: int(entry.HandlerPc)}}, CatchBodies: [][]statements.Statement{{loop}}}
			switch variant {
			case "catchall":
				entry.CatchType = 0
			case "wrong handler":
				handler.Handlers[0].EntryPC++
			case "range miss":
				entry.EndPc = entry.StartPc
			}
			// A source container does not cover a new checked call in its initializer,
			// condition, body or update. Each call still needs its original raw range.
			got, err := d.methodNeedsCheckedEscape(code, []statements.Statement{handler}, method)
			if err != nil || got != (variant != "contained") {
				t.Fatalf("escape=%v err=%v", got, err)
			}
		})
	}
}
