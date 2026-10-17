package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"sync"
	"testing"
)

func TestNativeLocalNestedAnonymousCaptureRequiresEveryOriginalEdge(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object open(){return new Object(){Object retained(){return alias;}};}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "foreign receiver", "slot zero written", "capture write", "static capture read", "wrong descriptor", "branch inside chain", "handler inside chain", "cached field role", "cached field index", "cached constructor", "cached super argument", "cached source constructor", "missing parent row", "duplicate parent row", "missing local role", "foreign group owner", "nonfinal enclosing field", "missing intermediate unit", "missing forest role", "stale intermediate capture", "stale intermediate code", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["ChainOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("physical group")
			}
			local := p.methodLocals["ChainOwner$1Local"]
			group := p.anonymousUnits["ChainOwner$1Local$1$1"]
			child := group.children["ChainOwner$1Local$1$1"]
			if child == nil {
				t.Fatal("physical anonymous child")
			}
			var code *CodeAttribute
			for _, m := range child.object.Methods {
				n, _ := sourceBridgeUTF8(child.object, m.NameIndex)
				if n == "retained" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil || len(code.Code) < 8 {
				t.Fatal("original two field path")
			}
			last := 7
			switch mode {
			case "foreign receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "slot zero written":
				code.Code = append(code.Code, byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_0), byte(core.OP_ARETURN))
			case "capture write":
				code.Code[last] = byte(core.OP_PUTFIELD)
			case "static capture read":
				code.Code[last] = byte(core.OP_GETSTATIC)
			case "wrong descriptor":
				index := int(binary.BigEndian.Uint16(code.Code[last+1:]))
				f := child.object.ConstantPool[index-1].(*ConstantFieldrefInfo)
				nt := child.object.ConstantPool[f.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				pool := NewConstantPoolWithConstant(&child.object.ConstantPool)
				nt.DescriptorIndex = uint16(pool.AddUtf8Info("Ljava/lang/String;"))
			case "branch inside chain":
				pc := len(code.Code)
				delta := last - pc
				code.Code = append(code.Code, byte(core.OP_GOTO), byte(delta>>8), byte(delta))
			case "handler inside chain":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: uint16(len(code.Code)), HandlerPc: uint16(len(code.Code) - 1)}}
			case "cached field role":
				child.enclosingField = "missing"
			case "cached field index":
				child.fields[child.enclosingField] = 1
			case "cached constructor":
				child.descriptor = "(Ljava/lang/Object;)V"
			case "cached super argument":
				child.superParams = append(child.superParams, 0)
			case "cached source constructor":
				child.sourceSuperDescriptor = "(Ljava/lang/Object;)V"
			case "missing parent row", "duplicate parent row":
				for _, a := range p.lexicalObjects[group.owner].Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							n, _ := sourceBridgeClassName(p.lexicalObjects[group.owner], row.InnerClassInfoIndex)
							if n == child.object.GetClassName() {
								if mode == "missing parent row" {
									table.Classes = append(table.Classes[:i], table.Classes[i+1:]...)
								} else {
									table.Classes = append(table.Classes, row)
								}
								break
							}
						}
					}
				}
			case "missing local role":
				delete(p.methodLocals, local.object.GetClassName())
			case "foreign group owner":
				group.owner = "Other"
			case "nonfinal enclosing field":
				for _, f := range child.object.Fields {
					n, _ := sourceBridgeUTF8(child.object, f.NameIndex)
					if n == child.enclosingField {
						f.AccessFlags = 0
					}
				}
			case "missing intermediate unit":
				delete(p.anonymousUnits, "ChainOwner$1Local$1")
			case "missing forest role":
				delete(p.anonymousForest.units, "ChainOwner$1Local$1")
			case "stale intermediate capture":
				p.anonymousForest.units["ChainOwner$1Local$1"].enclosingField = "missing"
			case "stale intermediate code":
				intermediate := p.anonymousForest.units["ChainOwner$1Local$1"].object
				for _, m := range intermediate.Methods {
					n, _ := sourceBridgeUTF8(intermediate, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code[1] = byte(core.OP_ALOAD_0)
							}
						}
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.nativeMethodLocalAnonymousCaptureUser(p, local, child.object.GetClassName(), "val$alias", d.Work)
			if got != (mode == "original") {
				t.Fatalf("proof %v", got)
			}
			if !got && len(local.anonymousCaptureReads) != 0 {
				t.Fatal("partial path evidence committed")
			}
		})
	}
}

func TestNativeLocalNestedAnonymousCaptureSourceRequiresEverySealedOperand(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object open(){return new Object(){Object retained(){return alias;}};}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "failed candidate", "stale declaring owner code", "removed local role", "removed intermediate unit", "unsealed receiver", "unsealed first read", "unsealed middle read", "stale intermediate code", "stale intermediate packet", "unsealed last read", "wrong PC", "wrong field", "foreign source ID", "changed binding", "stale code", "stale parent code", "foreign method", "field shadow"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ChainOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("owned group")
			}
			local := p.methodLocals["ChainOwner$1Local"]
			group := p.anonymousUnits["ChainOwner$1Local$1$1"]
			child := group.children["ChainOwner$1Local$1$1"]
			if mode == "field shadow" {
				pool := NewConstantPoolWithConstant(&child.object.ConstantPool)
				child.object.Fields = append(child.object.Fields, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info("alias")), DescriptorIndex: uint16(pool.AddUtf8Info("Ljava/lang/Object;"))})
			}
			if !d.nativeMethodLocalAnonymousCaptureUser(p, local, child.object.GetClassName(), "val$alias", nil) {
				t.Fatal("original physical reads")
			}
			var method *MemberInfo
			for _, m := range child.object.Methods {
				n, _ := sourceBridgeUTF8(child.object, m.NameIndex)
				if n == "retained" {
					method = m
				}
			}
			read := local.anonymousCaptureReads[child.object.GetClassName()]["retained()Ljava/lang/Object;"][7]
			if read == nil || read.prior == nil {
				t.Fatal("physical two-hop receipt")
			}
			binder := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			local.bindings = map[string]string{"val$alias": "alias"}
			local.sourceRefs = map[string]*values.JavaRef{"val$alias": binder}
			local.captureIDs = map[string]*utils.VariableId{"val$alias": binder.Id}
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(child.object.GetClassName()))
			receiver.IsParam = true
			receiver.MarkOriginalParameter(0)
			receiver.IsThis = true
			if mode != "unsealed receiver" {
				receiver.MarkOriginalReceiver()
			}
			firstRead := read.prior.prior
			first := values.NewRefMember(receiver, firstRead.field, types.NewJavaClass("ChainOwner$1Local$1"))
			first.OriginPC = firstRead.pc
			first.HasOriginPC = true
			if mode != "unsealed first read" {
				first.MarkOriginalFieldRead(&values.JavaClassMember{Name: firstRead.owner, Member: firstRead.field, Description: firstRead.descriptor}, firstRead.pc)
			}
			middle := values.NewRefMember(first, read.prior.field, types.NewJavaClass(local.object.GetClassName()))
			middle.OriginPC = read.prior.pc
			middle.HasOriginPC = true
			if mode != "unsealed middle read" {
				middle.MarkOriginalFieldRead(&values.JavaClassMember{Name: read.prior.owner, Member: read.prior.field, Description: read.prior.descriptor}, read.prior.pc)
			}
			last := values.NewRefMember(middle, read.field, types.NewJavaClass("java.lang.Object"))
			last.OriginPC = read.pc
			last.HasOriginPC = true
			if mode != "unsealed last read" {
				last.MarkOriginalFieldRead(&values.JavaClassMember{Name: read.owner, Member: read.field, Description: read.descriptor}, read.pc)
			}
			switch mode {
			case "failed candidate":
				p.failed = true
			case "stale declaring owner code":
				for _, method := range p.lexicalObjects[p.owner].Methods {
					for _, attribute := range method.Attributes {
						if code, ok := attribute.(*CodeAttribute); ok {
							code.Code = append(code.Code, byte(core.OP_NOP))
						}
					}
				}
			case "removed local role":
				delete(p.methodLocals, local.object.GetClassName())
			case "removed intermediate unit":
				delete(p.anonymousUnits, "ChainOwner$1Local$1")
			case "wrong PC":
				last.OriginPC++
			case "wrong field":
				first.Member = "other"
			case "foreign source ID":
				local.sourceRefs["val$alias"] = values.NewJavaRef(utils.NewRootVariableId(), nil, binder.Type())
			case "changed binding":
				local.bindings["val$alias"] = "another"
			case "stale intermediate code":
				p.anonymousForest.units["ChainOwner$1Local$1"].object.AccessFlags |= 0x10
			case "stale intermediate packet":
				p.anonymousForest.units["ChainOwner$1Local$1"].enclosingField = "missing"
			case "stale code":
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code = append(code.Code, byte(core.OP_NOP))
					}
				}
			case "stale parent code":
				local.object.AccessFlags |= 0x10
			case "foreign method":
				copy := *method
				method = &copy
			}
			c := NewClassObjectDumper(child.object)
			c.nativeMemberRoot = p
			c.CurrentMethod = method
			c.FuncCtx = &class_context.ClassContext{ClassName: child.object.GetClassName(), FunctionName: "retained", CurrentMethodDesc: "()Ljava/lang/Object;"}
			c.nativeAnonymousForest = p.anonymousForest
			c.wireNativeAnonymousForestCaptures(c.FuncCtx)
			c.wireNativeMethodLocalAnonymousCaptures()
			got := false
			if c.FuncCtx.SourceLexicalCapturedField != nil {
				_, got = c.FuncCtx.SourceLexicalCapturedField(last, read.pc, read.field)
			}
			if got != (mode == "original") {
				t.Fatalf("source permission %v", got)
			}
			if mode == "original" && !c.nativeLexicalCaptures()["alias"] {
				t.Fatal("lexical name barrier absent")
			}
		})
	}
}

func TestNativeLocalNestedSourceQueriesShareOnePublishedReceipt(t *testing.T) {
	const source = `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object open(){return new Object(){Object retained(){return alias;}};}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, order := range [][]string{{"ChainOwner$1Local$1$1", "ChainOwner$1Local$1", "ChainOwner$1Local", "ChainOwner"}, {"ChainOwner", "ChainOwner$1Local", "ChainOwner$1Local$1", "ChainOwner$1Local$1$1"}, {"ChainOwner$1Local", "ChainOwner$1Local$1$1", "ChainOwner", "ChainOwner$1Local$1"}} {
		t.Run(strings.Join(order, ":"), func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			objects := map[string]*ClassObject{}
			for _, name := range order {
				object, e := Parse(files[name+".class"])
				if e != nil {
					t.Fatal(e)
				}
				objects[name] = object
				owner, known := z.nativeMemberOriginalSourceOwner(object)
				transactionOwner, transactionKnown := z.nativeMemberTransactionOwner(object)
				if !known || owner != "ChainOwner" || !transactionKnown || transactionOwner != owner {
					t.Fatalf("original routing %s -> %s/%v, transaction %s/%v", name, owner, known, transactionOwner, transactionKnown)
				}
			}
			for _, name := range order {
				raw, e := z.ReadFile(name + ".class")
				if e != nil {
					t.Fatal(e)
				}
				if name != "ChainOwner" && !strings.HasPrefix(string(raw), "// original ") {
					t.Fatalf("independent source escaped completed lexical family for %s: %s", name, raw)
				}
			}
			receipt := z.nativeMemberEntry(objects["ChainOwner"])
			if receipt == nil || receipt.family == nil || receipt.family.anonymousForest == nil {
				t.Fatal("no completed lexical forest")
			}
			for _, name := range order {
				if z.nativeMemberEntry(objects[name]) != receipt {
					t.Fatalf("different ownership receipt for %s", name)
				}
			}
			before := z.nativeMembersBytes
			var group sync.WaitGroup
			errors := make(chan string, 16)
			for i := 0; i < 16; i++ {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					name := order[i%len(order)]
					if _, err := z.ReadFile(name + ".class"); err != nil || z.nativeMemberEntry(objects[name]) != receipt {
						errors <- name
					}
				}(i)
			}
			group.Wait()
			close(errors)
			for name := range errors {
				t.Errorf("concurrent ownership %s", name)
			}
			if z.nativeMembersBytes != before {
				t.Fatal("warm queries reserved lexical source again")
			}
		})
	}
}
