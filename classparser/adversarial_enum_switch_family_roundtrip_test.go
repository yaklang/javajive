package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"reflect"
	"strings"
	"testing"
)

func nativeEnumSwitchFamilySources(root string) map[string]string {
	source := `public class SwitchFamilyOwner{private final int offset;public SwitchFamilyOwner(int n){offset=n;}public abstract static class Task{public Task(){}public abstract int run(SwitchFamilyOwnerMode m);}public Task make(final int x){return new Task(){public int run(SwitchFamilyOwnerMode m){switch(m){case A:return offset+x;case B:return offset-x;default:return offset;}}};}}
class SwitchFamilyDriver{public static void main(String[]a)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){SwitchFamilyOwner owner=new SwitchFamilyOwner(n);SwitchFamilyOwner.Task task=owner.make(x);if(!task.getClass().getEnclosingMethod().getName().equals("make")||SwitchFamilyOwner.Task.class.getDeclaringClass()!=SwitchFamilyOwner.class)throw new AssertionError("lexical metadata");for(SwitchFamilyOwnerMode m:SwitchFamilyOwnerMode.values()){java.math.BigInteger expected=java.math.BigInteger.valueOf(n);if(m==SwitchFamilyOwnerMode.A)expected=expected.add(java.math.BigInteger.valueOf(x));else if(m==SwitchFamilyOwnerMode.B)expected=expected.subtract(java.math.BigInteger.valueOf(x));if(task.run(m)!=expected.intValue())throw new AssertionError("switch/overflow/binding");rows++;}try{task.run(null);throw new AssertionError("null selector");}catch(NullPointerException expected){}}System.out.println(rows+":enum:lexical:captured:case:null:overflow");}}`
	source = strings.ReplaceAll(source, "SwitchFamilyOwner", root)
	return map[string]string{root + ".java": source, root + "Mode.java": "public enum " + root + "Mode{A,B,C}"}
}
func TestAdversarialEnumSwitchOwnedAnonymousFamilyRoundTrip(t *testing.T) {
	testNativeEnumSwitchSourceFixture(t, nativeEnumSwitchFamilySources("SwitchFamilyOwner"), "SwitchFamilyOwner", "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
}
func TestAdversarialEnumSwitchOwnedAnonymousRenamedRoundTrip(t *testing.T) {
	testNativeEnumSwitchSourceFixture(t, nativeEnumSwitchFamilySources("DifferentSwitchOwner"), "DifferentSwitchOwner", "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
}

func TestAdversarialEnumSwitchEqualTypedParameterIdentityRoundTrip(t *testing.T) {
	sources := nativeEnumSwitchFamilySources("ParameterSwitchOwner")
	s := sources["ParameterSwitchOwner.java"]
	s = strings.ReplaceAll(s, "run(ParameterSwitchOwnerMode m)", "run(long padding,ParameterSwitchOwnerMode m,ParameterSwitchOwnerMode other)")
	s = strings.ReplaceAll(s, "task.run(m)", "task.run(0x123456789abcdefL,m,m==ParameterSwitchOwnerMode.A?ParameterSwitchOwnerMode.B:ParameterSwitchOwnerMode.A)")
	s = strings.ReplaceAll(s, "task.run(null)", "task.run(0x123456789abcdefL,null,ParameterSwitchOwnerMode.C)")
	sources["ParameterSwitchOwner.java"] = s
	testNativeEnumSwitchSourceFixture(t, sources, "ParameterSwitchOwner", "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
}

func TestAdversarialEnumSwitchDescendingCaseRegistrationRoundTrip(t *testing.T) {
	sources := nativeEnumSwitchFamilySources("OrderedSwitchOwner")
	sources["OrderedSwitchOwner.java"] = strings.Replace(sources["OrderedSwitchOwner.java"], "case A:return offset+x;case B:return offset-x;", "case B:return offset-x;case A:return offset+x;", 1)
	testNativeEnumSwitchSourceFixture(t, sources, "OrderedSwitchOwner", "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
}

func TestAdversarialEnumSwitchSharedTableTwoAnonymousScopesRoundTrip(t *testing.T) {
	sources := nativeEnumSwitchFamilySources("SharedSwitchOwner")
	s := sources["SharedSwitchOwner.java"]
	s = strings.Replace(s, "class SwitchFamilyDriver", `class SwitchFamilyDriver`, 1)
	split := strings.Index(s, "\nclass SwitchFamilyDriver")
	s = s[:split-1] + `public Task other(final int x){return new Task(){public int run(SharedSwitchOwnerMode m){switch(m){case C:return offset+x;case A:return offset-x;default:return offset;}}};}}` + s[split:]
	s = strings.Replace(s, "SharedSwitchOwner.Task task=owner.make(x);", "for(int which=0;which<2;which++){SharedSwitchOwner.Task task=which==0?owner.make(x):owner.other(x);", 1)
	s = strings.Replace(s, `getEnclosingMethod().getName().equals("make")`, `getEnclosingMethod().getName().equals(which==0?"make":"other")`, 1)
	s = strings.Replace(s, "if(m==SharedSwitchOwnerMode.A)", "if(which==0?m==SharedSwitchOwnerMode.A:m==SharedSwitchOwnerMode.C)", 1)
	s = strings.Replace(s, "else if(m==SharedSwitchOwnerMode.B)", "else if(which==0?m==SharedSwitchOwnerMode.B:m==SharedSwitchOwnerMode.A)", 1)
	s = strings.Replace(s, "System.out.println(rows", "}System.out.println(rows", 1)
	sources["SharedSwitchOwner.java"] = s
	testNativeEnumSwitchSourceFixture(t, sources, "SharedSwitchOwner", "SwitchFamilyDriver", "150:enum:lexical:captured:case:null:overflow\n")
}

func TestAdversarialEnumSwitchRootWithoutNamedChildrenRoundTrip(t *testing.T) {
	sources := map[string]string{"FlatSwitchOwner.java": `public class FlatSwitchOwner{public static int run(long extra,FlatSwitchOwnerMode first,FlatSwitchOwnerMode other){switch(first){case C:return 29;case A:return 17;default:return -31;}}}
 class FlatSwitchDriver{public static void main(String[]args){int rows=0;for(FlatSwitchOwnerMode m:FlatSwitchOwnerMode.values())for(FlatSwitchOwnerMode other:FlatSwitchOwnerMode.values()){int expected=m==FlatSwitchOwnerMode.C?29:m==FlatSwitchOwnerMode.A?17:-31;if(FlatSwitchOwner.run(Long.MIN_VALUE,m,other)!=expected)throw new AssertionError("enum parameter binding");rows++;}try{FlatSwitchOwner.run(0L,null,FlatSwitchOwnerMode.C);throw new AssertionError("null");}catch(NullPointerException expected){}System.out.println(rows+":enum:root:parameter:null");}}`, "FlatSwitchOwnerMode.java": `public enum FlatSwitchOwnerMode{A,B,C}`}
	testNativeEnumSwitchSourceFixture(t, sources, "FlatSwitchOwner", "FlatSwitchDriver", "9:enum:root:parameter:null\n")
}

// This oracle compares decoded executable packets independently of the source
// certificate: CP indices may change, but member identities, numeric operands,
// branch offsets, handlers and frame capacities must remain the same.
func nativeEnumSwitchOriginalPacketShape(t *testing.T, object *ClassObject) []string {
	t.Helper()
	var rows []string
	for _, m := range object.Methods {
		name, _ := sourceBridgeUTF8(object, m.NameIndex)
		desc, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
		rows = append(rows, fmt.Sprintf("method:%s%s:%x", name, desc, m.AccessFlags))
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			rows = append(rows, fmt.Sprintf("capacity:%d:%d", code.MaxStack, code.MaxLocals))
			dec := core.NewDecompiler(code.Code, nil)
			if e := dec.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			for _, op := range constructorMotionOps(dec) {
				operand := fmt.Sprint(op.Data)
				if member := constructorMotionMember(object, op, op.Instr.OpCode); member != nil {
					operand = member.Name + "." + member.Member + member.Description
				}
				if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W {
					index := int(op.Data[0])
					if len(op.Data) == 2 {
						index = int(core.Convert2bytesToInt(op.Data))
					}
					operand = fmt.Sprintf("literal:%#v", object.ConstantPool[index-1])
				}
				rows = append(rows, fmt.Sprintf("op:%d:%x:%s", op.CurrentOffset, op.Instr.OpCode, operand))
			}
			for _, h := range code.ExceptionTable {
				caught, ok := sourceBridgeClassName(object, h.CatchType)
				if !ok {
					t.Fatal("invalid original handler")
				}
				rows = append(rows, fmt.Sprintf("catch:%d:%d:%d:%s", h.StartPc, h.EndPc, h.HandlerPc, caught))
			}
		}
	}
	return rows
}
func testNativeEnumSwitchSourceFixture(t *testing.T, sources map[string]string, owner, driver, want string) {
	t.Helper()
	testNativePrivateSetterCompiledFixture(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	}, func(t *testing.T, name string, original, rebuilt []byte) {
		a, e := Parse(original)
		if e != nil {
			t.Fatal(e)
		}
		if a.AccessFlags&0x1000 == 0 {
			return
		}
		b, e := Parse(rebuilt)
		if e != nil {
			t.Fatal(e)
		}
		before, after := nativeEnumSwitchOriginalPacketShape(t, a), nativeEnumSwitchOriginalPacketShape(t, b)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("original enum artifact packet changed %s\n%v\n%v", name, before, after)
		}
	})
}
