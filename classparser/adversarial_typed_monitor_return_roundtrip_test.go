package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

// The original compiled driver checks all JVM value-return categories independently.
// It observes monitor ownership inside the untouched effectful delegate, including
// nested locks, exceptional release, null acquisition, raw floating bits and identity.
const typedMonitorReturnFixture = `class MonitorEffects {static int calls;static final RuntimeException failure=new RuntimeException("identity"); static void check(Object lock,Object other,int fail){if(!Thread.holdsLock(lock)||(other!=null&&!Thread.holdsLock(other)))throw new AssertionError("return evaluated outside lock");calls++;if(fail!=0)throw failure;}
static boolean qZ(Object lock,Object other,boolean value,int fail){check(lock,other,fail);return value;}
static byte qB(Object lock,Object other,byte value,int fail){check(lock,other,fail);return value;}
static short qS(Object lock,Object other,short value,int fail){check(lock,other,fail);return value;}
static char qC(Object lock,Object other,char value,int fail){check(lock,other,fail);return value;}
static int qI(Object lock,Object other,int value,int fail){check(lock,other,fail);return value;}
static long qJ(Object lock,Object other,long value,int fail){check(lock,other,fail);return value;}
static float qF(Object lock,Object other,float value,int fail){check(lock,other,fail);return value;}
static double qD(Object lock,Object other,double value,int fail){check(lock,other,fail);return value;}
static Object qA(Object lock,Object other,Object value,int fail){check(lock,other,fail);return value;}
}
class TypedMonitorOwner {final Object lock;final Object other=new Object();TypedMonitorOwner(Object lock){this.lock=lock;}
boolean vZ=true;boolean fieldZ(){synchronized(lock){return vZ;}} boolean callZ(boolean value,int fail){synchronized(lock){return MonitorEffects.qZ(lock,null,value,fail);}} boolean nestedZ(boolean value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qZ(lock,other,value,fail);}}}
byte vB=(byte)-128;byte fieldB(){synchronized(lock){return vB;}} byte callB(byte value,int fail){synchronized(lock){return MonitorEffects.qB(lock,null,value,fail);}} byte nestedB(byte value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qB(lock,other,value,fail);}}}
short vS=(short)-32768;short fieldS(){synchronized(lock){return vS;}} short callS(short value,int fail){synchronized(lock){return MonitorEffects.qS(lock,null,value,fail);}} short nestedS(short value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qS(lock,other,value,fail);}}}
char vC='\uffff';char fieldC(){synchronized(lock){return vC;}} char callC(char value,int fail){synchronized(lock){return MonitorEffects.qC(lock,null,value,fail);}} char nestedC(char value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qC(lock,other,value,fail);}}}
int vI=Integer.MIN_VALUE;int fieldI(){synchronized(lock){return vI;}} int callI(int value,int fail){synchronized(lock){return MonitorEffects.qI(lock,null,value,fail);}} int nestedI(int value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qI(lock,other,value,fail);}}}
long vJ=Long.MIN_VALUE;long fieldJ(){synchronized(lock){return vJ;}} long callJ(long value,int fail){synchronized(lock){return MonitorEffects.qJ(lock,null,value,fail);}} long nestedJ(long value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qJ(lock,other,value,fail);}}}
float vF=-0.0f;float fieldF(){synchronized(lock){return vF;}} float callF(float value,int fail){synchronized(lock){return MonitorEffects.qF(lock,null,value,fail);}} float nestedF(float value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qF(lock,other,value,fail);}}}
double vD=-0.0d;double fieldD(){synchronized(lock){return vD;}} double callD(double value,int fail){synchronized(lock){return MonitorEffects.qD(lock,null,value,fail);}} double nestedD(double value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qD(lock,other,value,fail);}}}
Object vA=new Object();Object fieldA(){synchronized(lock){return vA;}} Object callA(Object value,int fail){synchronized(lock){return MonitorEffects.qA(lock,null,value,fail);}} Object nestedA(Object value,int fail){synchronized(lock){synchronized(other){return MonitorEffects.qA(lock,other,value,fail);}}}
}
class MonitorDriver {
static void same(Object a,Object b,Class<?> type){if(type==Object.class){if(a!=b)throw new AssertionError("identity");}else if(type==float.class){if(Float.floatToRawIntBits((Float)a)!=Float.floatToRawIntBits((Float)b))throw new AssertionError("float bits");}else if(type==double.class){if(Double.doubleToRawLongBits((Double)a)!=Double.doubleToRawLongBits((Double)b))throw new AssertionError("double bits");}else if(!a.equals(b))throw new AssertionError("primitive value");}
static void released(TypedMonitorOwner o){if(Thread.holdsLock(o.lock)||Thread.holdsLock(o.other))throw new AssertionError("lock leak");}
static void check(String key,Class<?> type,Object[]inputs)throws Exception{TypedMonitorOwner owner=new TypedMonitorOwner(new Object());TypedMonitorOwner nil=new TypedMonitorOwner(null);for(String prefix:new String[]{"call","nested"}){java.lang.reflect.Method m=TypedMonitorOwner.class.getDeclaredMethod(prefix+key,type,int.class);for(Object input:inputs){MonitorEffects.calls=0;Object result=m.invoke(owner,input,0);same(result,input,type);if(MonitorEffects.calls!=1)throw new AssertionError("once");released(owner);}MonitorEffects.calls=0;try{m.invoke(owner,inputs[0],1);throw new AssertionError("missing failure");}catch(java.lang.reflect.InvocationTargetException e){if(e.getCause()!=MonitorEffects.failure||MonitorEffects.calls!=1)throw new AssertionError("exception identity/effects");}released(owner);MonitorEffects.calls=0;try{m.invoke(nil,inputs[0],0);throw new AssertionError("null monitor");}catch(java.lang.reflect.InvocationTargetException e){if(!(e.getCause()instanceof NullPointerException)||MonitorEffects.calls!=0)throw new AssertionError("acquire order");}}
java.lang.reflect.Method field=TypedMonitorOwner.class.getDeclaredMethod("field"+key);same(field.invoke(owner),TypedMonitorOwner.class.getDeclaredField("v"+key).get(owner),type);released(owner);try{field.invoke(nil);throw new AssertionError("null field monitor");}catch(java.lang.reflect.InvocationTargetException e){if(!(e.getCause()instanceof NullPointerException))throw e;}}
public static void main(String[]args)throws Exception{Object token=new Object();
check("Z",boolean.class,new Object[]{false,true,false,true});
check("B",byte.class,new Object[]{(byte)-128,(byte)-1,(byte)0,(byte)127});
check("S",short.class,new Object[]{(short)-32768,(short)-1,(short)0,(short)32767});
check("C",char.class,new Object[]{'\u0000','A','\u8000','\uffff'});
check("I",int.class,new Object[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE});
check("J",long.class,new Object[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE});
check("F",float.class,new Object[]{-0.0f,Float.NaN,Float.NEGATIVE_INFINITY,Float.MAX_VALUE});
check("D",double.class,new Object[]{-0.0d,Double.NaN,Double.NEGATIVE_INFINITY,Double.MAX_VALUE});
check("A",Object.class,new Object[]{null,token,token,new Object()});
System.out.println("126:typed-monitor:values:identity:once:failure:release");}}`

func TestAdversarialTypedMonitorReturnRoundTrip(t *testing.T) {
	for _, owner := range []string{"TypedMonitorOwner", "RenamedMonitorScope"} {
		t.Run(owner, func(t *testing.T) {
			source := strings.ReplaceAll(typedMonitorReturnFixture, "TypedMonitorOwner", owner)
			testNativePrivateSetterCompiledFixture(t, owner, "MonitorDriver", "126:typed-monitor:values:identity:once:failure:release\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, typedMonitorFieldReadProtected)
		})
	}
}

// A sequential field-value comparison cannot catch a racing read after release.
// Verify the rebuilt field access remains inside a catch-all monitor domain too.
func typedMonitorFieldReadProtected(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	for _, raw := range [][]byte{original, rebuilt} {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range obj.Methods {
			methodName, _ := sourceBridgeUTF8(obj, method.NameIndex)
			if !strings.HasPrefix(methodName, "field") {
				continue
			}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, op := range d.Opcodes() {
					if op.Instr.OpCode != core.OP_GETFIELD {
						continue
					}
					member := constructorMotionMember(obj, op, core.OP_GETFIELD)
					if member == nil || !strings.HasPrefix(member.Member, "v") {
						continue
					}
					found = true
					protected := false
					for _, handler := range code.ExceptionTable {
						if handler.CatchType == 0 && op.CurrentOffset >= handler.StartPc && op.CurrentOffset < handler.EndPc {
							protected = true
						}
					}
					if !protected {
						t.Fatalf("%s.%s reads %s after monitor release at pc%d", name, methodName, member.Member, op.CurrentOffset)
					}
				}
				if !found {
					t.Fatalf("%s missing field read", methodName)
				}
			}
		}
	}
}
