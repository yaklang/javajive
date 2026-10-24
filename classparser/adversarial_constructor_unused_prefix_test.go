package javaclassparser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

const constructorUnusedPrefixFixture = `class UnusedPrefixEffects {static String trace;static final RuntimeException prefix=new IllegalArgumentException("prefix"),parent=new IllegalStateException("parent"),body=new UnsupportedOperationException("body");
 static Object[] before(int mode){trace+="P";if(mode==1)throw prefix;return mode==4?null:new Object[]{new Object()};}
 static void finish(int mode){trace+="B";if(mode==3)throw body;}}
class UnusedPrefixBase {final Object input;UnusedPrefixBase(Object input,int mode){this.input=input;UnusedPrefixEffects.trace+="S";if(mode==2)throw UnusedPrefixEffects.parent;}}
public class UnusedPrefixOwner extends UnusedPrefixBase {final int result;public UnusedPrefixOwner(Object input,int mode){super(input,mode);Object[] unused=UnusedPrefixEffects.before(mode);UnusedPrefixEffects.finish(mode);result=mode*31;}}
class UnusedPrefixDriver {public static void main(String[]args)throws Exception{int rows=0;for(Object token:new Object[]{null,new Object()})for(int mode=0;mode<5;mode++){
 UnusedPrefixEffects.trace="";UnusedPrefixOwner value=null;Throwable error=null;try{value=new UnusedPrefixOwner(token,mode);}catch(Throwable e){error=e;}
 if(error!=(mode==1?UnusedPrefixEffects.prefix:mode==2?UnusedPrefixEffects.parent:mode==3?UnusedPrefixEffects.body:null)||!UnusedPrefixEffects.trace.equals(mode==1?"P":mode==2?"PS":"PSB")||error!=null&&value!=null||error==null&&(value.input!=token||value.result!=mode*31))throw new AssertionError("original pre-SUPER call/order/identity",error);rows++;}
 if(UnusedPrefixOwner.class.getDeclaredConstructors().length!=1||UnusedPrefixOwner.class.getDeclaredConstructors()[0].getExceptionTypes().length!=0)throw new AssertionError("constructor ABI");System.out.println(rows+":unused:prefix:ordered:effects");}}
`

// Produce a valid JVM-only constructor by moving a straight-line static call
// and its dead local store before initialization. No Code from this original
// is retained when the candidate is compiled. The unchanged driver first checks
// the authored order and exact failures on the original, under JVM verification.
func constructorUnusedPrefixOriginal(t *testing.T, debug string) map[string][]byte {
	t.Helper()
	return constructorUnusedPrefixOriginalSource(t, debug, constructorUnusedPrefixFixture)
}

func constructorUnusedPrefixOriginalSource(t *testing.T, debug, fixture string) map[string][]byte {
	t.Helper()
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"UnusedPrefixOwner.java": fixture}, debug, "8")
	obj := mustParseLowering(t, files["UnusedPrefixOwner.class"])
	changed := false
	for _, m := range obj.Methods {
		n, _ := obj.getUtf8(m.NameIndex)
		if n != "<init>" {
			continue
		}
		c := constructorLoweringCode(m)
		if c == nil || len(c.ExceptionTable) != 0 {
			t.Fatal("fixture exception regions changed")
		}
		d := core.NewDecompiler(c.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		if err := d.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		cut, end := -1, -1
		for _, op := range d.Opcodes() {
			if member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); member != nil && member.Name == "UnusedPrefixBase" {
				cut = int(op.CurrentOffset) + 3
			}
			if cut > 0 && int(op.CurrentOffset) >= cut && core.GetStoreIdx(op) == 3 {
				end = int(op.CurrentOffset) + 1
				break
			}
		}
		if cut != 6 || end != 11 {
			t.Fatalf("fixture packet changed: %x (%d,%d)", c.Code, cut, end)
		}
		old := append([]byte(nil), c.Code...)
		c.Code = append(append(append([]byte{}, old[cut:end]...), old[:cut]...), old[end:]...)
		for _, a := range c.Attributes {
			switch a := a.(type) {
			case *LineNumberTableAttribute:
				for _, line := range a.LineNumberTable {
					pc := int(line.StartPc)
					if pc < cut {
						line.StartPc += uint16(end - cut)
					} else if pc < end {
						line.StartPc -= uint16(cut)
					}
				}
			case *UnparsedAttribute:
				if a.Name != "LocalVariableTable" && a.Name != "LocalVariableTypeTable" {
					t.Fatalf("unexpected constructor Code attribute %s", a.Name)
				}
				if len(a.Info) < 2 || len(a.Info) != 2+10*int(binary.BigEndian.Uint16(a.Info)) {
					t.Fatal("bad fixture debug table")
				}
				for at := 2; at < len(a.Info); at += 10 {
					pc := int(binary.BigEndian.Uint16(a.Info[at:]))
					size := int(binary.BigEndian.Uint16(a.Info[at+2:]))
					slot := int(binary.BigEndian.Uint16(a.Info[at+8:]))
					if slot == 3 && pc == end {
						binary.BigEndian.PutUint16(a.Info[at:], uint16(end-cut))
						binary.BigEndian.PutUint16(a.Info[at+2:], uint16(size+cut))
					} else if !(pc == 0 || pc >= end) {
						t.Fatal("unexpected debug interval")
					}
				}
			default:
				t.Fatalf("unexpected code metadata %T", a)
			}
		}
		changed = true
	}
	if !changed {
		t.Fatal("missing fixture constructor")
	}
	files["UnusedPrefixOwner.class"] = obj.Bytes()
	return files
}

func TestAdversarialConstructorUnusedPrefixRetainsOriginalCallEffects(t *testing.T) {
	testNativePrivateSetterCompiledFixtureWithPolicies(t, "UnusedPrefixOwner", "UnusedPrefixDriver", "10:unused:prefix:ordered:effects\n", constructorUnusedPrefixOriginal, func(*testing.T, string, []byte, []byte) {}, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
}

func TestAdversarialConstructorUnusedPrefixKeepsAllResultCategories(t *testing.T) {
	for _, kind := range []string{"int", "long", "float", "double", "checked"} {
		t.Run(kind, func(t *testing.T) {
			fixture := constructorUnusedPrefixFixture
			if kind != "checked" {
				result := "mode*31"
				if kind == "long" {
					result = "Long.MIN_VALUE"
				}
				if kind == "float" {
					result = "-0.0F"
				}
				if kind == "double" {
					result = "-0.0D"
				}
				fixture = strings.Replace(fixture, "mode==4?null:new Object[]{new Object()}", result, 1)
				fixture = strings.Replace(fixture, "static Object[] before", "static "+kind+" before", 1)
				fixture = strings.Replace(fixture, "Object[] unused=", kind+" unused=", 1)
			} else {
				fixture = strings.Replace(fixture, `static final RuntimeException prefix=new IllegalArgumentException("prefix"),parent=`, `static final java.io.IOException prefix=new java.io.IOException("prefix");static final RuntimeException parent=`, 1)
				fixture = strings.Replace(fixture, "before(int mode){", "before(int mode)throws java.io.IOException{", 1)
				fixture = strings.Replace(fixture, "UnusedPrefixOwner(Object input,int mode){", "UnusedPrefixOwner(Object input,int mode)throws java.io.IOException{", 1)
				fixture = strings.Replace(fixture, "getExceptionTypes().length!=0", "getExceptionTypes().length!=1", 1)
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "UnusedPrefixOwner", "UnusedPrefixDriver", "10:unused:prefix:ordered:effects\n", func(t *testing.T, debug string) map[string][]byte {
				return constructorUnusedPrefixOriginalSource(t, debug, fixture)
			}, func(*testing.T, string, []byte, []byte) {}, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
