package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialReassignedParameterAccumulatorRetainsEveryWrite(t *testing.T) {
	const fixture = `
import java.util.*;
public class CollectorOwner {
 static Map<String,Object> relay(Map<String,Object> seed){return seed;}
 public static Map<String,Object> collect(Map<String,Object> seed,String[] names,Object[] values){
  seed=relay(seed);
  for(int i=0;i<names.length;i++){if(names[i]==null)continue;if(seed==null)seed=new LinkedHashMap<String,Object>();Object value=values[i];seed.put(names[i],value);}
  return seed;
 }
}
class CollectorDriver {public static void main(String[] args){int rows=0;Object a=new Object(),b=new Object(),c=new Object();
 String[][] lists={{},{"a"},{"a","b","c"},{"a","a","b"},{null,"b",null}};
 for(String[]names:lists)for(boolean seeded:new boolean[]{false,true}){Map<String,Object> seed=seeded?new LinkedHashMap<String,Object>():null;Map<String,Object> expected=seeded?new LinkedHashMap<String,Object>():null;if(seeded){seed.put("old",a);expected.put("old",a);}Object[] values={a,b,c};for(int i=0;i<names.length;i++)if(names[i]!=null){if(expected==null)expected=new LinkedHashMap<String,Object>();expected.put(names[i],values[i]);}
  Map<String,Object> got=CollectorOwner.collect(seed,names,values);if(!Objects.equals(expected,got)||(seeded&&got!=seed))throw new AssertionError("accumulator "+rows+":"+got+" expected "+expected);if(got!=null&&!new ArrayList<String>(got.keySet()).equals(new ArrayList<String>(expected.keySet())))throw new AssertionError("order");rows++;
 }
 Map<String,Object> partial=new LinkedHashMap<String,Object>();Throwable failure=null;try{CollectorOwner.collect(partial,new String[]{"a","b","c"},new Object[]{a,b});}catch(Throwable t){failure=t;}if(!(failure instanceof ArrayIndexOutOfBoundsException)||partial.size()!=2||partial.get("a")!=a||partial.get("b")!=b)throw new AssertionError("partial effects",failure);rows++;
 System.out.println(rows+":reassigned:parameter:accumulator:identity");}}
`

	for _, style := range []string{"static", "instance-wide-prefix"} {
		t.Run(style, func(t *testing.T) {
			text := fixture
			if style == "instance-wide-prefix" {
				text = strings.ReplaceAll(text, "public static Map<String,Object> collect(", "public Map<String,Object> collect(long prefix,")
				text = strings.ReplaceAll(text, "CollectorOwner.collect(", "new CollectorOwner().collect(37L,")
			}
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
						return compileLambdaParameterMetadataFixture(t, compiler, profile, text, "CollectorOwner", debug, false)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"CollectorOwner"}, "CollectorDriver", "11:reassigned:parameter:accumulator:identity\n", nil, nil, []string{"normal", "no-source-rewrites"})
				})
			}

		})
	}
}
