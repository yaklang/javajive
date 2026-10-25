package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialSwitchContinueRetainsConditionalEffectTail(t *testing.T) {
	const fixture = `public class SwitchTailSubject {
 public static int sum(int[] tags,int mask){int sum=0;for(int i=0;i<tags.length;i++){int value;switch(tags[i]){case 0:value=i+1;break;case 1:if((i&1)==0)continue;else throw SwitchTailTrace.FAILURE;case 2:value=0;break;default:throw new IllegalArgumentException();}if(value==0&&(mask&1)!=0)continue;SwitchTailTrace.calls++;sum+=value;if(sum>5&&(mask&2)!=0)sum=3;}return sum;}
}
class SwitchTailTrace {static int calls;static final RuntimeException FAILURE=new IllegalStateException("same");}
class SwitchTailDriver {public static void main(String[] args){
 for(int mask=0;mask<4;mask++){
  SwitchTailTrace.calls=0;int a=SwitchTailSubject.sum(new int[]{0,0,2,0},mask);if(a!=((mask&2)==0?7:3)||SwitchTailTrace.calls!=((mask&1)==0?4:3))throw new AssertionError("normal tail "+mask+":"+a+":"+SwitchTailTrace.calls);
  SwitchTailTrace.calls=0;int b=SwitchTailSubject.sum(new int[]{1,0,2,0},mask);if(b!=((mask&2)==0?6:3)||SwitchTailTrace.calls!=((mask&1)==0?3:2))throw new AssertionError("continue tail "+mask+":"+b+":"+SwitchTailTrace.calls);
 }
 SwitchTailTrace.calls=0;try{SwitchTailSubject.sum(new int[]{0,1},0);throw new AssertionError("missing error");}catch(RuntimeException failure){if(failure!=SwitchTailTrace.FAILURE||SwitchTailTrace.calls!=1)throw new AssertionError("failure identity/effects");}
 System.out.println("9:switch:continue:normal-tail:effects:exception-identity");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "SwitchTailSubject", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"SwitchTailSubject"}, "SwitchTailDriver", "9:switch:continue:normal-tail:effects:exception-identity\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialSwitchContinueRetainsMapWriteAndAliasTail(t *testing.T) {
	const fixture = `import java.util.*;
public class SwitchMapSubject {
 public static void read(Map target,int[] tags,long flags){Map output=target;if(target instanceof SwitchMapWrapper)output=(Map)((SwitchMapWrapper)target).unwrap(Map.class);for(int i=0;;i++){
  if(i>=tags.length)return;
  Object value;switch(tags[i]){case 0:value=Integer.valueOf(i+1);break;case 1:if((i&1)==0)continue;else throw SwitchMapTrace.FAILURE;case 2:value=null;break;default:throw new IllegalArgumentException();}
  if(value==null&&(flags&1L)!=0)continue;
  Object old=output.put(Integer.valueOf(i&1),value);
  if(old!=null&&(flags&2L)!=0){if(old instanceof Collection){((Collection)old).add(value);output.put(Integer.valueOf(i&1),old);}else{ArrayList values=new ArrayList();values.add(old);values.add(value);output.put(Integer.valueOf(i&1),values);}}
 }}
}
interface SwitchMapWrapper {Object unwrap(Class type);}
class SwitchMapTrace extends LinkedHashMap {static final RuntimeException FAILURE=new IllegalStateException("same");int calls;public Object put(Object k,Object v){calls++;return super.put(k,v);}}
class SwitchMapBox extends LinkedHashMap implements SwitchMapWrapper {final Map actual;SwitchMapBox(Map actual){this.actual=actual;}public Object unwrap(Class type){if(type!=Map.class)throw new AssertionError();return actual;}}
class SwitchMapDriver {public static void main(String[] args){
 String[] full={"{0=5, 1=4}","{0=5, 1=4}","{0=[1, null, 5], 1=[2, 4]}","{0=[1, 5], 1=[2, 4]}"};int[] calls={5,4,8,6};
 String[] skipped={"{1=4, 0=5}","{1=4, 0=5}","{1=[2, 4], 0=5}","{1=[2, 4], 0=5}"};int[] skippedCalls={4,3,5,4};
 for(boolean boxed:new boolean[]{false,true})for(int flags=0;flags<4;flags++)for(boolean skip:new boolean[]{false,true}){
  SwitchMapTrace actual=new SwitchMapTrace();Map target=boxed?new SwitchMapBox(actual):actual;
  SwitchMapSubject.read(target,skip?new int[]{1,0,2,0,0}:new int[]{0,0,2,0,0},flags);
  if(!actual.toString().equals((skip?skipped:full)[flags])||actual.calls!=(skip?skippedCalls:calls)[flags]||(boxed&&!target.isEmpty()))throw new AssertionError("tail "+boxed+":"+flags+":"+skip+":"+actual+":"+actual.calls);
 }
 System.out.println("16:map:continue:null:alias:duplicate-effects");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "SwitchMapSubject", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"SwitchMapSubject"}, "SwitchMapDriver", "16:map:continue:null:alias:duplicate-effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialSwitchContinueRetainsGuardedMapTail(t *testing.T) {
	const fixture = `import java.util.*;
public class GuardMapSubject {
 public static void read(Map target,int[] tags,long flags){Map output=target;if(target instanceof GuardMapWrapper)output=(Map)((GuardMapWrapper)target).unwrap(Map.class);for(int i=0;;i++){
  if(i>=tags.length)break;
  if(i!=0 && tags.length==0)throw GuardMapTrace.FAILURE; else {
  Object value;switch(tags[i]){case 0:value=Integer.valueOf(i+1);break;case 1:if((i&1)==0)continue;else throw GuardMapTrace.FAILURE;case 2:value=null;break;default:throw new IllegalArgumentException();}
  if(value==null&&(flags&1L)!=0)continue;
  Object old=output.put(Integer.valueOf(i&1),value);
  if(old!=null&&(flags&2L)!=0){if(old instanceof Collection){((Collection)old).add(value);output.put(Integer.valueOf(i&1),old);}else{ArrayList values=new ArrayList();values.add(old);values.add(value);output.put(Integer.valueOf(i&1),values);}}
 }}GuardMapTrace.ends++;}
}
interface GuardMapWrapper {Object unwrap(Class type);}
class GuardMapTrace extends LinkedHashMap {static final RuntimeException FAILURE=new IllegalStateException("same");static int ends;int calls;public Object put(Object k,Object v){calls++;return super.put(k,v);}}
class GuardMapBox extends LinkedHashMap implements GuardMapWrapper {final Map actual;GuardMapBox(Map actual){this.actual=actual;}public Object unwrap(Class type){if(type!=Map.class)throw new AssertionError();return actual;}}
class GuardMapDriver {public static void main(String[] args){
 String[] full={"{0=5, 1=4}","{0=5, 1=4}","{0=[1, null, 5], 1=[2, 4]}","{0=[1, 5], 1=[2, 4]}"};int[] calls={5,4,8,6};
 String[] skipped={"{1=4, 0=5}","{1=4, 0=5}","{1=[2, 4], 0=5}","{1=[2, 4], 0=5}"};int[] skippedCalls={4,3,5,4};
 for(boolean boxed:new boolean[]{false,true})for(int flags=0;flags<4;flags++)for(boolean skip:new boolean[]{false,true}){
  GuardMapTrace.ends=0;GuardMapTrace actual=new GuardMapTrace();Map target=boxed?new GuardMapBox(actual):actual;
  GuardMapSubject.read(target,skip?new int[]{1,0,2,0,0}:new int[]{0,0,2,0,0},flags);
  if(GuardMapTrace.ends!=1)throw new AssertionError("end");
  if(!actual.toString().equals((skip?skipped:full)[flags])||actual.calls!=(skip?skippedCalls:calls)[flags]||(boxed&&!target.isEmpty()))throw new AssertionError("tail "+boxed+":"+flags+":"+skip+":"+actual+":"+actual.calls);
 }
 System.out.println("16:map:continue:null:alias:duplicate-effects");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "GuardMapSubject", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"GuardMapSubject"}, "GuardMapDriver", "16:map:continue:null:alias:duplicate-effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialSwitchContinueRetainsEffectfulContinueMapTail(t *testing.T) {
	const fixture = `import java.util.*;
public class EffectMapSubject {
 public static void read(Map target,int[] tags,long flags){Map output=target;if(target instanceof EffectMapWrapper)output=(Map)((EffectMapWrapper)target).unwrap(Map.class);for(int i=0;;i++){
  if(i>=tags.length)break;
  if(i!=0 && tags.length==0)throw EffectMapTrace.FAILURE; else {
  Object value;switch(tags[i]){case 0:value=Integer.valueOf(i+1);break;case 1:if((i&1)==0){EffectMapTrace.skips++;continue;}else throw EffectMapTrace.FAILURE;case 2:value=null;break;default:throw new IllegalArgumentException();}
  if(value==null&&(flags&1L)!=0)continue;
  Object old=output.put(Integer.valueOf(i&1),value);
  if(old!=null&&(flags&2L)!=0){if(old instanceof Collection){((Collection)old).add(value);output.put(Integer.valueOf(i&1),old);}else{ArrayList values=new ArrayList();values.add(old);values.add(value);output.put(Integer.valueOf(i&1),values);}}
 }}EffectMapTrace.ends++;}
}
interface EffectMapWrapper {Object unwrap(Class type);}
class EffectMapTrace extends LinkedHashMap {static final RuntimeException FAILURE=new IllegalStateException("same");static boolean failPut;static int skips;static int ends;int calls;public Object put(Object k,Object v){calls++;if(failPut)throw FAILURE;return super.put(k,v);}}
class EffectMapBox extends LinkedHashMap implements EffectMapWrapper {final Map actual;EffectMapBox(Map actual){this.actual=actual;}public Object unwrap(Class type){if(type!=Map.class)throw new AssertionError();return actual;}}
class EffectMapDriver {public static void main(String[] args){
 String[] full={"{0=5, 1=4}","{0=5, 1=4}","{0=[1, null, 5], 1=[2, 4]}","{0=[1, 5], 1=[2, 4]}"};int[] calls={5,4,8,6};
 String[] skipped={"{1=4, 0=5}","{1=4, 0=5}","{1=[2, 4], 0=5}","{1=[2, 4], 0=5}"};int[] skippedCalls={4,3,5,4};
 for(boolean boxed:new boolean[]{false,true})for(int flags=0;flags<4;flags++)for(boolean skip:new boolean[]{false,true}){
  EffectMapTrace.ends=0;EffectMapTrace.skips=0;EffectMapTrace actual=new EffectMapTrace();Map target=boxed?new EffectMapBox(actual):actual;
  EffectMapSubject.read(target,skip?new int[]{1,0,2,0,0}:new int[]{0,0,2,0,0},flags);
  if(EffectMapTrace.ends!=1||EffectMapTrace.skips!=(skip?1:0))throw new AssertionError("end");
  if(!actual.toString().equals((skip?skipped:full)[flags])||actual.calls!=(skip?skippedCalls:calls)[flags]||(boxed&&!target.isEmpty()))throw new AssertionError("tail "+boxed+":"+flags+":"+skip+":"+actual+":"+actual.calls);
 }
 EffectMapTrace actual=new EffectMapTrace();EffectMapTrace.skips=0;EffectMapTrace.ends=0;
 try{EffectMapSubject.read(actual,new int[]{0,1,0},0);throw new AssertionError("missing case failure");}catch(RuntimeException e){if(e!=EffectMapTrace.FAILURE||actual.calls!=1||!actual.toString().equals("{0=1}")||EffectMapTrace.ends!=0||EffectMapTrace.skips!=0)throw new AssertionError("case failure identity/effects");}
 actual=new EffectMapTrace();EffectMapTrace.failPut=true;
 try{EffectMapSubject.read(actual,new int[]{0,0},0);throw new AssertionError("missing tail failure");}catch(RuntimeException e){if(e!=EffectMapTrace.FAILURE||actual.calls!=1||!actual.isEmpty()||EffectMapTrace.ends!=0||EffectMapTrace.skips!=0)throw new AssertionError("tail failure identity/effects");}finally{EffectMapTrace.failPut=false;}
 System.out.println("18:map:continue:null:alias:duplicate-effects:failures");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "EffectMapSubject", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"EffectMapSubject"}, "EffectMapDriver", "18:map:continue:null:alias:duplicate-effects:failures\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialSwitchContinueRetainsDescendingEffectfulContinueMapTail(t *testing.T) {
	const forward = `import java.util.*;
public class EffectMapSubject {
 public static void read(Map target,int[] tags,long flags){Map output=target;if(target instanceof EffectMapWrapper)output=(Map)((EffectMapWrapper)target).unwrap(Map.class);for(int i=0;;i++){
  if(i>=tags.length)break;
  if(i!=0 && tags.length==0)throw EffectMapTrace.FAILURE; else {
  Object value;switch(tags[i]){case 0:value=Integer.valueOf(i+1);break;case 1:if((i&1)==0){EffectMapTrace.skips++;continue;}else throw EffectMapTrace.FAILURE;case 2:value=null;break;default:throw new IllegalArgumentException();}
  if(value==null&&(flags&1L)!=0)continue;
  Object old=output.put(Integer.valueOf(i&1),value);
  if(old!=null&&(flags&2L)!=0){if(old instanceof Collection){((Collection)old).add(value);output.put(Integer.valueOf(i&1),old);}else{ArrayList values=new ArrayList();values.add(old);values.add(value);output.put(Integer.valueOf(i&1),values);}}
 }}EffectMapTrace.ends++;}
}
interface EffectMapWrapper {Object unwrap(Class type);}
class EffectMapTrace extends LinkedHashMap {static final RuntimeException FAILURE=new IllegalStateException("same");static boolean failPut;static int skips;static int ends;int calls;public Object put(Object k,Object v){calls++;if(failPut)throw FAILURE;return super.put(k,v);}}
class EffectMapBox extends LinkedHashMap implements EffectMapWrapper {final Map actual;EffectMapBox(Map actual){this.actual=actual;}public Object unwrap(Class type){if(type!=Map.class)throw new AssertionError();return actual;}}
class EffectMapDriver {public static void main(String[] args){
 String[] full={"{0=5, 1=4}","{0=5, 1=4}","{0=[1, null, 5], 1=[2, 4]}","{0=[1, 5], 1=[2, 4]}"};int[] calls={5,4,8,6};
 String[] skipped={"{1=4, 0=5}","{1=4, 0=5}","{1=[2, 4], 0=5}","{1=[2, 4], 0=5}"};int[] skippedCalls={4,3,5,4};
 for(boolean boxed:new boolean[]{false,true})for(int flags=0;flags<4;flags++)for(boolean skip:new boolean[]{false,true}){
  EffectMapTrace.ends=0;EffectMapTrace.skips=0;EffectMapTrace actual=new EffectMapTrace();Map target=boxed?new EffectMapBox(actual):actual;
  EffectMapSubject.read(target,skip?new int[]{1,0,2,0,0}:new int[]{0,0,2,0,0},flags);
  if(EffectMapTrace.ends!=1||EffectMapTrace.skips!=(skip?1:0))throw new AssertionError("end");
  if(!actual.toString().equals((skip?skipped:full)[flags])||actual.calls!=(skip?skippedCalls:calls)[flags]||(boxed&&!target.isEmpty()))throw new AssertionError("tail "+boxed+":"+flags+":"+skip+":"+actual+":"+actual.calls);
 }
 EffectMapTrace actual=new EffectMapTrace();EffectMapTrace.skips=0;EffectMapTrace.ends=0;
 try{EffectMapSubject.read(actual,new int[]{0,1,0},0);throw new AssertionError("missing case failure");}catch(RuntimeException e){if(e!=EffectMapTrace.FAILURE||actual.calls!=1||!actual.toString().equals("{0=1}")||EffectMapTrace.ends!=0||EffectMapTrace.skips!=0)throw new AssertionError("case failure identity/effects");}
 actual=new EffectMapTrace();EffectMapTrace.failPut=true;
 try{EffectMapSubject.read(actual,new int[]{0,0},0);throw new AssertionError("missing tail failure");}catch(RuntimeException e){if(e!=EffectMapTrace.FAILURE||actual.calls!=1||!actual.isEmpty()||EffectMapTrace.ends!=0||EffectMapTrace.skips!=0)throw new AssertionError("tail failure identity/effects");}finally{EffectMapTrace.failPut=false;}
 System.out.println("18:map:continue:null:alias:duplicate-effects:failures");}}
`
	fixture := strings.NewReplacer("EffectMap", "DescendingMap", "for(int i=0;;i++)", "for(int i=tags.length-1;;i--)", "if(i>=tags.length)", "if(i<0)",
		"{0=5, 1=4}", "{0=1, 1=2}", "{0=[1, null, 5], 1=[2, 4]}", "{0=[5, null, 1], 1=[4, 2]}", "{0=[1, 5], 1=[2, 4]}", "{0=[5, 1], 1=[4, 2]}",
		"String[] skipped={\"{1=4, 0=5}\",\"{1=4, 0=5}\",\"{1=[2, 4], 0=5}\",\"{1=[2, 4], 0=5}\"}", "String[] skipped={\"{0=null, 1=2}\",\"{0=5, 1=2}\",\"{0=[5, null], 1=[4, 2]}\",\"{0=5, 1=[4, 2]}\"}",
		"int[] skippedCalls={4,3,5,4}", "int[] skippedCalls={4,3,6,4}", "{0=1}", "{0=3}").Replace(forward)

	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "DescendingMapSubject", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"DescendingMapSubject"}, "DescendingMapDriver", "18:map:continue:null:alias:duplicate-effects:failures\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}
