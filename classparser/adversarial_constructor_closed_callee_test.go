package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A parent's private/final body may update its own independent storage without
// observing a child's pre-super capture. Keep the parent, effects and driver
// as original classfiles so a wrong motion cannot rewrite its own oracle.
const closedCalleeFixture = `
class ClosedEffects {static String trace;static final RuntimeException failure=new RuntimeException("original");static void mark(){trace+="M";}}
class ClosedParent {
 int count,result;long wide;Object this$0,reference;
 ClosedParent(int n,long w,Object input,boolean fail){ClosedEffects.trace+="P";CALL;ClosedEffects.trace+="Q";}
 METHOD
 Object captured(){return null;}
}
class ClosedOwner {final Object token;ClosedOwner(Object t){token=t;}final class Child extends ClosedParent{Child(int n,long w,Object input,boolean fail){super(n,w,input,fail);}Object captured(){return ClosedOwner.this.token;}}ClosedParent make(int n,long w,Object input,boolean fail){return new Child(n,w,input,fail);}}
class ClosedDriver {public static void main(String[]args)throws Exception{int rows=0;Object token=new Object();
 for(Object capture:new Object[]{null,token})for(Object input:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-3,-1,0,1,3,7,Integer.MAX_VALUE})for(long w:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){
 ClosedEffects.trace="";ClosedOwner owner=new ClosedOwner(capture);
 try{ClosedParent p=owner.make(n,w,input,fail);if(fail||p.count!=n||p.wide!=w||p.this$0!=input||p.captured()!=capture||!ClosedEffects.trace.equals("PMQ")||RESULT_CHECK)throw new AssertionError("parent storage, result, capture or effect order");java.lang.reflect.Field f=p.getClass().getDeclaredField("this$0");f.setAccessible(true);if(f.get(p)!=owner||f.getType()!=ClosedOwner.class)throw new AssertionError("separate physical capture storage");}
 catch(RuntimeException ex){if(!fail||ex!=ClosedEffects.failure||!ClosedEffects.trace.equals("PM"))throw new AssertionError("original exception identity and effect prefix",ex);}
 rows++;}System.out.println(rows+":closed:method:capture:effects");}}
`

var closedCalleeBanks = []struct{ name, call, method, check string }{
	{"void mutation", "prepare(n,w,input,fail)", "private void prepare(int n,long w,Object input,boolean fail){count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;}", "p.result!=0||p.reference!=null"},
	{"primitive return branches", "result=prepare(n,w,input,fail)", "final int prepare(int n,long w,Object input,boolean fail){count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;return n<0?-n:n;}", "p.result!=(n<0?-n:n)||p.reference!=null"},
	{"reference return branches", "reference=prepare(n,w,input,fail)", "private Object prepare(int n,long w,Object input,boolean fail){count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;return n<0?input:null;}", "p.reference!=(n<0?input:null)||p.result!=0"},
	{"nested exact callees", "prepare(n,w,input,fail)", "private void prepare(int n,long w,Object input,boolean fail){count=normalize(n);wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;}private int normalize(int n){result=n^7;return n;}", "p.result!=(n^7)||p.reference!=null"},
	{"callee loop invariant", "result=prepare(n,w,input,fail)", "private int prepare(int n,long w,Object input,boolean fail){count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure;int total=0;for(int i=0;i<(n&7);i++)total+=i;return total;}", "p.result!=((n&7)*((n&7)-1)/2)||p.reference!=null"},
}

func TestAdversarialConstructorClosedInstanceCalleesPreserveCaptureAndEffects(t *testing.T) {
	for _, row := range closedCalleeBanks {
		t.Run(row.name, func(t *testing.T) {
			for _, prefix := range []string{"Closed", "Ledger"} {
				t.Run(prefix, func(t *testing.T) {
					f := strings.ReplaceAll(closedCalleeFixture, "CALL", row.call)
					f = strings.ReplaceAll(f, "METHOD", row.method)
					f = strings.ReplaceAll(f, "RESULT_CHECK", row.check)
					f = strings.ReplaceAll(f, "Closed", prefix)
					testIndependentFlatClosedCalleeFamily(t, f, prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"})
				})
			}
		})
	}
}

// The same original family must also retain native declaring ownership, ABI
// and generic signatures, independently of the flat fallback's motion proof.
func TestNativeConstructorClosedCalleesPreserveLexicalOwnership(t *testing.T) {
	for _, row := range closedCalleeBanks {
		t.Run(row.name, func(t *testing.T) {
			for _, prefix := range []string{"Closed", "Ledger"} {
				t.Run(prefix, func(t *testing.T) {
					fixture := strings.ReplaceAll(closedCalleeFixture, "CALL", row.call)
					fixture = strings.ReplaceAll(fixture, "METHOD", row.method)
					fixture = strings.ReplaceAll(fixture, "RESULT_CHECK", row.check)
					fixture = strings.ReplaceAll(fixture, "Closed", prefix)
					testNativeIndependentFamilyFixture(t, fixture, []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}

// A private entry does not seal the virtual calls inside its body. The
// original superclass observes the child's early capture through an override.
// Selecting the parent's same-signature body would incorrectly certify motion.
const closedMethodObservationFixture = `
class ClosedObserveParent {Object seen;ClosedObserveParent(){observe();}private void observe(){seen=captured();}Object captured(){return null;}}
class ClosedObserveOwner {final Object token;ClosedObserveOwner(Object t){token=t;}final class Child extends ClosedObserveParent{Child(){super();}Object captured(){return ClosedObserveOwner.this.token;}}ClosedObserveParent make(){return new Child();}}
class ClosedObserveDriver {public static void main(String[]args){int rows=0;Object token=new Object();for(Object value:new Object[]{null,token}){try{ClosedObserveParent p=new ClosedObserveOwner(value).make();if(p.seen!=value||p.captured()!=value)throw new AssertionError("early capture observation");}catch(NullPointerException ex){throw new AssertionError("early capture unavailable during original callback",ex);}rows++;}System.out.println(rows+":open:dispatch:observation");}}
`

func TestAdversarialConstructorClosedMethodCannotSubstituteOpenDispatch(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"ClosedObserve", "BoundObserve"} {
		t.Run(prefix, func(t *testing.T) {
			fixture := strings.ReplaceAll(closedMethodObservationFixture, "ClosedObserve", prefix)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != "2:open:dispatch:observation\n" {
						t.Fatal("original callback oracle", got)
					}
					resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
								t.Run(string(mode), func(t *testing.T) {
									var source string
									var err error
									raw := bytes.Clone(files[prefix+"Owner$Child.class"])
									if mode == "legacy" {
										source, err = DecompileWithResolver(raw, resolve)
									} else {
										var result DecompileResult
										result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
										source = result.Source
									}
									if err != nil || !strings.Contains(source, DecompileStubMarker) {
										t.Fatalf("open callback cannot acquire a closed-body certificate: %v\n%s", err, source)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func testIndependentFlatClosedCalleeFamily(t *testing.T, fixture, prefix, expected string, owners []string) {
	t.Helper()
	testIndependentFlatMutatedClosedCalleeFamily(t, fixture, prefix, expected, owners, nil)
}

func testIndependentFlatMutatedClosedCalleeFamily(t *testing.T, fixture, prefix, expected string, owners []string, mutate func(*testing.T, map[string][]byte)) {
	t.Helper()
	testIndependentFlatCompilerClosedCalleeFamily(t, func(debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, prefix, expected, owners, mutate)
}
