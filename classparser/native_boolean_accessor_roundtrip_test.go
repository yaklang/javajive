package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

const nativeBooleanAccessorFixture = `class BoolAccessEffects{static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("original");static BoolAccessOwner receiver(BoolAccessOwner owner){trace+="R";if(fail==1)throw failure;return owner;}static boolean value(boolean value){trace+="V";if(fail==2)throw failure;return value;}}
class BoolAccessOwner{private boolean value;class Writer{boolean literal(){return value=true;}boolean clear(){return value=false;}boolean put(BoolAccessOwner owner,boolean next){return BoolAccessEffects.receiver(owner).value=BoolAccessEffects.value(next);}boolean get(){return value;}}Writer writer(){return new Writer();}}
class BoolAccessDriver{public static void main(String[]args){BoolAccessOwner owner=new BoolAccessOwner();BoolAccessOwner.Writer writer=owner.writer();if(!writer.literal()||!writer.get()||writer.clear()||writer.get())throw new AssertionError("literal/store/return");int rows=0;for(boolean next:new boolean[]{false,true}){BoolAccessEffects.fail=0;BoolAccessEffects.trace="";if(writer.put(owner,next)!=next||writer.get()!=next||!BoolAccessEffects.trace.equals("RV"))throw new AssertionError("value/order/once");for(int fail:new int[]{0,1,2}){BoolAccessEffects.fail=fail;BoolAccessEffects.trace="";try{writer.put(null,next);throw new AssertionError("missing failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=BoolAccessEffects.failure)throw new AssertionError("failure identity");}if(!BoolAccessEffects.trace.equals(fail==1?"R":"RV"))throw new AssertionError("failure order");}rows++;}System.out.println(rows+":boolean:accessor:store:result:order");}}`

func TestNativeBooleanAccessorRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeBooleanAccessorFixture, "BoolAccessOwner", "BoolAccessDriver", "2:boolean:accessor:store:result:order\n")
}
func TestNativeBooleanAccessorRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeBooleanAccessorFixture, "BoolAccessOwner", "IndependentBoolWriteScope")
	testNativePrivateSetterFixture(t, f, "IndependentBoolWriteScope", "BoolAccessDriver", "2:boolean:accessor:store:result:order\n")
}

// JVM boolean consumers narrow bit zero, not nonzero. These valid authored
// classfiles pass noncanonical int words directly to the original Z bridge.
func TestNativeBooleanAccessorNoncanonicalWordsRoundTrip(t *testing.T) {
	testNativePrivateSetterFixtureWithMutation(t, nativeBooleanAccessorFixture, "BoolAccessOwner", "BoolAccessDriver", "2:boolean:accessor:store:result:order\n", func(t *testing.T, files map[string][]byte) {
		obj, e := Parse(files["BoolAccessOwner$Writer.class"])
		if e != nil {
			t.Fatal(e)
		}
		changed := 0
		for _, m := range obj.Methods {
			name, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if name != "literal" && name != "clear" {
				continue
			}
			for _, a := range m.Attributes {
				c, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				d := core.NewDecompiler(c.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e := d.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				matches := 0
				for _, op := range d.Opcodes() {
					want, to := core.OP_ICONST_1, core.OP_ICONST_3
					if name == "clear" {
						want, to = core.OP_ICONST_0, core.OP_ICONST_2
					}
					if op.Instr.OpCode == want {
						c.Code[int(op.CurrentOffset)] = byte(to)
						matches++
					}
				}
				if matches != 1 {
					t.Fatalf("original %s literal count=%d", name, matches)
				}
				changed++
			}
		}
		if changed != 2 {
			t.Fatal("both original Z consumer packets required")
		}
		files["BoolAccessOwner$Writer.class"] = obj.Bytes()
	})
}
