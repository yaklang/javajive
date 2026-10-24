package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Cleanup must remain outside the body's handlers. Its normal completion may
// reach an original continuation whose allocation and class initialization
// cannot occur before finally, even when a cleanup failure masks the throw.
const conditionalFinallyContinuationFixture = `class PriorityReader {
 static int read(int mode,int cleanup,int version)throws Throwable {
  Throwable prior=null;
  try { PriorityEffects.body(mode);return 29; }
  catch(Throwable failure){prior=failure;PriorityEffects.trace+="C";}
  finally {if(version>=7)PriorityEffects.footer(prior,cleanup);else throw PriorityEffects.rethrow(prior,cleanup);}
  throw new PriorityAfterFailure("after");
 }
}
class PriorityEffects {
 static String trace="",initializationTrace;static int allocations,initializations;
 static final Throwable io=new java.io.IOException("io"),runtime=new IllegalArgumentException("runtime"),error=new AssertionError("error"),cleanup=new IllegalStateException("cleanup"),missing=new UnsupportedOperationException("missing");
 static void body(int mode)throws Throwable{trace+="B";if(mode==1)throw io;if(mode==2)throw runtime;if(mode==3)throw error;}
 static void footer(Throwable prior,int mode)throws Throwable{trace+="F";if(mode==1)throw cleanup;if(mode!=2&&prior!=null)throw prior;}
 static RuntimeException rethrow(Throwable prior,int mode)throws Throwable{trace+="F";if(mode!=0)throw cleanup;if(prior!=null)throw prior;throw missing;}
}
class PriorityAfterFailure extends Error {
 static { PriorityEffects.initializations++;PriorityEffects.initializationTrace=PriorityEffects.trace; }
 PriorityAfterFailure(String text){super(text);if(!PriorityEffects.trace.endsWith("F"))throw new AssertionError("continuation ran before cleanup");PriorityEffects.allocations++;PriorityEffects.trace+="A";}
}
class PriorityDriver {
 public static void main(String[]args){int rows=0,initialized=0;for(int version:new int[]{Integer.MIN_VALUE,-1,0,6,7,8,Integer.MAX_VALUE})for(int mode=0;mode<4;mode++)for(int cleanup=0;cleanup<3;cleanup++){
  PriorityEffects.trace="";PriorityEffects.allocations=0;Object actual=null;try{actual=Integer.valueOf(PriorityReader.read(mode,cleanup,version));}catch(Throwable failure){actual=failure;}
  boolean after=version>=7&&cleanup==2&&mode!=0;
  Object expected=cleanup==1||(version<7&&cleanup!=0)?PriorityEffects.cleanup:mode==1?PriorityEffects.io:mode==2?PriorityEffects.runtime:mode==3?PriorityEffects.error:version<7?PriorityEffects.missing:Integer.valueOf(29);
  if(after){if(!(actual instanceof PriorityAfterFailure)||!((Throwable)actual).getMessage().equals("after"))throw new AssertionError("post-finally original throw");initialized=1;}
  else if(expected instanceof Number){if(!(actual instanceof Number)||((Number)actual).longValue()!=((Number)expected).longValue())throw new AssertionError("original return word");}
  else if(actual!=expected)throw new AssertionError("exception identity and cleanup precedence");
  if(PriorityEffects.allocations!=(after?1:0)||PriorityEffects.initializations!=initialized||(initialized!=0&&!PriorityEffects.initializationTrace.equals("BCF")))throw new AssertionError("continuation allocation/class initialization order");
  String trace=mode==0?"BF":"BCF";if(after)trace+="A";
  if(!PriorityEffects.trace.equals(trace))throw new AssertionError("exactly once cleanup "+PriorityEffects.trace);
  rows++;
 }System.out.println(rows+":finally:conditional:identity:return:cleanup:continuation:initialization");}
}
`

func TestAdversarialConditionalFinallyContinuationRoundTrip(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "reversed condition", "wide return", "multiple cleanup effects", "numeric conversion body", "joined local body", "internal switch body"} {
		t.Run(variant, func(t *testing.T) {
			fixture := conditionalFinallyContinuationFixture
			owner, driver := "PriorityReader", "PriorityDriver"
			switch variant {
			case "renamed":
				fixture = strings.ReplaceAll(fixture, "Priority", "OrderedExit")
				owner, driver = "OrderedExitReader", "OrderedExitDriver"
			case "reversed condition":
				fixture = strings.Replace(fixture, "if(version>=7)PriorityEffects.footer(prior,cleanup);else throw PriorityEffects.rethrow(prior,cleanup);", "if(version<7)throw PriorityEffects.rethrow(prior,cleanup);else PriorityEffects.footer(prior,cleanup);", 1)
			case "wide return":
				fixture = strings.Replace(fixture, "static int read(", "static long read(", 1)
				fixture = strings.Replace(fixture, "return 29;", "return -9223372036854775779L;", 1)
				fixture = strings.Replace(fixture, "Integer.valueOf(PriorityReader.read", "Long.valueOf(PriorityReader.read", 1)
				fixture = strings.Replace(fixture, "Integer.valueOf(29)", "Long.valueOf(Long.MIN_VALUE+29)", 1)
			case "numeric conversion body":
				fixture = strings.Replace(fixture, "static String trace=", "static long scratch;static String trace=", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.body(mode);return 29;", "PriorityEffects.body(mode);PriorityEffects.scratch=(long)(version*0x40000001);return 29;", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;", "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;PriorityEffects.scratch=Long.MIN_VALUE;", 1)
				fixture = strings.Replace(fixture, "boolean after=", "if(PriorityEffects.scratch!=(mode==0?(long)java.math.BigInteger.valueOf(version).multiply(java.math.BigInteger.valueOf(0x40000001L)).intValue():Long.MIN_VALUE))throw new AssertionError(\"signed conversion at original store\");boolean after=", 1)
			case "joined local body":
				fixture = strings.Replace(fixture, "static String trace=", "static long scratch;static String trace=", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.body(mode);return 29;", "PriorityEffects.body(mode);int word;if((version&1)==0){word=version*0x40000001;PriorityEffects.trace+=\"L\";}else{word=version+17;PriorityEffects.trace+=\"R\";}PriorityEffects.scratch=(long)word;return 29;", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;", "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;PriorityEffects.scratch=Long.MIN_VALUE;", 1)
				fixture = strings.Replace(fixture, "boolean after=", "if(PriorityEffects.scratch!=(mode==0?(long)((version&1)==0?java.math.BigInteger.valueOf(version).multiply(java.math.BigInteger.valueOf(0x40000001L)):java.math.BigInteger.valueOf(version).add(java.math.BigInteger.valueOf(17))).intValue():Long.MIN_VALUE))throw new AssertionError(\"joined original store\");boolean after=", 1)
				fixture = strings.Replace(fixture, "String trace=mode==0?\"BF\":\"BCF\";", "String trace=mode==0?((version&1)==0?\"BLF\":\"BRF\"):\"BCF\";", 1)
			case "internal switch body":
				fixture = strings.Replace(fixture, "static String trace=", "static long scratch;static int stores;static final Throwable switchFailure=new java.io.IOException(\"switch\");static void store(long value){stores++;scratch=value;}static String trace=", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.body(mode);return 29;", "PriorityEffects.body(mode);switch(version&3){case 0:PriorityEffects.trace+=\"S\";case 1:PriorityEffects.store((long)(version+41));break;case 2:PriorityEffects.trace+=\"T\";break;default:throw PriorityEffects.switchFailure;}return 29;", 1)
				fixture = strings.Replace(fixture, "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;", "PriorityEffects.trace=\"\";PriorityEffects.allocations=0;PriorityEffects.stores=0;PriorityEffects.scratch=Long.MIN_VALUE;", 1)
				fixture = strings.Replace(fixture, "boolean after=", "int marker=version&3;boolean switchFailure=mode==0&&marker==3;if(PriorityEffects.stores!=((mode==0&&marker<2)?1:0)||PriorityEffects.scratch!=((mode==0&&marker<2)?(long)java.math.BigInteger.valueOf(version).add(java.math.BigInteger.valueOf(41)).intValue():Long.MIN_VALUE))throw new AssertionError(\"switch selector, fall-through and store cardinality\");boolean after=", 1)
				fixture = strings.Replace(fixture, "cleanup==2&&mode!=0", "cleanup==2&&(mode!=0||switchFailure)", 1)
				fixture = strings.Replace(fixture, "?PriorityEffects.cleanup:mode==1?", "?PriorityEffects.cleanup:switchFailure?PriorityEffects.switchFailure:mode==1?", 1)
				fixture = strings.Replace(fixture, "String trace=mode==0?\"BF\":\"BCF\";", "String trace=(mode!=0||switchFailure)?\"BCF\":marker==0?\"BSF\":marker==2?\"BTF\":\"BF\";", 1)
			case "multiple cleanup effects":
				fixture = strings.Replace(fixture, "if(version>=7)PriorityEffects.footer(prior,cleanup);", "if(version>=7){PriorityEffects.beforeFooter();PriorityEffects.footer(prior,cleanup);}", 1)
				fixture = strings.Replace(fixture, "static void footer(", "static void beforeFooter(){trace+=\"P\";}static void footer(", 1)
				fixture = strings.NewReplacer("\"BF\"", "(version>=7?\"BPF\":\"BF\")", "\"BCF\"", "(version>=7?\"BCPF\":\"BCF\")").Replace(fixture)
			}
			want := "84:finally:conditional:identity:return:cleanup:continuation:initialization\n"
			t.Run("modern", func(t *testing.T) { testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, want) })
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{owner + ".java": fixture}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, driver, want, nil)
			})
		})
	}
}
