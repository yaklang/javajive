package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialIntegerDecisionDAGRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const stages = 20
	var owner, driver, want strings.Builder
	owner.WriteString(`public class DecisionLadderOwner {static long cMask,dMask;static int failAt;static String trace="";static final RuntimeException SAME=new RuntimeException("same");static boolean c(int i){trace+="C"+i+";";if(failAt==2*i)throw SAME;return ((cMask>>>i)&1L)!=0;}static boolean d(int i){trace+="D"+i+";";if(failAt==2*i+1)throw SAME;return ((dMask>>>i)&1L)!=0;}`)
	driver.WriteString(`public class DecisionLadderDriver {public static void main(String[]args){`)
	words := [][2]int{{1, 0}, {2, 3}, {-2, -1}, {7, 7}}
	masks := [][2]int64{{0, 0}, {(1 << stages) - 1, (1 << stages) - 1}, {(1 << stages) - 1, 0}, {0x55555, 0x33333}, {1 << 19, 0}, {1 << 19, 1 << 19}}
	for w, pair := range words {
		fmt.Fprintf(&owner, `static int value%d(){try{return (`, w)
		for i := 0; i < stages; i++ {
			if i > 0 {
				owner.WriteString(" && ")
			}
			fmt.Fprintf(&owner, "(!c(%d) || d(%d))", i, i)
		}
		fmt.Fprintf(&owner, `)?%d:%d;}catch(RuntimeException e){trace+="E;";throw e;}finally{trace+="F;";}}`, pair[0], pair[1])
		for _, mask := range masks {
			for _, fail := range []int{-1, 0, 1, 18, 19, 38, 39} {
				fmt.Fprintf(&driver, `DecisionLadderOwner.cMask=%dL;DecisionLadderOwner.dMask=%dL;DecisionLadderOwner.failAt=%d;DecisionLadderOwner.trace="";try{System.out.print(DecisionLadderOwner.value%d()+":");}catch(RuntimeException e){System.out.print((e==DecisionLadderOwner.SAME)+":");}System.out.println(DecisionLadderOwner.trace);`, mask[0], mask[1], fail, w)
				var trace strings.Builder
				truth, thrown := true, false
				for i := 0; i < stages; i++ {
					fmt.Fprintf(&trace, "C%d;", i)
					if fail == 2*i {
						thrown = true
						break
					}
					if mask[0]&(1<<i) != 0 {
						fmt.Fprintf(&trace, "D%d;", i)
						if fail == 2*i+1 {
							thrown = true
							break
						}
						if mask[1]&(1<<i) == 0 {
							truth = false
							break
						}
					}
				}
				if thrown {
					fmt.Fprint(&trace, "E;F;")
					fmt.Fprintf(&want, "true:%s\n", trace.String())
				} else {
					trace.WriteString("F;")
					value := pair[1]
					if truth {
						value = pair[0]
					}
					fmt.Fprintf(&want, "%d:%s\n", value, trace.String())
				}
			}
		}
	}
	owner.WriteString(`static int miniMask;static boolean mini(int i){trace+=new String[]{"C;","S;","A;"}[i];if(failAt==i)throw SAME;return (miniMask&(1<<i))!=0;}static boolean leadingOr(){try{return mini(0)?(mini(1)||mini(2)):mini(1);}catch(RuntimeException e){trace+="E;";throw e;}finally{trace+="F;";}}static boolean leadingAnd(){try{return mini(0)?mini(1):(mini(1)&&mini(2));}catch(RuntimeException e){trace+="E;";throw e;}finally{trace+="F;";}}`)
	for _, kind := range []string{"leadingOr", "leadingAnd"} {
		for mask := 0; mask < 8; mask++ {
			for fail := -1; fail < 3; fail++ {
				fmt.Fprintf(&driver, `DecisionLadderOwner.miniMask=%d;DecisionLadderOwner.failAt=%d;DecisionLadderOwner.trace="";try{System.out.print(DecisionLadderOwner.%s()+":");}catch(RuntimeException e){System.out.print((e==DecisionLadderOwner.SAME)+":");}System.out.println(DecisionLadderOwner.trace);`, mask, fail, kind)
				var trace strings.Builder
				thrown := false
				gate := func(i int) bool {
					trace.WriteString([]string{"C;", "S;", "A;"}[i])
					if fail == i {
						thrown = true
					}
					return mask&(1<<i) != 0
				}
				c, result := gate(0), false
				if !thrown {
					result = gate(1)
					if !thrown && ((kind == "leadingOr" && c && !result) || (kind == "leadingAnd" && !c && result)) {
						result = gate(2)
					}
				}
				if thrown {
					trace.WriteString("E;F;")
					fmt.Fprintf(&want, "true:%s\n", trace.String())
				} else {
					trace.WriteString("F;")
					fmt.Fprintf(&want, "%t:%s\n", result, trace.String())
				}
			}
		}
	}
	owner.WriteString("}")
	driver.WriteString("}}")
	source := strings.ReplaceAll(owner.String(), `\"`, `"`)
	driverSource := strings.ReplaceAll(driver.String(), `\"`, `"`)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			of := filepath.Join(original, "DecisionLadderOwner.java")
			df := filepath.Join(original, "DecisionLadderDriver.java")
			os.WriteFile(of, []byte(source), 0600)
			os.WriteFile(df, []byte(driverSource), 0600)
			if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, of, df).CombinedOutput(); e != nil {
				t.Fatalf("original%v %s", e, out)
			}
			if got := t04RunJava(t, java, original, "DecisionLadderDriver"); got != want.String() {
				t.Fatalf("independent original decision oracle mismatch got%q want%q", got, want.String())
			}
			raw, e := os.ReadFile(filepath.Join(original, "DecisionLadderOwner.class"))
			if e != nil {
				t.Fatal(e)
			}
			resolve := func(n string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(original, n+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var r DecompileResult
					var err error
					if mode == "legacy" {
						r.Source, err = DecompileWithResolver(raw, resolve)
					} else {
						r, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					}
					if err != nil || len(r.StubMethods) > 0 || strings.Contains(r.Source, "undecompilable method body") {
						t.Fatalf("decision graph refused %v %v", err, r.StubMethods)
					}
					if len(r.Source) > 50000 {
						t.Fatalf("linear original predicate expanded to%dsource bytes", len(r.Source))
					}
					rebuilt := t.TempDir()
					f := filepath.Join(rebuilt, "DecisionLadderOwner.java")
					os.WriteFile(f, []byte(r.Source), 0600)
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, f).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt%v %s\n%s", e, out, r.Source)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "DecisionLadderDriver"); got != want.String() {
						t.Fatalf("decision graph changed literal/effect/throw order got%q want%q\n%s", got, want.String(), r.Source)
					}
				})
			}
		})
	}
}
