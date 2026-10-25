package javaclassparser

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestAdversarialSwitchOuterContinueRetainsInnerCommit(t *testing.T) {
	runOuterContinueFixture(t, outerContinueFixture, 11)
}

func runOuterContinueFixture(t *testing.T, fixture string, checks int) {
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
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "OuterWalker", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"OuterWalker"}, "OuterDriver", strconv.Itoa(checks)+":nested:outer-continue:inner-commit:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

const outerContinueFixture = `
class OuterStream {
 static final RuntimeException FAILURE=new IllegalStateException("same");boolean failCommit,failEnter;int[] tags;int index,calls,commits;boolean object;String trace="";OuterStream(int[] tags,boolean object){this.tags=tags;this.object=object;}
 int key(){if(++calls>100)throw new AssertionError("loop");return index<tags.length?index+1:0;}
 int tag(){if(++calls>100)throw new AssertionError("loop");return index<tags.length?tags[index++]:4;}
 void enter(int kind){trace+="E"+kind;if(failEnter)throw FAILURE;object=false;}
 void put(int key,Object value){commits++;trace+="P"+key+":"+value+";";if(failCommit)throw FAILURE;}
 void add(Object value){commits++;trace+="A"+value+";";if(failCommit)throw FAILURE;}
}
public class OuterWalker {
 public static String walk(OuterStream stream){ROOT:while(true){
 if(stream.object){int key=stream.key();while(key!=0){Object value;switch(stream.tag()){
 case 1:stream.enter(1);continue ROOT;
 case 3:stream.enter(3);continue ROOT;
 case 6:value="s";break;
 case 7:value=Integer.valueOf(17);break;
 case 11:value=null;break;
 default:throw new IllegalArgumentException();
 }stream.put(key,value);key=stream.key();}return stream.trace;
 }else{while(true){Object value;switch(stream.tag()){
 case 1:stream.enter(1);continue ROOT;
 case 3:stream.enter(3);continue ROOT;
 case 4:return stream.trace;
 case 6:value="s";break;
 case 7:value=Integer.valueOf(17);break;
 case 11:value=null;break;
 default:throw new IllegalArgumentException();
 }stream.add(value);}}}}
}
class OuterDriver{public static void main(String[] args){int rows=0;
 int[][] tags={ {},{6},{7,11,6},{1,6,7,4},{3,11,4},{6,1,7,4},{7,3,6,4}};
 String[] expect={"","P1:s;","P1:17;P2:null;P3:s;","E1As;A17;","E3Anull;","P1:s;E1A17;","P1:17;E3As;"};
 int[] commits={0,1,3,2,1,2,2};
 for(int i=0;i<tags.length;i++){OuterStream s=new OuterStream(tags[i],true);String got=OuterWalker.walk(s);if(!got.equals(expect[i])||s.commits!=commits[i]||s.index!=tags[i].length)throw new AssertionError(i+":"+got+":"+s.commits);rows++;}
 OuterStream s=new OuterStream(new int[]{6,7,11,4},false);if(!OuterWalker.walk(s).equals("As;A17;Anull;")||s.commits!=3)throw new AssertionError("array");rows++;
 for(boolean object:new boolean[]{true,false}){s=new OuterStream(new int[]{6,7,4},object);s.failCommit=true;try{OuterWalker.walk(s);throw new AssertionError("missing commit failure");}catch(RuntimeException e){if(e!=OuterStream.FAILURE||s.index!=1||s.commits!=1||!s.trace.equals(object?"P1:s;":"As;"))throw new AssertionError("commit identity/order",e);}rows++;}
 s=new OuterStream(new int[]{1,4},true);s.failEnter=true;try{OuterWalker.walk(s);throw new AssertionError("missing enter failure");}catch(RuntimeException e){if(e!=OuterStream.FAILURE||s.index!=1||s.commits!=0||!s.trace.equals("E1"))throw new AssertionError("enter identity/order",e);}rows++;
 System.out.println(rows+":nested:outer-continue:inner-commit:effects");}}
`

func TestAdversarialSparseSwitchOuterContinueRetainsInnerCommit(t *testing.T) {
	fixture := strings.NewReplacer("case 1:", "case 101:", "case 3:", "case 303:", "return index<tags.length?tags[index++]:4;", "int tag=index<tags.length?tags[index++]:4;return tag==1?101:tag==3?303:tag;").Replace(outerContinueFixture)
	runOuterContinueFixture(t, fixture, 11)
}

func innerOuterContinueFixture() string {
	fixture := strings.NewReplacer(
		"int key=stream.key();while(key!=0)", "for(int key=stream.key();key!=0;key=stream.key())",
		"}stream.put(key,value);key=stream.key();", "}stream.put(key,value);",
		"case 1:stream.enter(1);continue ROOT;", "case 1:stream.enter(1);continue;",
		"{1,6,7,4}", "{1,6,7}", "{6,1,7,4}", "{6,1,7}",
		"E1As;A17;", "E1P2:s;P3:17;", "P1:s;E1A17;", "P1:s;E1P3:17;",
		"boolean failCommit,failEnter;", "boolean failCommit,failEnter;int failKeyAt=-1;",
		"int key(){if(++calls", "int key(){if(index==failKeyAt)throw FAILURE;if(++calls",
		" System.out.println(rows+", ` for(int first:new int[]{1,6}){s=new OuterStream(new int[]{first,7},true);s.failKeyAt=1;try{OuterWalker.walk(s);throw new AssertionError("missing step failure");}catch(RuntimeException e){if(e!=OuterStream.FAILURE||s.index!=1||s.commits!=(first==1?0:1)||!s.trace.equals(first==1?"E1":"P1:s;"))throw new AssertionError("step identity/order",e);}rows++;}
 System.out.println(rows+`).Replace(outerContinueFixture)
	return fixture
}

func TestAdversarialSwitchInnerAndOuterContinueRetainsCommit(t *testing.T) {
	runOuterContinueFixture(t, innerOuterContinueFixture(), 13)
}

func TestAdversarialProtectedSwitchInnerAndOuterContinueRetainsHandler(t *testing.T) {
	fixture := strings.NewReplacer(
		"walk(OuterStream stream){ROOT:", "walk(OuterStream stream){try{ROOT:",
		"}stream.add(value);}}}}", "}stream.add(value);}}}}catch(RuntimeException failure){stream.trace+=\"C\";throw failure;}}",
		"object?\"P1:s;\":\"As;\"", "object?\"P1:s;C\":\"As;C\"",
		"s.trace.equals(\"E1\")", "s.trace.equals(\"E1C\")",
		"first==1?\"E1\":\"P1:s;\"", "first==1?\"E1C\":\"P1:s;C\"").Replace(innerOuterContinueFixture())
	runOuterContinueFixture(t, fixture, 13)
}
