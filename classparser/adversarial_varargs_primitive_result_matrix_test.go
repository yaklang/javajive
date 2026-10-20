package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// An independent reachability model of JLS5.1.2 is checked against both actual
// compilers. Unlike the production JVM-tail grammar, this graph contains no
// bytecode opcodes or boxing declarations. The 37 forbidden pairs must fail at
// their own reference expressions, rather than because the fixture is broken.
func TestAdversarialVarargsPrimitiveResultMatrixMatchesBothOriginalCompilers(t *testing.T) {
	types := []struct{ name, samples string }{
		{"boolean", "false,true"},
		{"byte", "(byte)-128,(byte)-1,(byte)0,(byte)1,(byte)127"},
		{"short", "(short)-32768,(short)-1,(short)0,(short)1,(short)32767"},
		{"char", "(char)0,(char)1,(char)0xd800,(char)0xdfff,(char)0xffff"},
		{"int", "Integer.MIN_VALUE,-1,0,129,Integer.MAX_VALUE"},
		{"long", "Long.MIN_VALUE,-1L,0L,129L,Long.MAX_VALUE"},
		{"float", "0f,-0f,Float.intBitsToFloat(0x7fc00001),Float.POSITIVE_INFINITY,Float.MAX_VALUE"},
		{"double", "0d,-0d,Double.longBitsToDouble(0x7ff8000000000001L),Double.POSITIVE_INFINITY,Double.MAX_VALUE"},
	}
	edges := map[string][]string{"byte": {"short"}, "short": {"int"}, "char": {"int"}, "int": {"long"}, "long": {"float"}, "float": {"double"}}
	legal := func(from, to string) bool {
		seen := map[string]bool{}
		pending := []string{from}
		for len(pending) > 0 {
			n := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			if n == to {
				return true
			}
			pending = append(pending, edges[n]...)
		}
		return false
	}
	var target, positive, negative, driver strings.Builder
	target.WriteString("class PrimitiveResultTarget {\n")
	for _, x := range types {
		fmt.Fprintf(&target, "public %s read%s(%s... x){return x[0];}\n", x.name, x.name, x.name)
	}
	target.WriteString("}\n")
	driver.WriteString("class PrimitiveResultDriver {public static void main(String[]args){PrimitiveResultTarget target=new PrimitiveResultTarget();int rows=0;\n")
	var owners []string
	for _, from := range types {
		for _, to := range types {
			name := "PrimitiveResult" + from.name + "To" + to.name
			decl := fmt.Sprintf("interface %sPort {%s apply(%s value);}\nclass %s {static %sPort make(PrimitiveResultTarget target){return target::read%s;}}\n", name, to.name, from.name, name, name, from.name)
			if !legal(from.name, to.name) {
				negative.WriteString(decl)
				continue
			}
			positive.WriteString(decl)
			owners = append(owners, name)
			fmt.Fprintf(&driver, "for(%s input:new %s[]{%s}){%s expected=(%s)input;%s actual=%s.make(target).apply(input);", from.name, from.name, from.samples, to.name, to.name, to.name, name)
			switch to.name {
			case "float":
				driver.WriteString("if(Float.floatToRawIntBits(actual)!=Float.floatToRawIntBits(expected))throw new AssertionError(\"float result bits\");")
			case "double":
				driver.WriteString("if(Double.doubleToRawLongBits(actual)!=Double.doubleToRawLongBits(expected))throw new AssertionError(\"double result bits\");")
			default:
				driver.WriteString("if(actual!=expected)throw new AssertionError(\"primitive result value\");")
			}
			driver.WriteString("rows++;}\n")
		}
	}
	if len(owners) != 27 {
		t.Fatalf("independent language-domain model has %d legal pairs", len(owners))
	}
	driver.WriteString("if(rows!=132)throw new AssertionError(\"matrix coverage\");System.out.println(\"132:primitive:result:matrix:bits\");}}\n")
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			t.Run("all 37 illegal reference results", func(t *testing.T) {
				dir := t.TempDir()
				source := target.String() + negative.String()
				path := filepath.Join(dir, "PrimitiveRejected.java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-J-Duser.language=en", "-J-Duser.country=US", "-proc:none", "-g:none", "-d", dir}
				if profile == NativeJavac8 {
					args = append(args, "-source", "8", "-target", "8")
				} else {
					args = append(args, "--release", "8")
				}
				args = append(args, path)
				output, err := exec.Command(compiler, args...).CombinedOutput()
				if err == nil {
					t.Fatalf("compiler accepted forbidden method reference result pairs\n%s", output)
				}
				matches := regexp.MustCompile(`(?m)^.*PrimitiveRejected\.java:(\d+): error:`).FindAllStringSubmatch(string(output), -1)
				badLines := map[int]bool{}
				for _, m := range matches {
					n, e := strconv.Atoi(m[1])
					if e != nil {
						t.Fatal(e)
					}
					badLines[n] = true
				}
				lines := strings.Split(source, "\n")
				expected := map[int]bool{}
				for i, line := range lines {
					if strings.Contains(line, "return target::") {
						expected[i+1] = true
					}
				}
				if len(expected) != 37 || len(matches) != 37 || len(badLines) != 37 {
					t.Fatalf("invalid pair coverage matches=%d distinct=%d expected=%d\n%s", len(matches), len(badLines), len(expected), output)
				}
				for line := range expected {
					if !badLines[line] {
						t.Fatalf("compiler did not reject original pair on line%d\n%s", line, output)
					}
				}
			})
			testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, target.String()+positive.String()+driver.String(), "PrimitiveResultTarget", debug, true)
			}, profile, compiler, owners, "PrimitiveResultDriver", "132:primitive:result:matrix:bits\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
				if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
					t.Fatalf("primitive return changed original metadata%s\n%s\n%s", name, x, y)
				}
			})
		})
	}
}
