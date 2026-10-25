package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInheritedGenericBridgeRequiresExactSuperclassSubstitution(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedGenericSuperFixture(t), "BridgeOuter", "none", false)
	for _, variant := range []string{"exact", "weaker scalar check", "weaker array check", "missing check", "virtual dispatch", "wrong wide slot", "raw superclass", "wrong substitution", "unbound substitution", "wrong parent signature", "duplicate signature", "method formal", "missing type bound", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, b := range files {
				objects[n] = mustParseLowering(t, bytes.Clone(b))
			}
			obj, parent := objects["BridgeOuter$Leaf.class"], objects["BridgeOuter.class"]
			var bridge, target *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				if n == "exchange" && m.AccessFlags == 1 {
					target = m
				}
			}
			if bridge == nil || target == nil {
				t.Fatal("authored declarations missing")
			}
			code := constructorLoweringCode(bridge)
			d := core.NewDecompiler(code.Code, nil)
			if d.ParseOpcode() != nil {
				t.Fatal("original code")
			}
			ops := constructorMotionOps(d)
			var casts []int
			for _, op := range ops {
				if op.Instr.OpCode == core.OP_CHECKCAST {
					casts = append(casts, int(op.CurrentOffset))
				}
			}
			if len(casts) != 2 {
				t.Fatal("original checks")
			}
			setSignature := func(o *ClassObject, attrs *[]AttributeInfo, s string) {
				var kept []AttributeInfo
				for _, a := range *attrs {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				*attrs = append(kept, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(o.ConstantPoolManager.AddUtf8Info(s))})
			}
			var work *workbudget.Budget
			switch variant {
			case "weaker scalar check":
				binary.BigEndian.PutUint16(code.Code[casts[0]+1:], uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/Comparable")))
			case "weaker array check":
				binary.BigEndian.PutUint16(code.Code[casts[1]+1:], uint16(obj.ConstantPoolManager.AddNewClassInfo("[Ljava/lang/Comparable;")))
			case "missing check":
				copy(code.Code[casts[0]:casts[0]+3], []byte{core.OP_NOP, core.OP_NOP, core.OP_NOP})
			case "virtual dispatch":
				code.Code[int(ops[len(ops)-2].CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "wrong wide slot":
				code.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
			case "raw superclass":
				setSignature(obj, &obj.Attributes, "<X::LBridgeMarker<TX;>;>LBridgeOuter;")
			case "wrong substitution":
				setSignature(obj, &obj.Attributes, "<X::LBridgeMarker<TX;>;>LBridgeOuter<Ljava/lang/String;>;")
			case "unbound substitution":
				setSignature(obj, &obj.Attributes, "<X::LBridgeMarker<TX;>;>LBridgeOuter<TMissing;>;")
			case "wrong parent signature":
				setSignature(obj, &obj.Attributes, "<X::LBridgeMarker<TX;>;>Ljava/lang/Object;")
			case "duplicate signature":
				for _, a := range obj.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						obj.Attributes = append(obj.Attributes, a)
						break
					}
				}
			case "method formal":
				setSignature(parent, &target.Attributes, "<X::Ljava/lang/Comparable<TX;>;>(JDTX;[TX;)Ljava/lang/String;")
			case "missing type bound":
				delete(objects, "BridgeMarker.class")
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			reader := NewClassObjectDumper(obj)
			reader.Work = work
			reader.foldSiblingResolver = func(n string) ([]byte, bool) {
				o, k := objects[n+".class"]
				if !k {
					return nil, false
				}
				return o.Bytes(), true
			}
			p := reader.inheritedBridgeOriginal(bridge)
			if (p != nil) != (variant == "exact") {
				t.Fatalf("accepted=%v", p != nil)
			}
			if p != nil && (p.sourceDescriptor != "(JDLBridgeMarker;[LBridgeMarker;)Ljava/lang/String;" || p.targetDescriptor != "(JDLjava/lang/Comparable;[Ljava/lang/Comparable;)Ljava/lang/String;") {
				t.Fatalf("lost source/physical distinction: %+v", p)
			}
		})
	}
}

// These are mutations of independently compiled Java, not hand-built success
// packets. Both removed nodes must be private to this compilation graph, and
// every fresh CHECKCAST must remain in the surviving original bridge.
func testInheritedGenericBridgeFreshClosure(t *testing.T, owner string, original, fresh []byte, a *JarReconstructionArtifact) {
	t.Helper()
	for _, variant := range []string{"exact", "weaker surviving check", "weaker extra check", "extra flags", "extra effects", "extra handle", "extra caller", "virtual super", "helper signature", "helper throws", "swapped wide slot"} {
		t.Run(variant, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(fresh))
			var bridge, helper, extra, ctor *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				d, _ := sourceBridgeUTF8(o, m.DescriptorIndex)
				if n == "<init>" {
					ctor = m
				} else if n == "exchange" {
					switch d {
					case "(JDLjava/lang/Object;[Ljava/lang/Object;)Ljava/lang/String;":
						bridge = m
					case "(JDLBridgeMarker;[LBridgeMarker;)Ljava/lang/String;":
						helper = m
					case "(JDLjava/lang/Comparable;[Ljava/lang/Comparable;)Ljava/lang/String;":
						extra = m
					}
				}
			}
			if bridge == nil || helper == nil || extra == nil || ctor == nil {
				t.Fatal("independently compiled graph missing")
			}
			forward := inheritedBridgeForwardPacket(o, bridge, core.OP_INVOKEVIRTUAL, nil)
			target := inheritedBridgeForwardPacket(o, helper, core.OP_INVOKESPECIAL, nil, true)
			if forward == nil || target == nil {
				t.Fatal("fresh graph grammar")
			}
			reference := uint16(o.ConstantPoolManager.AddNewMethodInfo(owner, "exchange", "(JDLjava/lang/Comparable;[Ljava/lang/Comparable;)Ljava/lang/String;"))
			switch variant {
			case "weaker surviving check", "weaker extra check":
				m := bridge
				if variant == "weaker extra check" {
					m = extra
				}
				c := constructorLoweringCode(m)
				d := core.NewDecompiler(c.Code, nil)
				if d.ParseOpcode() != nil {
					t.Fatal("fresh parse")
				}
				changed := false
				for _, op := range constructorMotionOps(d) {
					if op.Instr.OpCode == core.OP_CHECKCAST {
						binary.BigEndian.PutUint16(c.Code[int(op.CurrentOffset)+1:], uint16(o.ConstantPoolManager.AddNewClassInfo("java/lang/Comparable")))
						changed = true
						break
					}
				}
				if !changed {
					t.Fatal("fresh check missing")
				}
			case "extra flags":
				extra.AccessFlags = 1
			case "extra effects":
				c := constructorLoweringCode(extra)
				c.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, c.Code...)
				c.AttrLen += 2
			case "extra handle":
				o.ConstantPool = append(o.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 5, ReferenceIndex: reference})
			case "extra caller":
				c := constructorLoweringCode(ctor)
				priorSize := len(c.Code)
				c.Code = append(bytes.Clone(c.Code[:len(c.Code)-1]), core.OP_ALOAD_0, core.OP_LCONST_0, core.OP_DCONST_0, core.OP_ACONST_NULL, core.OP_ACONST_NULL, core.OP_INVOKEVIRTUAL, byte(reference>>8), byte(reference), core.OP_POP, core.OP_RETURN)
				c.AttrLen += uint32(len(c.Code) - priorSize)
				c.MaxStack = 7
			case "virtual super":
				constructorLoweringCode(helper).Code[target.invokePC] = core.OP_INVOKEVIRTUAL
			case "helper signature":
				changed := false
				for _, at := range helper.Attributes {
					if s, ok := at.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(o.ConstantPoolManager.AddUtf8Info("(JDLjava/lang/Object;[Ljava/lang/Object;)Ljava/lang/String;"))
						changed = true
					}
				}
				if !changed {
					t.Fatal("expected generic helper declaration")
				}
			case "helper throws":
				for _, at := range helper.Attributes {
					if ex, ok := at.(*ExceptionsAttribute); ok {
						ex.ExceptionIndexTable = []uint16{uint16(o.ConstantPoolManager.AddNewClassInfo("java/lang/RuntimeException"))}
					}
				}
			case "swapped wide slot":
				constructorLoweringCode(helper).Code[1] = core.OP_LLOAD_2
			}
			input := o.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation did not reach graph proof", err)
			}
			saved := bytes.Clone(input)
			out, err := a.source.jar.sourceConstructorLowering.lowerInheritedBridges(owner, original, input, nil)
			if err != nil || !bytes.Equal(input, saved) {
				t.Fatal("completion altered input", err)
			}
			if variant != "exact" {
				if !bytes.Equal(out, input) {
					t.Fatal("unproved graph exposed a partial completion")
				}
				return
			}
			got := mustParseLowering(t, out)
			want := mustParseLowering(t, original)
			if len(got.Methods) != len(want.Methods) {
				t.Fatal("temporary graph remains")
			}
			var surviving *MemberInfo
			for _, m := range got.Methods {
				if m.AccessFlags == 0x1041 {
					surviving = m
				}
			}
			if surviving == nil {
				t.Fatal("original bridge missing")
			}
			before, after := constructorLoweringCode(bridge), constructorLoweringCode(surviving)
			expected := bytes.Clone(before.Code)
			expected[forward.invokePC] = core.OP_INVOKESPECIAL
			binary.BigEndian.PutUint16(expected[forward.invokePC+1:], target.invokeIndex)
			if !bytes.Equal(after.Code, expected) || after.MaxLocals != before.MaxLocals || after.MaxStack != before.MaxStack {
				t.Fatal("completion changed fresh executable code outside proven super call")
			}
		})
	}
}

func TestInheritedGenericBridgeStaticScopeRequiresOriginalDeclaration(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedAnonymousGenericSuperFixture(t), "BridgeOuter", "none", false)
	for _, variant := range []string{"exact", "missing owner", "missing enclosing method", "wrong owner", "initializer edge", "duplicate edge", "changed bound", "free type variable"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, b := range files {
				objects[n] = mustParseLowering(t, bytes.Clone(b))
			}
			obj, owner := objects["BridgeOuter$1.class"], objects["BridgeOuter.class"]
			var bridge, method *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					bridge = m
				}
			}
			for _, m := range owner.Methods {
				n, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if n == "leaf" {
					method = m
				}
			}
			var edge *UnparsedAttribute
			for _, a := range obj.Attributes {
				if r, ok := a.(*UnparsedAttribute); ok && r.Name == "EnclosingMethod" {
					edge = r
				}
			}
			if bridge == nil || method == nil || edge == nil || method.AccessFlags&StaticFlag == 0 {
				t.Fatal("static anonymous original graph")
			}
			switch variant {
			case "missing owner":
				delete(objects, "BridgeOuter.class")
			case "missing enclosing method":
				var keep []*MemberInfo
				for _, m := range owner.Methods {
					if m != method {
						keep = append(keep, m)
					}
				}
				owner.Methods = keep
			case "wrong owner":
				binary.BigEndian.PutUint16(edge.Info[:2], uint16(obj.ConstantPoolManager.AddNewClassInfo("UnrelatedScope")))
			case "initializer edge":
				binary.BigEndian.PutUint16(edge.Info[2:], 0)
			case "duplicate edge":
				obj.Attributes = append(obj.Attributes, edge)
			case "changed bound", "free type variable":
				sig := "<X:Ljava/lang/String;>()Ljava/lang/Object;"
				if variant == "free type variable" {
					sig = "<X:TMissing;>()Ljava/lang/Object;"
				}
				changed := false
				for _, a := range method.Attributes {
					if s, ok := a.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(owner.ConstantPoolManager.AddUtf8Info(sig))
						changed = true
					}
				}
				if !changed {
					t.Fatal("missing method type declaration")
				}
			}
			d := NewClassObjectDumper(obj)
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				o, k := objects[n+".class"]
				if !k {
					return nil, false
				}
				return o.Bytes(), true
			}
			got := d.inheritedBridgeOriginal(bridge)
			if (got != nil) != (variant == "exact") {
				t.Fatalf("accepted=%v", got != nil)
			}
		})
	}
}
