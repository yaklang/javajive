package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A precise resource rethrow is still the original catch parameter after a
// monitor/finally region. Keep both resource suppression and the enclosing
// anonymous allocation; moving its captures after Thread's constructor would
// be a different proof. The binary driver is never decompiled or recompiled.
const anonymousResourceRethrowFixture = `
class ResourceTaskEffects {
 static int mode;
 static String trace;
 static Object expectedLock;
 static final java.io.IOException first=new java.io.IOException("first");
 static final java.io.IOException second=new java.io.IOException("second");
 static final java.io.IOException read=new java.io.IOException("read");
 static final java.io.IOException outClose=new java.io.IOException("out-close");
 static final java.io.IOException inClose=new java.io.IOException("in-close");
 static final RuntimeException body=new IllegalStateException("body");
 static final RuntimeException finish=new IllegalArgumentException("finish");
 static void outside(String step){if(Thread.holdsLock(expectedLock))throw new AssertionError("monitor held during "+step);trace+=step;}
 static ResourceTaskResource open(int id)throws java.io.IOException {
  outside(id==0?"A":"B");
  if(mode==id+1)throw id==0?first:second;
  return new ResourceTaskResource(id);
 }
 static void finish(){outside("F");if(mode==10)throw finish;}
}
class ResourceTaskResource implements AutoCloseable {
 final int id;
 ResourceTaskResource(int n){id=n;}
 int read()throws java.io.IOException {
  ResourceTaskEffects.outside("R");
  if(ResourceTaskEffects.mode==3||ResourceTaskEffects.mode==8)throw ResourceTaskEffects.read;
  return 1;
 }
 public void close()throws java.io.IOException {
  ResourceTaskEffects.outside(id==0?"I":"O");
  int mode=ResourceTaskEffects.mode;
  if((id==0&&(mode==6||mode==7||mode==8||mode==9))||(id==1&&(mode==5||mode==7||mode==8||mode==9)))throw id==0?ResourceTaskEffects.inClose:ResourceTaskEffects.outClose;
 }
}
class ResourceTaskOwner {
 static Thread make(final Object lock,final int[] state,final Object token) {
  return new Thread() {
   public void run() {
    try {
     try(ResourceTaskResource in=ResourceTaskEffects.open(0);ResourceTaskResource out=ResourceTaskEffects.open(1)) {
      int command=in.read();
      synchronized(lock) {
       ResourceTaskEffects.trace+="L";
       if(state[0]==-2)return;
       if(ResourceTaskEffects.mode==4)throw ResourceTaskEffects.body;
       state[0]+=command;
       if(token!=lock)throw new AssertionError("capture identity");
      }
      ResourceTaskEffects.outside("W");
     }
    }catch(RuntimeException|Error failure) {
     throw failure;
    }catch(Exception failure) {
     throw new RuntimeException(failure);
    }finally {
     ResourceTaskEffects.finish();
    }
   }
  };
 }
}
class ResourceTaskDriver {
 public static void main(String[]args) {
  int rows=0;
  for(int mode=0;mode<=10;mode++)for(boolean early:new boolean[]{false,true}) {
   ResourceTaskEffects.mode=mode;ResourceTaskEffects.trace="";
   Object lock=new Object();int initial=early?-2:17;int[] state={initial};
   ResourceTaskEffects.expectedLock=lock;
   Thread task=ResourceTaskOwner.make(lock,state,lock);
   if(!task.getClass().isAnonymousClass()||!task.getClass().getEnclosingMethod().getName().equals("make")||task.getClass().getEnclosingClass()!=ResourceTaskOwner.class)throw new AssertionError("anonymous ownership");
   Throwable actual=null;
   try{task.run();}catch(Throwable failure){actual=failure;}
   Throwable primary=null;String prefix;
   if(mode==1){primary=ResourceTaskEffects.first;prefix="A";}
   else if(mode==2){primary=ResourceTaskEffects.second;prefix="ABI";}
   else if(mode==3||mode==8){primary=ResourceTaskEffects.read;prefix="ABROI";}
   else {
    prefix="ABRL"+(early||mode==4?"":"W")+"OI";
    if(!early&&mode==4)primary=ResourceTaskEffects.body;
    else if(mode==5||mode==7||mode==9)primary=ResourceTaskEffects.outClose;
    else if(mode==6)primary=ResourceTaskEffects.inClose;
   }
   if(!ResourceTaskEffects.trace.equals(prefix+"F"))throw new AssertionError("order "+mode+":"+ResourceTaskEffects.trace);
   if(mode==10) {
    if(actual!=ResourceTaskEffects.finish)throw new AssertionError("finally override identity");
   }else if(primary instanceof RuntimeException) {
    if(actual!=primary)throw new AssertionError("unchecked rethrow identity");
   }else if(primary!=null) {
    if(actual==null||actual.getClass()!=RuntimeException.class||actual.getCause()!=primary)throw new AssertionError("checked wrapping identity "+mode);
    Throwable[] suppressed=primary.getSuppressed();
    int per=mode==8?2:(mode==7||mode==9?1:0);
    // Throwable singletons accumulate suppression across the two independent
    // early-return worlds. Verify the new suffix rather than clearing it.
    int before=mode==8?(early?2:0):(mode==7||mode==9?(mode==9?2:0)+(early?1:0):0);
    if(per>0) {
     if(suppressed.length!=before+per||suppressed[before]!=(mode==8?ResourceTaskEffects.outClose:ResourceTaskEffects.inClose)||per==2&&suppressed[before+1]!=ResourceTaskEffects.inClose)throw new AssertionError("resource suppression identity/order "+mode);
    }
   }else if(actual!=null)throw new AssertionError("unexpected failure",actual);
   int expected=initial+(!early&&(mode==0||mode==5||mode==6||mode==7||mode==9||mode==10)?1:0);
   if(state[0]!=expected)throw new AssertionError("monitor/capture state "+mode);
   rows++;
  }
  System.out.println(rows+":anonymous:monitor:resources:precise-rethrow:identity:suppression");
 }
}`

func TestAdversarialAnonymousResourceRethrowPreservesSuppressionAndCaptures(t *testing.T) {
	testAnonymousResourceRethrowFixture(t, false, false, false)
}

func TestAdversarialAnonymousInitializerWithResourceRethrowKeepsSourceBoundary(t *testing.T) {
	testAnonymousResourceRethrowFixture(t, true, false, false)
}

func TestAdversarialAnonymousResourceLoopReturnClosesOnce(t *testing.T) {
	testAnonymousResourceRethrowFixture(t, false, true, false)
}

func TestAdversarialAnonymousResourceLoopFinallyOwnsArrayCleanup(t *testing.T) {
	testAnonymousResourceRethrowFixture(t, false, true, true)
}

func testAnonymousResourceRethrowFixture(t *testing.T, initializer, loop, arrayCleanup bool) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"ResourceTaskOwner", "DifferentResourceTaskOwner"} {
			t.Run(compiler.name+"/"+owner, func(t *testing.T) {
				source := anonymousResourceRethrowFixture
				if loop {
					// A return from one iteration exits the method, whereas a break
					// or back edge remains inside the resource/finally scopes. A
					// second read reaches EOF; it must not replay either close.
					source = strings.Replace(source, " final int id;", " final int id;int reads;", 1)
					source = strings.Replace(source, "  return 1;", "  return reads++==0?1:-1;", 1)
					source = strings.Replace(source, "      int command=in.read();", "     while(true){\n      int command=in.read();\n      if(command<0)break;", 1)
					source = strings.Replace(source, "      ResourceTaskEffects.outside(\"W\");", "      ResourceTaskEffects.outside(\"W\");\n     }", 1)
					source = strings.Replace(source, "?\"\":\"W\")", "?\"\":\"WR\")", 1)
				}
				if arrayCleanup {
					// Each original finally copy allocates a fresh argument array.
					// Their local names may differ; the allocation/store/invocation
					// identities and captured element must remain the same protocol.
					source = strings.Replace(source, "static void finish(){", "static void finish(Object... tokens){if(tokens.length!=1||tokens[0]!=expectedLock)throw new AssertionError(\"cleanup argument identity\");", 1)
					source = strings.Replace(source, "ResourceTaskEffects.finish();", "ResourceTaskEffects.finish(new Object[]{lock});", 1)
				}
				if initializer {
					// The same protected body also occurs after an original instance
					// initializer (EnclosingMethod index zero). A spurious escape
					// wrapper must not prevent that initializer's source recovery.
					source = strings.Replace(source, "class ResourceTaskOwner {\n static Thread make(final Object lock,final int[] state,final Object token) {\n  return new Thread() {\n   public void run() {", "interface ResourceTaskMarker {Object owner();}\nclass ResourceTaskOwner {\n final ResourceTaskMarker marker=new ResourceTaskMarker(){public Object owner(){return ResourceTaskOwner.this;}};\n ResourceTaskOwner(final Object lock,final int[] state,final Object token) {", 1)
					source = strings.Replace(source, "   }\n  };\n }\n}\nclass ResourceTaskDriver", " }\n}\nclass ResourceTaskDriver", 1)
					source = strings.Replace(source, "   Thread task=ResourceTaskOwner.make(lock,state,lock);\n   if(!task.getClass().isAnonymousClass()||!task.getClass().getEnclosingMethod().getName().equals(\"make\")||task.getClass().getEnclosingClass()!=ResourceTaskOwner.class)throw new AssertionError(\"anonymous ownership\");\n   Throwable actual=null;\n   try{task.run();}catch(Throwable failure){actual=failure;}", "   ResourceTaskOwner task=null;Throwable actual=null;\n   try{task=new ResourceTaskOwner(lock,state,lock);}catch(Throwable failure){actual=failure;}\n   if(task!=null&&(task.marker.owner()!=task||!task.marker.getClass().isAnonymousClass()||task.marker.getClass().getEnclosingMethod()!=null||task.marker.getClass().getEnclosingConstructor()!=null||task.marker.getClass().getEnclosingClass()!=ResourceTaskOwner.class))throw new AssertionError(\"initializer ownership\");", 1)
				}
				source = strings.ReplaceAll(source, "ResourceTaskOwner", owner)
				compile := func(debug string) map[string][]byte {
					if compiler.profile == NativeJavac8 {
						return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
					}
					return nativeCompileDebugClasses(t, source, debug)
				}
				testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, "ResourceTaskDriver", "22:anonymous:monitor:resources:precise-rethrow:identity:suppression\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
			})
		}
	}
}
