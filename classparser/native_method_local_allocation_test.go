package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMethodLocalAllocationsBindOriginalParameterSlots(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			source := `class LocalSiteOwner {Object make(final long seed,final long other,final Object token,boolean mode){class Entry {long get(){return seed;}Object token(){return token;}}if(mode)return new Entry();return new Entry();}static Object stat(final double number,final Object token){class Node {double get(){return number;}Object token(){return token;}}return new Node();}Object producer(Object token){final Object made=new Object();class Produced {Object get(){return made;}}return new Produced();}Object phi(Object a,Object b,boolean flag){final Object made=flag?a:b;class Merged {Object get(){return made;}}return new Merged();}}`
			files := nativeCompileDebugClasses(t, source, debug)
			for _, variant := range []string{"original", "static wide", "producer", "phi", "null outer", "second site swapped wide parameter", "small stack", "small locals", "oversize frames", "cached capture name", "cached capture PC", "foreign owner", "wrong method descriptor", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LocalSiteOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					simple := "Entry"
					if variant == "static wide" {
						simple = "Node"
					}
					if variant == "producer" {
						simple = "Produced"
					}
					if variant == "phi" {
						simple = "Merged"
					}
					var local *ClassObject
					for path, raw := range files {
						if strings.HasSuffix(path, "1"+simple+".class") {
							local, e = Parse(append([]byte(nil), raw...))
							if e != nil {
								t.Fatal(e)
							}
						}
					}
					if local == nil {
						t.Fatal("actual named local missing")
					}
					owner, known := originalMethodLocalOwner(local, root, nil)
					if !known {
						t.Fatal("actual owner")
					}
					constructor, known := originalMethodLocalDefaultConstructor(local, root, nil)
					if !known {
						t.Fatal("actual default capture protocol")
					}
					var code *CodeAttribute
					for _, a := range owner.declaration.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
					if code == nil {
						t.Fatal("actual declaring body")
					}
					switch variant {
					case "null outer", "second site swapped wide parameter":
						decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(root.ConstantPool, i) })
						if e := decoder.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						matches := 0
						for _, op := range decoder.Opcodes() {
							if variant == "null outer" && op.Instr.OpCode == core.OP_ALOAD_0 {
								code.Code[op.CurrentOffset] = byte(core.OP_ACONST_NULL)
								matches++
							}
							if variant == "second site swapped wide parameter" && op.Instr.OpCode == core.OP_LLOAD_1 {
								matches++
								if matches == 2 {
									code.Code[op.CurrentOffset] = byte(core.OP_LLOAD_3)
								}
							}
						}
						if matches != 2 {
							t.Fatalf("original two allocation capture loads %d", matches)
						}
					case "small stack":
						code.MaxStack = 1
					case "small locals":
						code.MaxLocals = 1
					case "oversize frames":
						code.MaxLocals = 65535
						code.MaxStack = 65535
					case "cached capture name":
						index := constructor.captures[constructor.enclosingField]
						delete(constructor.captures, constructor.enclosingField)
						constructor.captures["foreignCapture"] = index
					case "cached capture PC":
						constructor.capturePCs[constructor.enclosingField]++
					case "foreign owner":
						owner.owner += "Other"
					case "wrong method descriptor":
						owner.descriptor = "()V"
					}
					d := NewClassObjectDumper(root)
					if variant == "budget" {
						d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					}
					if variant == "canceled" {
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					sites, known := d.nativeMethodLocalParameterAllocations(local, owner, constructor)
					want := variant == "original" || variant == "static wide"
					if known != want {
						t.Fatalf("known=%v sites=%+v", known, sites)
					}
					if known {
						expected := 2
						if variant == "static wide" {
							expected = 1
						}
						if len(sites) != expected {
							t.Fatalf("original sites: %d", len(sites))
						}
						for pc, site := range sites {
							if pc != site.newPC || site.invokePC <= site.newPC {
								t.Fatalf("site identity %+v", site)
							}
							if variant == "original" && (site.slots[constructor.captures[constructor.enclosingField]] != 0 || site.slots[constructor.captures["val$seed"]] != 1 || site.slots[constructor.captures["val$token"]] != 5) {
								t.Fatalf("category2 captured parameter identity %+v", site)
							}
						}
					}
				})
			}
		})
	}
}

func TestNativeMethodLocalSourceParameterRejectsEqualTypedReplacement(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "same typed different slot", "same typed replaced seed", "missing physical registration", "not parameter", "custom text", "stack replacement", "different type", "this", "nil", "wrapper depth", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			seed := values.NewJavaLiteral("seed", typ)
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
			ref.IsParam = true
			if variant != "missing physical registration" {
				ref.MarkOriginalParameter(3)
			}
			var value values.JavaValue = ref
			slot := 3
			var work *workbudget.Budget
			switch variant {
			case "renamed":
				ref.Id = coreutils.NewRootVariableId()
			case "same typed different slot":
				slot = 5
			case "same typed replaced seed":
				ref.Val = values.NewJavaLiteral("seed", typ)
			case "not parameter":
				ref.IsParam = false
			case "custom text":
				ref.CustomValue = &values.CustomValue{}
			case "stack replacement":
				ref.StackVar = seed
			case "different type":
				value = values.NewJavaRef(coreutils.NewRootVariableId(), seed, types.NewJavaClass("java.lang.String"))
			case "this":
				ref.IsThis = true
			case "nil":
				value = (*values.JavaRef)(nil)
			case "wrapper depth":
				for i := 0; i < 40; i++ {
					value = values.NewSlotValue(value, typ)
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("fixture budget setup")
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			ctx := &class_context.ClassContext{ClassName: "LocalSiteOwner", IsStatic: false}
			if got := nativeMethodLocalParameterOperand(value, slot, "Ljava/lang/Object;", ctx, work); got != (variant == "original" || variant == "renamed") {
				t.Fatalf("source slot accepted %v", got)
			}
		})
	}
}
