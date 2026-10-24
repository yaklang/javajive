package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLocalOwnedAnonymousCaptureRequiresOriginalReceiverPath(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object retained(){return alias;}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "foreign receiver", "slot zero written", "capture write", "static capture read", "wrong descriptor", "branch inside chain", "handler inside chain", "cached field role", "cached field index", "cached constructor", "cached super argument", "cached source constructor", "missing parent row", "duplicate parent row", "missing local role", "foreign group owner", "nonfinal enclosing field", "budget", "memory", "canceled"} {
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
			group := p.memberAnonymous[local.object.GetClassName()]
			child := group.children["ChainOwner$1Local$1"]
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
			last := 4
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
				for _, a := range local.object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							n, _ := sourceBridgeClassName(local.object, row.InnerClassInfoIndex)
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

func TestNativeLocalOwnedAnonymousCaptureSourceRequiresSealedBinding(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object retained(){return alias;}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "unsealed receiver", "unsealed first read", "unsealed last read", "wrong PC", "wrong field", "foreign source ID", "changed binding", "stale code", "stale parent code", "foreign method", "field shadow"} {
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
			group := p.memberAnonymous[local.object.GetClassName()]
			child := group.children["ChainOwner$1Local$1"]
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
			read := local.anonymousCaptureReads[child.object.GetClassName()]["retained()Ljava/lang/Object;"][4]
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
			first := values.NewRefMember(receiver, read.prior.field, types.NewJavaClass(local.object.GetClassName()))
			first.OriginPC = read.prior.pc
			first.HasOriginPC = true
			if mode != "unsealed first read" {
				first.MarkOriginalFieldRead(&values.JavaClassMember{Name: read.prior.owner, Member: read.prior.field, Description: read.prior.descriptor}, read.prior.pc)
			}
			last := values.NewRefMember(first, read.field, types.NewJavaClass("java.lang.Object"))
			last.OriginPC = read.pc
			last.HasOriginPC = true
			if mode != "unsealed last read" {
				last.MarkOriginalFieldRead(&values.JavaClassMember{Name: read.owner, Member: read.field, Description: read.descriptor}, read.pc)
			}
			switch mode {
			case "wrong PC":
				last.OriginPC++
			case "wrong field":
				first.Member = "other"
			case "foreign source ID":
				local.sourceRefs["val$alias"] = values.NewJavaRef(utils.NewRootVariableId(), nil, binder.Type())
			case "changed binding":
				local.bindings["val$alias"] = "another"
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

func TestNativeLocalOwnedAnonymousAllocationRequiresOriginalThis(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object retained(){return alias;}};}}return new Local().create();}}`
	originals := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "owning receiver overwritten"} {
		t.Run(mode, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range originals {
				files[n] = append([]byte(nil), b...)
			}
			if mode != "original" {
				local, e := Parse(files["ChainOwner$1Local.class"])
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, m := range local.Methods {
					n, _ := sourceBridgeUTF8(local, m.NameIndex)
					if n != "create" {
						continue
					}
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							c.Code = append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_0)}, c.Code...)
							found = true
						}
					}
				}
				if !found {
					t.Fatal("allocation owner body")
				}
				files["ChainOwner$1Local.class"] = local.Bytes()
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ChainOwner.class"])
			prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
			emitted := false
			if prepared != nil {
				entry := z.finishNativeMemberFamily(prepared, z.nativeMemberLookup, false)
				emitted = entry != nil
			}
			if emitted != (mode == "original") {
				t.Fatalf("source emission admitted=%v", emitted)
			}
		})
	}
}

func TestNativeLocalOwnedAnonymousPublicationReopensInputFingerprints(t *testing.T) {
	source := `class ChainOwner {static Object make(final Object alias){class Local {Object create(){return new Object(){Object retained(){return alias;}};}}return new Local().create();}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ChainOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "child changed", "parent changed"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["ChainOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("group")
			}
			local := p.methodLocals["ChainOwner$1Local"]
			child := p.memberAnonymous[local.object.GetClassName()].children["ChainOwner$1Local$1"]
			if !d.nativeMethodLocalAnonymousCaptureUser(p, local, child.object.GetClassName(), "val$alias", nil) {
				t.Fatal("path")
			}
			c := NewClassObjectDumper(child.object)
			c.nativeMemberRoot = p
			if !c.nativeMethodLocalAnonymousInputsUnchanged() {
				t.Fatal("original seal")
			}
			if mode == "child changed" {
				child.object.AccessFlags |= 0x10
			}
			if mode == "parent changed" {
				local.object.AccessFlags |= 0x10
			}
			if c.nativeMethodLocalAnonymousInputsUnchanged() != (mode == "original") {
				t.Fatal("changed immutable input certified at publication")
			}
		})
	}
}
