package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The inner handler builds an array only on the selected conditional arm.
// Original dependency classes record operand order and exceptional exits; only
// the owner is rebuilt, so its missing/spurious allocation cannot hide in an
// identically mistranslated driver.
const handlerConditionalArrayFixture = `interface HandlerResult {}
class HandlerProduct implements HandlerResult {final java.lang.reflect.Method method;HandlerProduct(java.lang.reflect.Method method){HandlerSupport.trace+="P";this.method=method;}}
class HandlerSupport {
 static final HandlerResult legacy=new HandlerResult(){};static final HandlerResult fallback=new HandlerResult(){};
 static RuntimeException failure;static Throwable caught;static String trace;
 static void start(int mode){trace+="S";if(mode!=0)throw failure;}
 static boolean left(boolean value){trace+="L";return value;}
 static boolean right(boolean value){trace+="R";return value;}
 static Class<?> element(int mode){trace+="E";if(mode==2)throw failure;if(mode==3)return Integer.class;if(mode==4)return null;return String.class;}
 static void pick(String input){}
}
public class HandlerArrayOwner {
 public static HandlerResult run(boolean a,boolean b,int mode){
  try {try {HandlerSupport.start(mode);return new HandlerProduct(null);}
  catch(Exception first){return HandlerSupport.left(a)&&HandlerSupport.right(b)?HandlerSupport.legacy:new HandlerProduct(HandlerSupport.class.getDeclaredMethod("pick",new Class<?>[]{HandlerSupport.element(mode)}));}}
  catch(Exception second){HandlerSupport.caught=second;HandlerSupport.trace+="X";return HandlerSupport.fallback;}
 }
}
class HandlerArrayDriver {public static void main(String[] args){int rows=0;
 for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true})for(int mode=0;mode<5;mode++){
 HandlerSupport.trace="";HandlerSupport.caught=null;HandlerSupport.failure=new IllegalArgumentException("same");
 HandlerResult result=HandlerArrayOwner.run(a,b,mode);boolean legacy=mode!=0&&a&&b;boolean operand=mode!=0&&!legacy;boolean failed=operand&&mode>=2;
 String expected=mode==0?"SP":"SL"+(a?"R":"")+(legacy?"":"E"+(failed?"X":"P"));
 if(!HandlerSupport.trace.equals(expected)||legacy&&result!=HandlerSupport.legacy||failed&&result!=HandlerSupport.fallback)throw new AssertionError("lazy arm / evaluation order");
 if(!legacy&&!failed&&(!(result instanceof HandlerProduct)||mode==0&&((HandlerProduct)result).method!=null||mode==1&&((HandlerProduct)result).method.getParameterTypes()[0]!=String.class))throw new AssertionError("reflection binding");
 if(!failed&&HandlerSupport.caught!=null||failed&&mode==2&&HandlerSupport.caught!=HandlerSupport.failure||failed&&mode==3&&!(HandlerSupport.caught instanceof NoSuchMethodException)||failed&&mode==4&&!(HandlerSupport.caught instanceof NoSuchMethodException))throw new AssertionError("handler identity mode="+mode+" a="+a+" b="+b+" caught="+HandlerSupport.caught);rows++;
 }System.out.println(rows+":handler:array:lazy:binding:exceptions");}}
`

func TestAdversarialHandlerConditionalArrayPreservesSelectedAllocation(t *testing.T) {
	for _, rename := range []bool{false, true} {
		for _, method := range []string{"getDeclaredMethod", "getMethod", "ordinary method"} {
			for _, shape := range []string{"conditional", "direct"} {
				t.Run(fmt.Sprintf("renamed%v/%s/%s", rename, method, shape), func(t *testing.T) {
					fixture, owner, driver := handlerConditionalArrayFixture, "HandlerArrayOwner", "HandlerArrayDriver"
					if method == "getMethod" {
						fixture = strings.ReplaceAll(fixture, "getDeclaredMethod", method)
						fixture = strings.Replace(fixture, "static void pick(", "public static void pick(", 1)
					}
					if method == "ordinary method" {
						fixture = strings.Replace(fixture, "static void pick(String input){}", `static java.lang.reflect.Method getMethod(Class<?> type)throws NoSuchMethodException{return HandlerSupport.class.getDeclaredMethod("pick",type);}static void pick(String input){}`, 1)
						fixture = strings.Replace(fixture, `HandlerSupport.class.getDeclaredMethod("pick",new Class<?>[]{HandlerSupport.element(mode)})`, `HandlerSupport.getMethod(HandlerSupport.element(mode))`, 1)
					}
					if shape == "direct" {
						fixture = strings.Replace(fixture, "return HandlerSupport.left(a)&&HandlerSupport.right(b)?HandlerSupport.legacy:new HandlerProduct", "if(HandlerSupport.left(a)&&HandlerSupport.right(b))return HandlerSupport.legacy;return new HandlerProduct", 1)
					}
					if rename {
						fixture = strings.NewReplacer("HandlerArrayOwner", "DifferentOwner", "HandlerArrayDriver", "DifferentDriver", "HandlerSupport", "DifferentEffects", "HandlerProduct", "DifferentProduct", "HandlerResult", "DifferentResult", "pick", "resolve").Replace(fixture)
						owner, driver = "DifferentOwner", "DifferentDriver"
					}
					testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, driver, "20:handler:array:lazy:binding:exceptions\n", func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
					}, nil, []string{"normal"})
				})
			}
		}
	}
}

func TestAdversarialHandlerConditionalLiteralArrayKeepsOriginalDefinition(t *testing.T) {
	for _, layout := range []string{"parameter", "reused handler slots", "nested receivers", "concrete arms", "checked join", "nested reflection"} {
		t.Run(layout, func(t *testing.T) {
			fixture := strings.Replace(handlerConditionalArrayFixture, "HandlerSupport.element(mode)", "String.class", 1)
			fixture = strings.Replace(fixture, "mode<5", "mode<2", 1)
			fixture = strings.Replace(fixture, `"E"+(failed?"X":"P")`, `(failed?"X":"P")`, 1)
			if layout == "concrete arms" || layout == "checked join" || layout == "nested reflection" {
				fixture = "class HandlerLegacy implements HandlerResult {}" + fixture
				fixture = strings.Replace(fixture, "static final HandlerResult legacy=new HandlerResult(){}", "static final HandlerLegacy legacy=new HandlerLegacy()", 1)
			}
			if layout == "checked join" || layout == "nested reflection" {
				expression := `HandlerSupport.left(a)&&HandlerSupport.right(b)?HandlerSupport.legacy:new HandlerProduct(HandlerSupport.class.getDeclaredMethod("pick",new Class<?>[]{String.class}))`
				fixture = strings.Replace(fixture, "return "+expression+";", "return (HandlerResult)(Object)("+expression+");", 1)
			}
			if layout == "nested receivers" {
				fixture = `class HandlerPredicate {final boolean flag;HandlerPredicate(boolean flag){this.flag=flag;}boolean test(){HandlerSupport.trace+="L";return flag;}}` + fixture
				fixture = strings.Replace(fixture, "static boolean left(boolean value)", `static HandlerPredicate version(boolean value){return new HandlerPredicate(value);}static Class<?> scope(boolean value){trace+="R";return value?String.class:HandlerSupport.class;}static boolean left(boolean value)`, 1)
				fixture = strings.Replace(fixture, "HandlerSupport.left(a)&&HandlerSupport.right(b)", "HandlerSupport.version(a).test()&&HandlerSupport.scope(b).getClassLoader()==null", 1)
			}
			if layout == "reused handler slots" || layout == "nested reflection" {
				fixture = strings.Replace(fixture, "static RuntimeException failure;", "static boolean a,b;static int mode;static Class<?> first,second;static RuntimeException failure;", 1)
				fixture = strings.Replace(fixture, "run(boolean a,boolean b,int mode)", "run()", 1)
				fixture = strings.Replace(fixture, "HandlerSupport.start(mode);return new HandlerProduct(null);", `Class<?> first=Class.forName("java.lang.String");Class<?> second=Class.forName("java.lang.Object");HandlerSupport.first=first;HandlerSupport.second=second;HandlerSupport.start(HandlerSupport.mode);return new HandlerProduct(null);`, 1)
				fixture = strings.Replace(fixture, "HandlerSupport.left(a)&&HandlerSupport.right(b)", "HandlerSupport.left(HandlerSupport.a)&&HandlerSupport.right(HandlerSupport.b)", 1)
				fixture = strings.Replace(fixture, "HandlerResult result=HandlerArrayOwner.run(a,b,mode);", "HandlerSupport.a=a;HandlerSupport.b=b;HandlerSupport.mode=mode;HandlerResult result=HandlerArrayOwner.run();", 1)
			}
			if layout == "nested reflection" {
				fixture = `class HandlerFactory {public static java.lang.reflect.Method resolve(Class<?> owner,String name,Class<?>[] args)throws Exception{return owner.getDeclaredMethod(name,args);}}` + fixture
				fixture = strings.Replace(fixture, "HandlerProduct(java.lang.reflect.Method method)", `HandlerProduct(java.lang.reflect.Method method,java.lang.reflect.Method name,java.lang.reflect.Method params){this(method);if(!name.getName().equals("getName")||!params.getName().equals("getParameterTypes"))throw new AssertionError("earlier reflective operands");}HandlerProduct(java.lang.reflect.Method method)`, 1)
				fixture = strings.Replace(fixture, `Class<?> first=Class.forName("java.lang.String");Class<?> second=Class.forName("java.lang.Object");HandlerSupport.first=first;HandlerSupport.second=second;HandlerSupport.start(HandlerSupport.mode);return new HandlerProduct(null);`, `Class<?> first=Class.forName("HandlerFactory");Class<?> second=Class.forName("java.lang.reflect.Method");HandlerSupport.start(HandlerSupport.mode);return new HandlerProduct((java.lang.reflect.Method)first.getMethod("resolve",new Class<?>[]{Class.class,String.class,Class[].class}).invoke(null,new Object[]{HandlerSupport.class,"pick",new Class<?>[]{String.class}}),second.getMethod("getName",new Class<?>[0]),second.getMethod("getParameterTypes",new Class<?>[0]));`, 1)
				fixture = strings.Replace(fixture, "mode==0&&((HandlerProduct)result).method!=null", "mode==0&&(((HandlerProduct)result).method==null||((HandlerProduct)result).method.getParameterTypes()[0]!=String.class)", 1)
				fixture = strings.Replace(fixture, "public static HandlerResult run()", "public HandlerResult run()", 1)
				fixture = strings.Replace(fixture, "HandlerArrayOwner.run()", "new HandlerArrayOwner().run()", 1)
			}
			for _, release := range []int{5, 8} {
				t.Run(fmt.Sprintf("original%d", release), func(t *testing.T) {
					testNativePrivateSetterCompiledFixtureWithPolicies(t, "HandlerArrayOwner", "HandlerArrayDriver", "8:handler:array:lazy:binding:exceptions\n", func(t *testing.T, debug string) map[string][]byte {
						native := os.Getenv("JAVA8_JAVAC")
						if native == "" {
							t.Fatal("actual native compiler required")
						}
						files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, "HandlerArrayOwner", debug, nil, release, false)
						if int(mustParseLowering(t, files["HandlerArrayOwner.class"]).MajorVersion) != 44+release {
							t.Fatal("original compiler target mismatch")
						}
						return files
					}, nil, []string{"normal"})
				})
			}
		})
	}
}

// Restoring the array definition must not erase the CHECKCAST at the join.
// The unchanged driver observes successful, null and incompatible values on
// one arm, and the reflective allocation on the other arm.
func TestAdversarialHandlerConditionalArrayKeepsFailingJoinCast(t *testing.T) {
	const fixture = `interface CastResult {}
class CastProduct implements CastResult {final java.lang.reflect.Method method;CastProduct(java.lang.reflect.Method method){CastEffects.trace+="P";this.method=method;}}
class CastEffects {
 static String trace;static Throwable caught;static Object selected;
 static final CastResult fallback=new CastProduct(null),good=new CastProduct(null);
 static final Object incompatible=new Object();
 static void start(){trace+="S";throw new IllegalStateException();}
 static boolean left(boolean flag){trace+="L";return flag;}
 static boolean right(boolean flag){trace+="R";return flag;}
 static void target(String argument){}
}
public class CastArrayOwner {
 public static CastResult run(boolean a,boolean b){
  try{try{CastEffects.start();return null;}catch(Exception first){
   return (CastResult)(CastEffects.left(a)&&CastEffects.right(b)?CastEffects.selected:new CastProduct(CastEffects.class.getDeclaredMethod("target",new Class<?>[]{String.class})));
  }}catch(Exception second){CastEffects.caught=second;CastEffects.trace+="X";return CastEffects.fallback;}
 }
}
class CastArrayDriver {public static void main(String[] args){int rows=0;
 for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true})for(int mode=0;mode<3;mode++){
  CastEffects.trace="";CastEffects.caught=null;CastEffects.selected=mode==0?CastEffects.good:mode==1?CastEffects.incompatible:null;
  CastResult result=CastArrayOwner.run(a,b);boolean selected=a&&b;boolean fails=selected&&mode==1;
  String trace="SL"+(a?"R":"")+(selected?(fails?"X":""):"P");
  if(!trace.equals(CastEffects.trace))throw new AssertionError("evaluation order");
  if(fails){if(result!=CastEffects.fallback||!(CastEffects.caught instanceof ClassCastException))throw new AssertionError("original failed cast and handler");}
  else{if(CastEffects.caught!=null)throw new AssertionError("unexpected exception");
   if(selected){if(result!=CastEffects.selected)throw new AssertionError("null or object identity");}
   else if(!(result instanceof CastProduct)||((CastProduct)result).method.getParameterTypes()[0]!=String.class)throw new AssertionError("array binding");}
  rows++;
 }System.out.println(rows+":checked:join:null:failure:array");}}
`
	for _, release := range []int{5, 8} {
		t.Run(fmt.Sprintf("original%d", release), func(t *testing.T) {
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "CastArrayOwner", "CastArrayDriver", "12:checked:join:null:failure:array\n", func(t *testing.T, debug string) map[string][]byte {
				native := os.Getenv("JAVA8_JAVAC")
				if native == "" {
					t.Fatal("actual native compiler required")
				}
				files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, "CastArrayOwner", debug, nil, release, false)
				if int(mustParseLowering(t, files["CastArrayOwner.class"]).MajorVersion) != 44+release {
					t.Fatal("original compiler target mismatch")
				}
				return files
			}, nil, []string{"normal"})
		})
	}
}
