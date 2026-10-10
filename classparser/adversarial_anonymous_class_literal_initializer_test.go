package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

const anonymousClassLiteralInitializerFixture = `interface LiteralRead {Class<?> get();}
class LiteralEffects {static String trace="";static int initialized;static final RuntimeException error=new RuntimeException("original");static Class<?> pick(Class<?> value,int mode){trace+="L";if(initialized!=0)throw new AssertionError("class literal initialized subject");if(mode==1)throw error;return value;}}
class LiteralSubject {static{LiteralEffects.initialized++;}}
class LiteralOwner {LiteralRead make(final int mode){return new LiteralRead(){final Class<?> value=LiteralEffects.pick(LiteralSubject.class,mode);public Class<?> get(){return value;}};}}
class LiteralDriver {public static void main(String[]args){for(int mode:new int[]{0,1}){LiteralEffects.trace="";try{LiteralRead result=new LiteralOwner().make(mode);if(mode!=0||!result.getClass().isAnonymousClass()||result.get()!=LiteralSubject.class||LiteralEffects.initialized!=0||!LiteralEffects.trace.equals("L"))throw new AssertionError("class literal identity/initializer/order");}catch(RuntimeException e){if(mode!=1||e!=LiteralEffects.error||LiteralEffects.initialized!=0||!LiteralEffects.trace.equals("L"))throw new AssertionError("class literal abrupt order",e);}}System.out.println("2:class:literal:initializer");}}
`

func TestAdversarialAnonymousClassLiteralInitializerKeepsOriginalSubject(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			for _, subject := range []string{"LiteralSubject", "LiteralSubject[]", "int[]", "LiteralSubject[][]"} {
				t.Run(subject, func(t *testing.T) {
					source := strings.ReplaceAll(anonymousClassLiteralInitializerFixture, "LiteralSubject.class", subject+".class")
					owner := "LiteralOwner"
					if rename == "renamed" {
						source = strings.NewReplacer("LiteralOwner", "IdentityEnvelope", "LiteralSubject", "ReferencedPayload").Replace(source)
						owner = "IdentityEnvelope"
					}
					testNativePrivateSetterFixture(t, source, owner, "LiteralDriver", "2:class:literal:initializer\n")
				})
			}
		})
	}
}

const anonymousClassLiteralLinkageFixture = `
interface LiteralLinkRead {int get();}
class LiteralLinkEffects {static String trace="";static int initialized;static void before(){trace+="B";}static void observe(Class<?> c){trace+="C";if(initialized!=0)throw new AssertionError("subject initialized");}static void after(){trace+="A";}}
class LiteralLinkSubject {static{LiteralLinkEffects.initialized++;}}
class LiteralLinkOwner {
 public static String run(){LiteralLinkEffects.trace="";try{LiteralLinkRead r=new LiteralLinkRead(){{LiteralLinkEffects.before();LiteralLinkEffects.observe(LiteralLinkSubject.class);LiteralLinkEffects.after();}public int get(){return 1;}};if(!r.getClass().isAnonymousClass()||r.get()!=1||LiteralLinkEffects.initialized!=0||!LiteralLinkEffects.trace.equals("BCA"))throw new AssertionError("class literal success identity/order");return "BCA:1";}catch(LinkageError e){if(e.getClass()!=NoClassDefFoundError.class||!(e.getCause() instanceof ClassNotFoundException)||!LiteralLinkEffects.trace.equals("B")||LiteralLinkEffects.initialized!=0)throw new AssertionError("class literal linkage phase",e);return "B:missing";}}
}
class LiteralLinkLoader extends ClassLoader {
 private final boolean deny;LiteralLinkLoader(boolean deny){super(LiteralLinkDriver.class.getClassLoader());this.deny=deny;}
 protected Class<?> loadClass(String name,boolean resolve)throws ClassNotFoundException{
  if(deny&&name.equals("LiteralLinkSubject"))throw new ClassNotFoundException(name);
  if(!name.startsWith("LiteralLinkOwner")&&!name.equals("LiteralLinkRead")&&!name.equals("LiteralLinkEffects")&&!name.equals("LiteralLinkSubject"))return super.loadClass(name,resolve);
  Class<?> found=findLoadedClass(name);if(found==null){try(java.io.InputStream in=getParent().getResourceAsStream(name+".class")){
   java.io.ByteArrayOutputStream bytes=new java.io.ByteArrayOutputStream();byte[]buffer=new byte[512];int n;while((n=in.read(buffer))!=-1)bytes.write(buffer,0,n);byte[]data=bytes.toByteArray();found=defineClass(name,data,0,data.length);
  }catch(java.io.IOException e){throw new ClassNotFoundException(name,e);}}
  if(resolve)resolveClass(found);return found;
 }
}
class LiteralLinkDriver {public static void main(String[]args)throws Exception{for(boolean deny:new boolean[]{false,true}){Class<?> owner=Class.forName("LiteralLinkOwner",true,new LiteralLinkLoader(deny));java.lang.reflect.Method run=owner.getDeclaredMethod("run");run.setAccessible(true);String got=(String)run.invoke(null);if(!got.equals(deny?"B:missing":"BCA:1"))throw new AssertionError("literal linkage caller "+got);}System.out.println("2:class:literal:linkage");}}
`

func TestAdversarialAnonymousClassLiteralInitializerKeepsLinkagePhase(t *testing.T) {
	testNativePrivateSetterFixture(t, anonymousClassLiteralLinkageFixture, "LiteralLinkOwner", "LiteralLinkDriver", "2:class:literal:linkage\n")
}

func TestAdversarialAnonymousClassLiteralDirectStoreAndVirtualReceiver(t *testing.T) {
	for _, shape := range []string{"direct-field", "class-receiver"} {
		t.Run(shape, func(t *testing.T) {
			fixture := anonymousClassLiteralInitializerFixture
			want := "2:class:literal:initializer\n"
			if shape == "direct-field" {
				fixture = strings.ReplaceAll(fixture, "LiteralEffects.pick(LiteralSubject.class,mode)", "LiteralSubject.class")
				fixture = strings.ReplaceAll(fixture, "new int[]{0,1}", "new int[]{0}")
				fixture = strings.ReplaceAll(fixture, "!LiteralEffects.trace.equals(\"L\")", "!LiteralEffects.trace.equals(\"\")")
				fixture = strings.ReplaceAll(fixture, "2:class:literal:initializer", "1:class:literal:initializer")
				want = "1:class:literal:initializer\n"
			} else {
				fixture = strings.ReplaceAll(fixture, "LiteralSubject.class,mode", "LiteralSubject.class.asSubclass(Object.class),mode")
			}
			testNativePrivateSetterFixture(t, fixture, "LiteralOwner", "LiteralDriver", want)
		})
	}
}

func TestAdversarialAnonymousClassLiteralWideConstantPoolRetainsEarlierStores(t *testing.T) {
	padding := ""
	for i := 0; i < 90; i++ {
		padding += fmt.Sprintf("String padding%d=\"v%d\";", i, i)
	}
	fixture := strings.Replace(anonymousClassLiteralInitializerFixture, "new LiteralRead(){", "new LiteralRead(){"+padding, 1)
	fixture = strings.Replace(fixture, "LiteralEffects.initialized!=0||!LiteralEffects.trace.equals(\"L\")", "LiteralEffects.initialized!=0||!LiteralEffects.trace.equals(\"L\")", 1)
	fixture = strings.Replace(fixture, "}catch(RuntimeException e){", "for(int i=0;i<90;i++){try{java.lang.reflect.Field f=result.getClass().getDeclaredField(\"padding\"+i);f.setAccessible(true);if(!f.get(result).equals(\"v\"+i))throw new AssertionError(\"wide class literal earlier store\");}catch(ReflectiveOperationException e){throw new AssertionError(e);}}}catch(RuntimeException e){", 1)
	testNativePrivateSetterFixtureWithMutation(t, fixture, "LiteralOwner", "LiteralDriver", "2:class:literal:initializer\n", func(t *testing.T, files map[string][]byte) {
		obj, err := Parse(files["LiteralOwner$1.class"])
		if err != nil {
			t.Fatal(err)
		}
		wide := false
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if n != "<init>" {
				continue
			}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if d.ParseOpcode() != nil {
					t.Fatal("original decoder")
				}
				for _, op := range constructorMotionOps(d) {
					if op.Instr.OpCode == core.OP_LDC_W {
						subject, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
						wide = wide || known && subject == "LiteralSubject"
					}
				}
			}
		}
		if !wide {
			t.Fatal("fixture must execute actual original wide class LDC")
		}
	})
}
