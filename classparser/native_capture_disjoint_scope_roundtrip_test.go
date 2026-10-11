package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeCaptureDisjointScopeFixture = `class ScopeEffects{static String trace="";static Object read(Object token){trace+="V";return token;}static int side(int width){trace+="S";return width;}}
abstract class ScopeReader{final Object observed;ScopeReader(){ScopeEffects.trace+="P";observed=read();}abstract Object read();}
class ScopeOwner{ScopeReader make(boolean dense,int width,Object token){if(dense){final Object slice=ScopeEffects.read(token);switch(width){case 1:return new ScopeReader(){Object read(){return slice;}};case 2:return new ScopeReader(){Object read(){return slice;}};default:throw new IllegalArgumentException();}}else{int count=ScopeEffects.side(width);if(count==0)return null;else{final Object slice=ScopeEffects.read(token);switch(width){case 1:return new ScopeReader(){Object read(){return slice;}};case 2:return new ScopeReader(){Object read(){return slice;}};default:throw new IllegalArgumentException();}}}}}
class ScopeDriver{public static void main(String[]args){int count=0;ScopeOwner owner=new ScopeOwner();for(boolean dense:new boolean[]{false,true})for(int width:new int[]{0,1,2,3})for(Object token:new Object[]{null,new Object(),new String("identity")}){ScopeEffects.trace="";try{ScopeReader reader=owner.make(dense,width,token);if(!dense&&width==0){if(reader!=null||!ScopeEffects.trace.equals("S"))throw new AssertionError("empty path");}else{if(width!=1&&width!=2||reader==null||reader.read()!=token||reader.observed!=token||!reader.getClass().isAnonymousClass()||!ScopeEffects.trace.equals(dense?"VP":"SVP"))throw new AssertionError("scope/capture/effect/early callback");}}catch(IllegalArgumentException e){if(width==1||width==2||!dense&&width==0||!ScopeEffects.trace.equals(dense?"V":"SV"))throw new AssertionError("bad branch/effects",e);}count++;}System.out.println(count+":disjoint:lexical:captures:identity");}}`

func TestNativeCaptureDisjointLexicalNamesKeepDistinctBindings(t *testing.T) {
	for _, root := range []string{"ScopeOwner", "DifferentBranchScope"} {
		t.Run(root, func(t *testing.T) {
			fixture := strings.ReplaceAll(nativeCaptureDisjointScopeFixture, "ScopeOwner", root)
			testNativeIndependentFamilyFixture(t, fixture, []string{root}, "ScopeDriver", "24:disjoint:lexical:captures:identity\n", nativeLexicalExactSignatures)
		})
	}
}

func TestNativeCaptureDisjointNamesNativeCompiler(t *testing.T) {
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy == "" {
		t.Skip("JAVA8_JAVAC is required for the independent original compiler")
	}
	version, err := exec.Command(legacy, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original javac8 identity", err, string(version))
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		dir := t.TempDir()
		source := filepath.Join(dir, "ScopeOwner.java")
		if err := os.WriteFile(source, []byte(nativeCaptureDisjointScopeFixture), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(legacy, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, source).CombinedOutput(); err != nil {
			t.Fatal("original compile", err, string(out))
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".class") {
				raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = raw
			}
		}
		return files
	}, NativeJavac8, legacy, []string{"ScopeOwner"}, "ScopeDriver", "24:disjoint:lexical:captures:identity\n", nil, nativeLexicalExactSignatures)
}
