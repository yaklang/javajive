package cross

import (
	"github.com/yaklang/javajive"
	"testing"
)

func TestAuditHistoricalReferenceDeclarations(t *testing.T) {
	cases := []struct{ name, body, driver string }{
		{"typed_catches_finally_switch", `public static String f(String cmd){StringBuilder b=new StringBuilder();try{switch(cmd){case "add":b.append(3);break;case "div":b.append(10/Integer.parseInt("0"));break;case "len":b.append(cmd.length());break;default:b.append('?');}}catch(ArithmeticException e){b.append("AE");}catch(RuntimeException e){b.append("RE");}finally{b.append("|fin");}return b.toString();}`, `for(String s:new String[]{"add","div","len","x",null})System.out.println(Fixture.f(s));`},
		{"same_type_array_locals", `int[] a;public Fixture(int[] v){a=v;}public int[] f(Fixture other){int[] left;int n=(left=this.a).length;int[] right=other.a;int m=right.length;int[] out=new int[n+m-1];for(int i=0;i<n;i++){int x=left[i];for(int j=0;j<m;j++)out[i+j]+=x*right[j];}return out;}`, `System.out.println(java.util.Arrays.toString(new Fixture(new int[]{1,2,3}).f(new Fixture(new int[]{4,5}))));`},
		{"constructor_embedded_assignment", `int total;public Fixture(int[] a){this(a.length);for(int i=0;i<a.length;i++){int x;if((x=a[i])>0)total+=x;}}Fixture(int n){total=n;}`, `System.out.println(new Fixture(new int[]{1,-1,3}).total);System.out.println(new Fixture(new int[]{}).total);`},
		{"empty_catch_continuation", `public static int f(String s){try{return Integer.parseInt(s);}catch(NumberFormatException e){}if(s.equals("fallback"))return 42;throw new IllegalArgumentException(s);}`, `for(String s:new String[]{"7","fallback","bad"}){try{System.out.println(Fixture.f(s));}catch(IllegalArgumentException e){System.out.println(e.getClass().getName()+":"+e.getMessage());}}`},
		{"growable_array_handler", `public static String f(int[] a,int from,int length){char[] out=new char[length];int used=0;for(int i=from;i<from+length;i++){while(true){try{out[used]=(char)a[i];out[used+1]='!';used+=2;break;}catch(ArrayIndexOutOfBoundsException e){out=java.util.Arrays.copyOf(out,out.length*2+1);}}}return new String(out,0,used);}`, `System.out.println(Fixture.f(new int[]{65,66,67},0,3));System.out.println(Fixture.f(new int[]{65,66,67},1,1));System.out.println(Fixture.f(new int[0],0,0));`},
		{"reference_assignment_in_condition", `public static int f(Object[][] a,String s){int total=0;for(int i=0;i<a.length;i++){Object[] row;if((row=a[i])[0].equals(s))total+=((Integer)row[1]).intValue();}return total;}`, `Object[][] a={{"a",1},{"b",2},{"a",4}};System.out.println(Fixture.f(a,"a"));System.out.println(Fixture.f(a,"b"));System.out.println(Fixture.f(a,"c"));`},
		{"receiver_assignment_in_condition", `public static String f(StringBuilder[] a){String out="";for(int i=0;i<a.length;i++){StringBuilder row;if((row=a[i]).length()==0)out+="empty;";else out+=row.toString()+":"+row.charAt(0)+";";}return out;}`, `System.out.println(Fixture.f(new StringBuilder[]{new StringBuilder(),new StringBuilder("ab"),new StringBuilder("z")}));`},
		{"nested_switch_effectful_fallthrough", `public static int f(int a,int b,int c){switch(a){case 1:switch(b){case 2:c++;case 3:return c;default:if(c>0)return 8;else return 9;}case 4:return 10;default:break;}return 11;}`, `for(int a:new int[]{0,1,4})for(int b:new int[]{1,2,3})for(int c:new int[]{-1,1})System.out.println(Fixture.f(a,b,c));`},
		{"nested_switch_returns", `public static int f(int a,int b,int c){switch(a){case 1:switch(b){case 2:case 3:return 7;default:if(c>0)return 8;else return 9;}case 4:return 10;default:break;}return 11;}`, `for(int a:new int[]{0,1,4})for(int b:new int[]{1,2,3})for(int c:new int[]{-1,1})System.out.println(Fixture.f(a,b,c));`},
		{"reflection_hoisted_array", `public static String f(boolean b){Class<?>[] p=new Class<?>[]{b?String.class:Integer.class};try{return String.class.getConstructor(p).getName();}catch(NoSuchMethodException e){return e.getClass().getName();}}`, `System.out.println(Fixture.f(true));System.out.println(Fixture.f(false));`},

		{"scanner_switch_loop_exit", `static char[] buffer;static int pos,limit;static boolean fill(int n){return false;}static void touch(){}public static String f(String s){buffer=s.toCharArray();pos=0;limit=buffer.length;StringBuilder b=null;int i=0;scan:while(true){while(pos+i<limit){switch(buffer[pos+i]){case '#':case '/':touch();case ' ':case ';':case ',':break scan;default:i++;}}if(i<buffer.length){if(fill(i+1))continue;break;}if(b==null)b=new StringBuilder();b.append(buffer,pos,i);pos+=i;i=0;if(!fill(1))break;}String result=b==null?new String(buffer,pos,i):b.append(buffer,pos,i).toString();pos+=i;return result;}`, `for(String s:new String[]{"","abc","ab cd",";","xy;z","abc#rest","/"})System.out.println(Fixture.f(s));`},
		{"constructor_primitive_cast_array", `long value;public Fixture(long n){this(new int[]{(int)(n>>>32),(int)(n&0xffffffffL)});}Fixture(int[] a){value=((long)a[0]<<32)|(a[1]&0xffffffffL);}`, `for(long n:new long[]{0,1,-1,0x12345678abcdef12L})System.out.println(new Fixture(n).value);`},
		{"parameter_null_fallback", `public static String f(CharSequence x){if(x==null)x="null";return x.length()+":"+(x instanceof StringBuilder)+":"+x;}`, `System.out.println(Fixture.f(null));System.out.println(Fixture.f("ok"));System.out.println(Fixture.f(new StringBuilder("abc")));`},
		{"generic_reassignment", `@SuppressWarnings("unchecked") public static <T> T f(T x,boolean b){T y=x;if(b)y=(T)copy(x);return y;}static Object copy(Object x){return x;}`, `System.out.println(Fixture.f("first",false));System.out.println(Fixture.f("second",true));`},
		{"constructor_lambda_array", `static String log="";static int next(int n){log+=n;return n;}public Fixture(){this(next(1),new java.util.function.IntSupplier[]{()->next(2),()->next(3)});}Fixture(int x,java.util.function.IntSupplier[] a){log+=":";for(java.util.function.IntSupplier s:a)s.getAsInt();}`, `new Fixture();System.out.println(Fixture.log);`},
		{"list_reassignment", `public static int f(boolean b){java.util.List<String> x=new java.util.ArrayList<String>();x.add("first");if(b)x=java.util.Arrays.asList("second","third");return x.size()*10+x.get(0).length();}`, `System.out.println(Fixture.f(false));System.out.println(Fixture.f(true));`},
		{"class_to_type", `public static String f(boolean b)throws Exception{java.lang.reflect.Type x=String.class;if(b)x=Fixture.class.getDeclaredField("items").getGenericType();return x.getTypeName();}public java.util.List<String> items;`, `System.out.println(Fixture.f(false));System.out.println(Fixture.f(true));`},
		{"stream_reassignment", `public static int f(boolean b)throws Exception{java.io.InputStream x=new java.io.ByteArrayInputStream(new byte[]{7,8});if(b)x=new java.io.BufferedInputStream(x);return x.read()*10+x.read();}`, `System.out.println(Fixture.f(false));System.out.println(Fixture.f(true));`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
					t.Run(debug+"/"+string(mode), func(t *testing.T) {
						if tc.name == "list_reassignment" {
							// JDK 8's bounded invocation catalog omits List, so the
							// correct API result is explicit unsupported for add(Object).
							// Keep the stronger source compile, verifier, and runtime
							// comparison oracles for this case.
							auditRoundTripWithExpectedUnsupported(t, "audit", tc.body, tc.driver, debug, mode,
								"invoke java.util.List.add(Ljava/lang/Object;)Z")
							return
						}
						auditRoundTrip(t, "audit", tc.body, tc.driver, debug, mode)
					})
				}
			}
		})
	}
}

func TestAuditHistoricalTypeGraphs(t *testing.T) {
	for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			t.Run("sequential_resource_exception_identity", func(t *testing.T) {
				auditSourceSet(t, map[string]string{
					"Resource.java": `import java.io.*;public class Resource implements Closeable {String name;boolean fail;public Resource(String n,boolean f){name=n;fail=f;}public void close()throws IOException{Fixture.log+="close:"+name+";";if(fail)throw new IOException(name);}}`,
					"Fixture.java":  `import java.io.*;public class Fixture {static String log="";static void work(int n, int stage)throws IOException{log+="work:"+stage+";";if(n==stage)throw new IOException("work"+stage);}public static void f(int n)throws IOException{try(Resource outer=new Resource("outer",n==4)){try(Resource first=new Resource("first",n==1)){work(n,1);}try(Resource second=new Resource("second",n==2)){work(n,2);}work(n,3);}}}`,
				}, `public class Driver {public static void main(String[] x){for(int n=0;n<5;n++){Fixture.log="";try{Fixture.f(n);System.out.println("ok");}catch(Exception e){System.out.println(e.getClass().getName()+":"+e.getMessage());for(Throwable t:e.getSuppressed())System.out.println("suppressed:"+t.getMessage());}System.out.println(Fixture.log);}}}`, mode)
			})
			t.Run("shared_interface_and_copy", func(t *testing.T) {
				auditSourceSet(t, map[string]string{
					"Common.java":  "public interface Common {int id();}",
					"Left.java":    "public interface Left extends Common {}",
					"Right.java":   "public interface Right extends Common {}",
					"One.java":     "public class One implements Left,java.io.Serializable {public int id(){return 1;}}",
					"Two.java":     "public class Two implements Right,java.io.Serializable {public int id(){return 2;}}",
					"Fixture.java": "public class Fixture {static Left left(){return new One();}static Right right(){return new Two();}public static int f(int n){Common value=n>0?left():right();Common cached=value;if(n==2)value=right();return cached.id()*10+value.id();}}",
				}, "public class Driver {public static void main(String[] x){for(int i=0;i<3;i++)System.out.println(Fixture.f(i));}}", mode)
			})
			t.Run("inaccessible_common_class", func(t *testing.T) {
				auditSourceSetWithLedgerExpectations(t, map[string]string{
					"api/Common.java":  "package api;public interface Common {int id();}",
					"impl/Hidden.java": "package impl;class Hidden implements api.Common {public int id(){return 1;}}",
					"impl/One.java":    "package impl;public class One extends Hidden {}",
					"impl/Two.java":    "package impl;public class Two extends Hidden {public int id(){return 2;}}",
					"Fixture.java":     "public class Fixture {public static int f(boolean b){api.Common value=new impl.One();if(b)value=new impl.Two();return value.id();}}",
				}, "public class Driver {public static void main(String[] x){System.out.println(Fixture.f(false));System.out.println(Fixture.f(true));}}", mode,
					[]memberLedgerExpectation{{owner: "impl/One", name: "id", descriptor: "()I", state: "regenerated", evidence: "inherited bridge"}})
			})
			t.Run("overloaded_generic_witness", func(t *testing.T) {
				auditSourceSet(t, map[string]string{
					"Box.java":     "public class Box<Q> {public Q value;public Box(Q q){value=q;}}",
					"Helper.java":  "public class Helper {public static <Y> Y pick(String s,Y y,Box<Y> b){return s.length()>0?y:b.value;}public static <Y> Y pick(Integer s,Y y,Box<Y> b){return s>0?y:b.value;}}",
					"Fixture.java": "public class Fixture {@SuppressWarnings(\"unchecked\") public static <T> T f(T input,Box<T> b){Object value=input;value=Helper.pick(\"\",(T)value,b);return (T)value;}}",
				}, "public class Driver {public static void main(String[] x){System.out.println(Fixture.f(\"first\",new Box<String>(\"second\")));}}", mode)
			})
		})
	}
}
