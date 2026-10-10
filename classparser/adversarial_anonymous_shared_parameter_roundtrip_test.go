package javaclassparser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

const anonymousSharedParameterFixture = `class SharedParameterEffects{static String trace="";static boolean fail;static SharedParameterBase published;static final RuntimeException error=new RuntimeException("identity");}
abstract class SharedParameterBase{final SharedParameterOwner owner;final Object token;final long word;SharedParameterBase(SharedParameterOwner owner,Object token,long word){SharedParameterEffects.trace+="P";SharedParameterEffects.published=this;if(ownerRead()!=owner||tokenRead()!=token||wordRead()!=word)throw new AssertionError("capture before SUPER callback");this.owner=owner;this.token=token;this.word=word;if(SharedParameterEffects.fail)throw SharedParameterEffects.error;}abstract SharedParameterOwner ownerRead();abstract Object tokenRead();abstract long wordRead();abstract long compute();}
class SharedParameterOwner{long seed;SharedParameterOwner(long seed){this.seed=seed;}SharedParameterBase make(final Object keptToken,final long keptWord){return new SharedParameterBase(this,keptToken,keptWord){SharedParameterOwner ownerRead(){return SharedParameterOwner.this;}Object tokenRead(){return keptToken;}long wordRead(){return keptWord;}long compute(){SharedParameterEffects.trace+="R";return keptWord*31+SharedParameterOwner.this.seed;}};}}
class SharedParameterDriver{public static void main(String[]a){Object identity=new Object();int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(boolean fail:new boolean[]{false,true}){SharedParameterOwner owner=new SharedParameterOwner(seed);SharedParameterEffects.trace="";SharedParameterEffects.fail=fail;SharedParameterEffects.published=null;try{SharedParameterBase view=owner.make(token,word);if(fail||view!=SharedParameterEffects.published||view.owner!=owner||view.token!=token||view.word!=word||!SharedParameterEffects.trace.equals("P"))throw new AssertionError("constructor identity/order");long expected=java.math.BigInteger.valueOf(word).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(seed)).longValue();if(view.compute()!=expected||!SharedParameterEffects.trace.equals("PR"))throw new AssertionError("overflow oracle/order");owner.seed=~seed;expected=java.math.BigInteger.valueOf(word).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(~seed)).longValue();if(view.compute()!=expected||view.tokenRead()!=token||view.ownerRead()!=owner||view.wordRead()!=word||!SharedParameterEffects.trace.equals("PRR"))throw new AssertionError("live enclosing/captured identity");}catch(RuntimeException e){SharedParameterBase partial=SharedParameterEffects.published;if(!fail||e!=SharedParameterEffects.error||partial==null||partial.owner!=owner||partial.token!=token||partial.word!=word||partial.ownerRead()!=owner||partial.tokenRead()!=token||partial.wordRead()!=word||!SharedParameterEffects.trace.equals("P"))throw new AssertionError("exception/publication/capture identity");}rows++;}System.out.println(rows+":anonymous:shared-parameter:overflow:callback:identity");}}
`

// A legal JVM compiler representation can reuse an unchanged parameter for a
// capture store and SUPER. Construct that representation without foreign bytes:
// collapse only equal descriptor/load operands in the linear original packet,
// and update the one original allocation to pass that operand once. The JVM
// runs this modified input before any decompiler or rebuilt-source assertion.
func anonymousSharedParameterClasses(t *testing.T, source, owner, debug string, shared map[string]bool) map[string][]byte {
	t.Helper()
	files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
	child, e := Parse(append([]byte(nil), files[owner+"$1.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	root, e := Parse(append([]byte(nil), files[owner+".class"]...))
	if e != nil {
		t.Fatal(e)
	}
	var ctor *MemberInfo
	var ctorCode, callerCode *CodeAttribute
	for _, m := range child.Methods {
		if n, _ := sourceBridgeUTF8(child, m.NameIndex); n == "<init>" {
			ctor = m
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					ctorCode = c
				}
			}
		}
	}
	for _, m := range root.Methods {
		if n, _ := sourceBridgeUTF8(root, m.NameIndex); n == "make" {
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					callerCode = c
				}
			}
		}
	}
	if ctor == nil || ctorCode == nil || callerCode == nil || len(ctorCode.ExceptionTable) != 0 || len(callerCode.ExceptionTable) != 0 {
		t.Fatal("original linear allocation/constructor")
	}
	oldDescriptor, _ := sourceBridgeUTF8(child, ctor.DescriptorIndex)
	params, ret, e := callbinding.Descriptor(oldDescriptor)
	if e != nil || ret != "V" {
		t.Fatal("original constructor descriptor")
	}
	ops := func(obj *ClassObject, code *CodeAttribute) []*core.OpCode {
		d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		if e := d.ParseOpcode(); e != nil {
			t.Fatal(e)
		}
		return constructorMotionOps(d)
	}
	callerOps := ops(root, callerCode)
	if len(callerOps) != len(params)+4 || callerOps[0].Instr.OpCode != core.OP_NEW || callerOps[1].Instr.OpCode != core.OP_DUP || callerOps[len(callerOps)-1].Instr.OpCode != core.OP_ARETURN {
		t.Fatal("only untouched argument loads may be collapsed")
	}
	call := callerOps[len(callerOps)-2]
	member := constructorMotionMember(root, call, core.OP_INVOKESPECIAL)
	if member == nil || member.Name != child.GetClassName() || member.Member != "<init>" || member.Description != oldDescriptor {
		t.Fatal("exact original allocation")
	}
	alias := make([]int, len(params))
	kept := []int{}
	for i, p := range params {
		load := callerOps[2+i]
		if !constructorMotionLoad(load, p) {
			t.Fatal("original argument load")
		}
		alias[i] = i
		if shared[p] {
			for _, earlier := range kept {
				if params[earlier] == p && core.GetRetrieveIdx(callerOps[2+earlier]) == core.GetRetrieveIdx(load) {
					alias[i] = earlier
					break
				}
			}
		}
		if alias[i] == i {
			kept = append(kept, i)
		}
	}
	if len(kept) == len(params) {
		t.Fatal("MVP did not actually reuse a physical parameter")
	}
	newParams, newSlots := []string{}, map[int]int{}
	slot := 1
	for _, index := range kept {
		newParams = append(newParams, params[index])
		newSlots[index] = slot
		slot++
		if params[index] == "J" || params[index] == "D" {
			slot++
		}
	}
	descriptor := "(" + strings.Join(newParams, "") + ")V"
	oldSlots := constructorParameterSlots(params)
	loadBytes := func(p string, slot int) []byte {
		op := byte(core.OP_ALOAD)
		switch p {
		case "J":
			op = core.OP_LLOAD
		case "D":
			op = core.OP_DLOAD
		case "F":
			op = core.OP_FLOAD
		case "Z", "B", "C", "S", "I":
			op = core.OP_ILOAD
		}
		return []byte{op, byte(slot)}
	}
	packet := []byte{}
	for _, op := range ops(child, ctorCode) {
		if index, known := oldSlots[core.GetRetrieveIdx(op)]; known && constructorMotionLoad(op, params[index]) {
			packet = append(packet, loadBytes(params[index], newSlots[alias[index]])...)
		} else {
			packet = append(packet, byte(op.Instr.OpCode))
			packet = append(packet, op.Data...)
		}
	}
	ctor.DescriptorIndex = uint16(child.ConstantPoolManager.AddUtf8Info(descriptor))
	ctor.Attributes = []AttributeInfo{ctorCode}
	ctorCode.Code, ctorCode.MaxLocals, ctorCode.Attributes = packet, uint16(slot), nil
	ctorCode.AttrLen = uint32(12 + len(packet))
	packet = []byte{byte(callerOps[0].Instr.OpCode)}
	packet = append(packet, callerOps[0].Data...)
	packet = append(packet, core.OP_DUP)
	for _, index := range kept {
		op := callerOps[2+index]
		packet = append(packet, byte(op.Instr.OpCode))
		packet = append(packet, op.Data...)
	}
	ref := root.ConstantPool[int(binary.BigEndian.Uint16(call.Data))-1].(*ConstantMethodrefInfo)
	nt := root.ConstantPool[int(ref.NameAndTypeIndex)-1].(*ConstantNameAndTypeInfo)
	nt.DescriptorIndex = uint16(root.ConstantPoolManager.AddUtf8Info(descriptor))
	packet = append(packet, byte(call.Instr.OpCode))
	packet = append(packet, call.Data...)
	packet = append(packet, core.OP_ARETURN)
	callerCode.Code, callerCode.Attributes = packet, nil
	callerCode.AttrLen = uint32(12 + len(packet))
	files[owner+".class"], files[owner+"$1.class"] = root.Bytes(), child.Bytes()
	return files
}

func testAnonymousSharedParameterRoundTrip(t *testing.T, role string) {
	for _, owner := range []string{"SharedParameterOwner", "IndependentSharedParameterScope"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(anonymousSharedParameterFixture, "SharedParameterOwner", owner)
			shared := map[string]bool{}
			switch role {
			case "outer":
				shared["L"+owner+";"] = true
			case "reference":
				shared["Ljava/lang/Object;"] = true
			case "wide":
				shared["J"] = true
			case "all":
				shared["L"+owner+";"], shared["Ljava/lang/Object;"], shared["J"] = true, true, true
			default:
				t.Fatal(role)
			}
			testNativePrivateSetterCompiledFixtureWithShape(t, owner, "SharedParameterDriver", "100:anonymous:shared-parameter:overflow:callback:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return anonymousSharedParameterClasses(t, fixture, owner, debug, shared)
			}, func(t *testing.T, name string, original, rebuilt []byte) {
				if name != owner+"$1.class" {
					if nativeBinaryShape(t, original) != nativeBinaryShape(t, rebuilt) {
						t.Fatal("unrelated ABI changed")
					}
					return
				}
				// Recompilation allocates distinct hidden capture parameters. Check
				// the explicit, independently identified representation difference;
				// retain every non-constructor declaration and capture field.
				a, e := Parse(original)
				if e != nil {
					t.Fatal(e)
				}
				b, e := Parse(rebuilt)
				if e != nil {
					t.Fatal(e)
				}
				constructors := 0
				for _, m := range b.Methods {
					if n, _ := sourceBridgeUTF8(b, m.NameIndex); n == "<init>" {
						constructors++
						desc, _ := sourceBridgeUTF8(b, m.DescriptorIndex)
						if m.AccessFlags != 0 || desc != "(L"+owner+";L"+owner+";Ljava/lang/Object;JLjava/lang/Object;J)V" {
							t.Fatalf("unexpected regenerated physical constructor: flags=%x descriptor=%s", m.AccessFlags, desc)
						}
					}
				}
				if constructors != 1 {
					t.Fatal("regenerated anonymous constructor count")
				}
				stripConstructor := func(o *ClassObject) {
					methods := []*MemberInfo{}
					for _, m := range o.Methods {
						if n, _ := sourceBridgeUTF8(o, m.NameIndex); n != "<init>" {
							methods = append(methods, m)
						}
					}
					o.Methods = methods
				}
				stripConstructor(a)
				stripConstructor(b)
				if nativeBinaryShape(t, a.Bytes()) != nativeBinaryShape(t, b.Bytes()) {
					t.Fatal("non-constructor ABI changed")
				}
				if nativeAnonymousAccessorShape(t, original) != nativeAnonymousAccessorShape(t, rebuilt) {
					t.Fatal("accessor ABI changed")
				}
			})
		})
	}
}

func TestAdversarialAnonymousSharedEnclosingParameterRoundTrip(t *testing.T) {
	testAnonymousSharedParameterRoundTrip(t, "outer")
}
func TestAdversarialAnonymousSharedReferenceParameterRoundTrip(t *testing.T) {
	testAnonymousSharedParameterRoundTrip(t, "reference")
}
func TestAdversarialAnonymousSharedWideParameterRoundTrip(t *testing.T) {
	testAnonymousSharedParameterRoundTrip(t, "wide")
}
func TestAdversarialAnonymousSharedMixedParameterRoundTrip(t *testing.T) {
	testAnonymousSharedParameterRoundTrip(t, "all")
}
