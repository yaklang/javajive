package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An enum selector and an error-message read share one private symbol;
// serialization invokes another. The unchanged driver checks partial input
// consumption, nullable selectors, and exact observed failure identities.
const privateEnumSerializationFixture = `class PacketState{static String trace="";static final RuntimeException failure=new RuntimeException("original");}
class PacketInput{final String name;final PacketOwner.Kind kind;final int word;final int fail;int step;
 PacketInput(String name,PacketOwner.Kind kind,int word,int fail){this.name=name;this.kind=kind;this.word=word;this.fail=fail;}
 void event(String s){PacketState.trace+=s;if(++step==fail)throw PacketState.failure;}
 String name(){event("N");return name;}PacketOwner.Kind kind(){event("K");return kind;}int word(){event("I");return word;}}
class PacketOutput{int word;void put(int value){PacketState.trace+="W";word=value;}}
class PacketOwner{
 enum Kind{TEXT,NUMBER,OTHER}
 private Kind kind;private String name;private Object missing;
 static final Object FIRST=new Object(){public String toString(){return "FIRST";}};
 static final Object LAST=new Object(){public String toString(){return "LAST";}};
 PacketOwner(String name,Kind kind){this.name=name;this.kind=kind;}
 void setMissing(Object value){missing=value;}Object missing(){return missing;}String name(){return name;}
 private void serialize(PacketOutput output){if(kind==Kind.TEXT)output.put(missing==FIRST?1:0);else if(kind==Kind.NUMBER)output.put(((Integer)missing).intValue());else throw new IllegalArgumentException("unsupported="+kind);}
 static class Provider{
  PacketOwner read(PacketInput input){PacketOwner packet=new PacketOwner(input.name(),input.kind());
   switch(packet.kind){case TEXT:int word=input.word();if(word==1)packet.setMissing(FIRST);else packet.setMissing(LAST);break;case NUMBER:packet.setMissing(Integer.valueOf(input.word()));break;default:throw new IllegalArgumentException("unsupported="+packet.kind);}
   return packet;}
  void write(PacketOwner packet,PacketOutput output){packet.serialize(output);}
 }
}
class PacketDriver{public static void main(String[]args){int rows=0;
 for(PacketOwner.Kind kind:new PacketOwner.Kind[]{PacketOwner.Kind.TEXT,PacketOwner.Kind.NUMBER,PacketOwner.Kind.OTHER,null})for(int word:new int[]{Integer.MIN_VALUE,-1,0,1,2,3,0x13579bdf,Integer.MAX_VALUE})for(int fail=0;fail<=3;fail++){
  PacketState.trace="";PacketOwner.Provider provider=new PacketOwner.Provider();PacketInput input=new PacketInput("identity",kind,word,fail);
  boolean valid=kind==PacketOwner.Kind.TEXT||kind==PacketOwner.Kind.NUMBER;
  try{PacketOwner packet=provider.read(input);if(!valid||fail!=0)throw new AssertionError("missing failure");
   if(packet.name()!=input.name||!PacketState.trace.equals("NKI")||input.step!=3)throw new AssertionError("original read order");
   if(kind==PacketOwner.Kind.TEXT?(packet.missing()!=(word==1?PacketOwner.FIRST:PacketOwner.LAST)):((Integer)packet.missing()).intValue()!=word)throw new AssertionError("selector value and alias");
   PacketOutput output=new PacketOutput();provider.write(packet,output);if(output.word!=(kind==PacketOwner.Kind.TEXT?(word==1?1:0):word)||!PacketState.trace.equals("NKIW"))throw new AssertionError("private invocation and store");
  }catch(RuntimeException ex){
   if(fail==1||fail==2||valid&&fail==3){if(ex!=PacketState.failure||input.step!=fail||!PacketState.trace.equals(fail==1?"N":fail==2?"NK":"NKI"))throw new AssertionError("exception identity/partial read");}
   else if(kind==null){if(!(ex instanceof NullPointerException)||input.step!=2||!PacketState.trace.equals("NK"))throw new AssertionError("null selector");}
   else if(kind==PacketOwner.Kind.OTHER){if(!(ex instanceof IllegalArgumentException)||!ex.getMessage().equals("unsupported=OTHER")||input.step!=2||!PacketState.trace.equals("NK"))throw new AssertionError("shared selector default");}
   else throw new AssertionError("unexpected failure",ex);
  }rows++;
 }System.out.println(rows+":enum:private:selector:call:effects");}}
`

func TestAdversarialPrivateEnumSerializationKeepsSharedSelectorAndCallBinding(t *testing.T) {

	for _, renamed := range []bool{false, true} {
		name, owner, f := "original names", "PacketOwner", privateEnumSerializationFixture
		if renamed {
			name, owner = "independent names", "SeparatePacketScope"
			f = strings.ReplaceAll(f, "PacketOwner", owner)
		}
		t.Run(name, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, f, []string{owner}, "PacketDriver", "128:enum:private:selector:call:effects\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialPrivateEnumSerializationNativeCompilerProtocol(t *testing.T) {
	testPrivateEnumNative8Fixture(t, privateEnumSerializationFixture, "PacketOwner", "PacketDriver")
}

func testPrivateEnumNative8Fixture(t *testing.T, fixture, owner, driver string) {
	t.Helper()
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		root := t.TempDir()
		path := filepath.Join(root, "PacketOwner.java")
		if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
			t.Fatal(err)
		}
		if raw, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
			t.Fatal("authored original compile", err, string(raw))
		}
		files := map[string][]byte{}
		if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".class") {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[filepath.ToSlash(rel)] = raw
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}, NativeJavac8, javac, []string{owner}, driver, "128:enum:private:selector:call:effects\n", nil, nativeLexicalExactSignatures)
}

// Both source scopes use the same enum protocol. The owner switch reads a
// private field directly; the provider reaches it through javac's accessor.
func TestAdversarialPrivateEnumSerializationSwitchesInOwnerAndMember(t *testing.T) {
	fixture := privateEnumOwnerSwitchFixture(t)
	testNativeIndependentFamilyFixture(t, fixture, []string{"PacketOwner"}, "PacketDriver", "128:enum:private:selector:call:effects\n", nativeLexicalExactSignatures)
}

func privateEnumOwnerSwitchFixture(t *testing.T) string {
	t.Helper()
	fixture := strings.Replace(privateEnumSerializationFixture,
		`private void serialize(PacketOutput output){if(kind==Kind.TEXT)output.put(missing==FIRST?1:0);else if(kind==Kind.NUMBER)output.put(((Integer)missing).intValue());else throw new IllegalArgumentException("unsupported="+kind);}`,
		`private void serialize(PacketOutput output){switch(kind){case TEXT:output.put(missing==FIRST?1:0);break;case NUMBER:output.put(((Integer)missing).intValue());break;default:throw new IllegalArgumentException("unsupported="+kind);}}`, 1)
	if fixture == privateEnumSerializationFixture {
		t.Fatal("missing independent owner switch fixture")
	}
	return fixture
}

func TestAdversarialPackagedEnumSelectorKeepsOriginalFieldNamespace(t *testing.T) {
	for _, pkg := range []string{"packet", "independent.scope"} {
		for _, static := range []bool{false, true} {
			name := pkg + "/instance"
			fixture := privateEnumOwnerSwitchFixture(t)
			if static {
				name = pkg + "/static"
				fixture = privateEnumStaticSwitchFixture(t)
			}
			t.Run(name, func(t *testing.T) {
				prefix := strings.ReplaceAll(pkg, ".", "/") + "/"
				testNativeIndependentFamilyFixture(t, "package "+pkg+";\n"+fixture, []string{prefix + "PacketOwner"}, pkg+".PacketDriver", "128:enum:private:selector:call:effects\n", nativeLexicalExactSignatures)
			})
		}
	}
}

func privateEnumStaticSwitchFixture(t *testing.T) string {
	t.Helper()
	// GETSTATIC must retain its declaring type and initialize it exactly once.
	fixture := strings.Replace(privateEnumSerializationFixture, "private Kind kind;", "private static final Kind shared=initial();static Kind initial(){PacketState.trace+=\"Z\";return Kind.NUMBER;}static int staticWord(){switch(shared){case NUMBER:return 37;case TEXT:return -5;default:return 19;}}private Kind kind;", 1)
	fixture = strings.Replace(fixture, "int rows=0;", "int rows=0;if(PacketOwner.staticWord()!=37||!PacketState.trace.equals(\"Z\")||PacketOwner.staticWord()!=37||!PacketState.trace.equals(\"Z\"))throw new AssertionError(\"static enum field binding/init order\");", 1)
	if fixture == privateEnumSerializationFixture {
		t.Fatal("missing independent static selector fixture")
	}
	return fixture
}

func TestAdversarialPackagedEnumSelectorNativeCompilerProtocol(t *testing.T) {
	for _, pkg := range []string{"packet", "independent.scope"} {
		for _, static := range []bool{false, true} {
			name := pkg + "/instance"
			fixture := privateEnumOwnerSwitchFixture(t)
			if static {
				name = pkg + "/static"
				fixture = privateEnumStaticSwitchFixture(t)
			}
			t.Run(name, func(t *testing.T) {
				testPrivateEnumNative8Fixture(t, "package "+pkg+";\n"+fixture, strings.ReplaceAll(pkg, ".", "/")+"/PacketOwner", pkg+".PacketDriver")
			})
		}
	}
}
