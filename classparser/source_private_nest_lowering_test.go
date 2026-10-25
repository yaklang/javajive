package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Mutate independently compiled fresh classes, not a hand-built success packet.
// Check parsing separately so a malformed class-file length cannot accidentally
// make a semantic negative test pass.
func testPrivateNestCompletionClosure(t *testing.T, artifact *JarReconstructionArtifact, originals, fresh map[string][]byte) {
	t.Helper()
	authority := artifact.source.jar.sourceConstructorLowering
	variants := []string{"exact", "nest members reordered", "nest members missing", "nest members extra", "nest members duplicate", "wrong wide slot", "wrong reference slot", "helper effects", "helper flags", "helper throws", "helper handler", "helper target", "helper handle", "missing helper", "private target flags", "completed target flags", "extra caller", "consumer invocation kind", "consumer missing", "consumer wrong binding", "wrong nest host", "missing member source", "pending source", "stale original", "wrong source receipt", "work", "cancelled"}
	for _, variant := range variants {
		t.Run("closure/"+variant, func(t *testing.T) {
			owner := "NestOwner"
			if strings.HasPrefix(variant, "consumer ") || variant == "wrong nest host" {
				owner += "$Reader"
			}
			o := mustParseLowering(t, bytes.Clone(fresh[owner+".class"]))
			a := &sourceConstructorLoweringAuthority{jar: authority.jar, nestActive: map[string]constructorNativeSourceOrigin{}}
			for n, v := range authority.nestActive {
				a.nestActive[n] = v
			}
			var helper, target, ctor, read *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				d, _ := sourceBridgeUTF8(o, m.DescriptorIndex)
				switch {
				case strings.HasPrefix(n, "jdec$private$"):
					helper = m
				case n == "pick" && strings.Contains(d, "DLjava/lang/Object;"):
					target = m
				case n == "<init>":
					ctor = m
				case n == "read":
					read = m
				}
			}
			if owner == "NestOwner" && (helper == nil || target == nil || ctor == nil) || owner != "NestOwner" && read == nil {
				t.Fatal("fresh authored graph missing")
			}
			var work *workbudget.Budget
			switch variant {
			case "nest members reordered", "nest members missing", "nest members extra", "nest members duplicate":
				original := mustParseLowering(t, originals[owner+".class"])
				members, ok := originalNestAttribute(original, "NestMembers")
				if !ok || len(members) != 2 || hasOriginalNestAttribute(o, "NestMembers") {
					t.Fatal("independent flat/nested fixture boundary")
				}
				members[0], members[1] = members[1], members[0]
				switch variant {
				case "nest members missing":
					members = members[:1]
				case "nest members extra":
					members = append(members, "UnrelatedOwner")
				case "nest members duplicate":
					members[1] = members[0]
				}
				body := privateNestWord(uint16(len(members)))
				for _, member := range members {
					body = append(body, privateNestWord(uint16(o.ConstantPoolManager.AddNewClassInfo(member)))...)
				}
				o.ConstantPoolManager.AddUtf8Info("NestMembers")
				o.Attributes = append(o.Attributes, &UnparsedAttribute{Name: "NestMembers", Length: uint32(len(body)), Info: body})
			case "wrong wide slot":
				constructorLoweringCode(helper).Code[1] = core.OP_LLOAD_2
			case "wrong reference slot":
				c := constructorLoweringCode(helper)
				d := core.NewDecompiler(c.Code, nil)
				if d.ParseOpcode() != nil {
					t.Fatal("helper code")
				}
				ops := constructorMotionOps(d)
				c.Code[int(ops[3].CurrentOffset)+1] = 6
			case "helper effects":
				c := constructorLoweringCode(helper)
				c.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, c.Code...)
				c.AttrLen += 2
			case "helper flags":
				helper.AccessFlags |= 0x20
			case "helper throws":
				for _, at := range helper.Attributes {
					if ex, ok := at.(*ExceptionsAttribute); ok {
						ex.ExceptionIndexTable[0] = uint16(o.ConstantPoolManager.AddNewClassInfo("java/lang/RuntimeException"))
					}
				}
			case "helper handler":
				c := constructorLoweringCode(helper)
				c.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: uint16(len(c.Code) - 1)}}
				c.AttrLen += 8
			case "helper target":
				c := constructorLoweringCode(helper)
				ref := o.ConstantPoolManager.AddNewMethodInfo(owner, "pick", "(JDLjava/lang/String;[Ljava/lang/Object;)Ljava/lang/Object;")
				binary.BigEndian.PutUint16(c.Code[len(c.Code)-3:], uint16(ref))
			case "helper handle":
				n, _ := sourceBridgeUTF8(o, helper.NameIndex)
				d, _ := sourceBridgeUTF8(o, helper.DescriptorIndex)
				ref := o.ConstantPoolManager.AddNewMethodInfo(owner, n, d)
				o.ConstantPool = append(o.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 5, ReferenceIndex: uint16(ref)})
			case "missing helper":
				var keep []*MemberInfo
				for _, m := range o.Methods {
					if m != helper {
						keep = append(keep, m)
					}
				}
				o.Methods = keep
			case "private target flags":
				target.AccessFlags = 1
			case "completed target flags":
				b, err := a.lowerPrivateNestAdapters(originals[owner+".class"], fresh[owner+".class"], nil)
				if err != nil {
					t.Fatal(err)
				}
				o = mustParseLowering(t, b)
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n == "pick" {
						m.AccessFlags = 1
					}
				}
			case "extra caller":
				n, _ := sourceBridgeUTF8(o, helper.NameIndex)
				d, _ := sourceBridgeUTF8(o, helper.DescriptorIndex)
				ref := uint16(o.ConstantPoolManager.AddNewMethodInfo(owner, n, d))
				c := constructorLoweringCode(ctor)
				size := len(c.Code)
				c.Code = append(bytes.Clone(c.Code[:size-1]), core.OP_ALOAD_0, core.OP_LCONST_0, core.OP_DCONST_0, core.OP_ACONST_NULL, core.OP_ACONST_NULL, core.OP_INVOKEVIRTUAL, byte(ref>>8), byte(ref), core.OP_POP, core.OP_RETURN)
				c.AttrLen += uint32(len(c.Code) - size)
				c.MaxStack = 7
			case "consumer invocation kind", "consumer missing", "consumer wrong binding":
				c := constructorLoweringCode(read)
				d := core.NewDecompiler(c.Code, nil)
				if d.ParseOpcode() != nil {
					t.Fatal("consumer code")
				}
				changed := false
				for _, op := range constructorMotionOps(d) {
					if op.Instr.OpCode != core.OP_INVOKEVIRTUAL {
						continue
					}
					at := int(op.CurrentOffset)
					switch variant {
					case "consumer invocation kind":
						c.Code[at] = core.OP_INVOKESPECIAL
					case "consumer missing":
						copy(c.Code[at:at+3], []byte{core.OP_NOP, core.OP_NOP, core.OP_NOP})
					case "consumer wrong binding":
						ref := o.ConstantPoolManager.AddNewMethodInfo("NestOwner", "pick", "(JDLjava/lang/Object;[Ljava/lang/Object;)Ljava/lang/Object;")
						binary.BigEndian.PutUint16(c.Code[at+1:], uint16(ref))
					}
					changed = true
				}
				if !changed {
					t.Fatal("consumer edge missing")
				}
			case "wrong nest host":
				o.ConstantPoolManager.AddUtf8Info("NestHost")
				ref := uint16(o.ConstantPoolManager.AddNewClassInfo("UnrelatedOwner"))
				o.Attributes = append(o.Attributes, &UnparsedAttribute{Name: "NestHost", Length: 2, Info: privateNestWord(ref)})
			case "missing member source":
				delete(a.nestActive, "NestOwner$Child")
			case "pending source":
				a.nestPending = a.nestActive
				a.nestActive = nil
			case "stale original":
				v := a.nestActive[owner]
				v.originalHash[0] ^= 1
				a.nestActive[owner] = v
			case "wrong source receipt":
				v := a.nestActive[owner]
				v.sourceHash = [32]byte{}
				a.nestActive[owner] = v
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := o.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation did not reach semantic proof", err)
			}
			saved := bytes.Clone(input)
			out, err := a.lowerPrivateNestAdapters(originals[owner+".class"], input, work)
			if !bytes.Equal(saved, input) {
				t.Fatal("input mutated")
			}
			if variant == "pending source" || variant == "stale original" || variant == "wrong source receipt" {
				if err != nil || !bytes.Equal(out, input) {
					t.Fatal("unpublished domain changed", err)
				}
				return
			}
			if variant != "exact" && variant != "nest members reordered" {
				if err == nil || out != nil {
					t.Fatal("unproved graph exposed completion", err)
				}
				return
			}
			if err != nil {
				t.Fatal("proved closure", err)
			}
		})
	}
	// For every retained method, only the constant-pool operand of the proved
	// private call may change. PCs, handler/debug tables and stack/local bounds
	// must remain precisely those produced by fresh javac, not original Code.
	for name, input := range fresh {
		t.Run("fresh-code/"+name, func(t *testing.T) {
			out, err := authority.lowerPrivateNestAdapters(originals[name], input, nil)
			if err != nil {
				t.Fatal(err)
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			for _, am := range after.Methods {
				an, _ := sourceBridgeUTF8(after, am.NameIndex)
				ad, _ := sourceBridgeUTF8(after, am.DescriptorIndex)
				var bm *MemberInfo
				for _, m := range before.Methods {
					n, _ := sourceBridgeUTF8(before, m.NameIndex)
					d, _ := sourceBridgeUTF8(before, m.DescriptorIndex)
					if n == an && d == ad {
						bm = m
					}
				}
				if bm == nil {
					t.Fatal("invented declaration")
				}
				ac, bc := constructorLoweringCode(am), constructorLoweringCode(bm)
				if ac == nil || bc == nil {
					if ac != nil || bc != nil {
						t.Fatal("changed Code presence")
					}
					continue
				}
				expected := bytes.Clone(bc.Code)
				dec := core.NewDecompiler(bc.Code, nil)
				if dec.ParseOpcode() != nil {
					t.Fatal("fresh instructions")
				}
				for _, op := range constructorMotionOps(dec) {
					if op.Instr.OpCode != core.OP_INVOKEVIRTUAL {
						continue
					}
					at := int(op.CurrentOffset)
					key, ok := privateNestReferenceKey(before, binary.BigEndian.Uint16(bc.Code[at+1:]))
					if !ok || !strings.HasPrefix(key.name, "jdec$private$") {
						continue
					}
					got, ok := privateNestReferenceKey(after, binary.BigEndian.Uint16(ac.Code[at+1:]))
					if !ok || got.owner != key.owner || got.descriptor != key.descriptor || got.name != "pick" {
						t.Fatal("private target changed")
					}
					copy(expected[at+1:at+3], ac.Code[at+1:at+3])
				}
				if !bytes.Equal(expected, ac.Code) || bc.MaxStack != ac.MaxStack || bc.MaxLocals != ac.MaxLocals || !reflect.DeepEqual(bc.ExceptionTable, ac.ExceptionTable) || !reflect.DeepEqual(bc.Attributes, ac.Attributes) {
					t.Fatal("fresh method changed outside proved constant operands")
				}
			}
		})
	}
}
