package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// This is a source contract for the metadata-pinned original three-predicate
// word producer, not a permissive type/name regex. Entire expression equality
// after removing only grouping/whitespace rejects extra calls and reordering.
func reviewedThreePredicateWord(body, receiver, argument string) (string, bool) {
	compact := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == '(' || r == ')' {
				return -1
			}
			return r
		}, s)
	}
	expected := receiver + ".isSingleton&&this.allowCircularReferences&&this.isSingletonCurrentlyInCreation" + argument + "?1:0"
	for _, m := range regexp.MustCompile(`int\s+(\w+)\s*=\s*([^;]+);`).FindAllStringSubmatch(body, -1) {
		if compact(m[2]) == expected {
			return m[1], true
		}
	}
	return "", false
}

func TestReviewedThreePredicateWordRejectsDuplicateOrReorderedInputs(t *testing.T) {
	good := `int saved=((bean.isSingleton()) && ((this.allowCircularReferences) && (this.isSingletonCurrentlyInCreation(name)))) ? (1) : (0);`
	if _, ok := reviewedThreePredicateWord(good, "bean", "name"); !ok {
		t.Fatal("exact metadata-bound word refused")
	}
	for _, bad := range []string{
		`int saved=(this.allowCircularReferences && bean.isSingleton() && this.isSingletonCurrentlyInCreation(name)) ? 1:0;`,
		`int saved=(bean.isSingleton() && this.allowCircularReferences && this.isSingletonCurrentlyInCreation(name) && bean.isSingleton()) ? 1:0;`,
		`int saved=(bean.isSingleton() && this.allowCircularReferences && this.isSingletonCurrentlyInCreation(other)) ? 1:0;`,
		`int saved=(bean.isSingleton() && this.allowCircularReferences && this.isSingletonCurrentlyInCreation(name)) ? 2:0;`,
	} {
		if _, ok := reviewedThreePredicateWord(bad, "bean", "name"); ok {
			t.Fatalf("unproven word accepted: %s", bad)
		}
	}
}

// The second consumer must use the captured word even when all sources mutate.
// Each callable predicate has a trace and singleton failure; the direct-field
// variant makes the first call set the field so reordering the read changes truth.
func TestAdversarialThreePredicateCanonicalWordTwoConsumersRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const owner = `public class ThreePredicateWordOwner {
 static int mask,fail;static boolean allow;static String trace="";static final RuntimeException SAME=new RuntimeException("identity");
 static boolean first(){trace+="A;";if(fail==0)throw SAME;allow=(mask&2)!=0;return (mask&1)!=0;}
 static boolean second(){trace+="B;";if(fail==1)throw SAME;return (mask&2)!=0;}
 static boolean third(){trace+="C;";if(fail==2)throw SAME;allow=false;return (mask&4)!=0;}
 static void mutate(){mask^=7;allow=!allow;trace+="M;";}
 static int calls(){int saved=first()&&second()&&third()?1:0;int score=0;if(saved!=0){trace+="Y;";score=11;}else{trace+="N;";}mutate();if(saved!=0){trace+="Y;";score+=23;}else{trace+="N;";}return score;}
 static int field(){int saved=first()&&allow&&third()?1:0;int score=0;if(saved!=0){trace+="Y;";score=11;}else{trace+="N;";}mutate();if(saved!=0){trace+="Y;";score+=23;}else{trace+="N;";}return score;}
}`
	var driver, want strings.Builder
	driver.WriteString(`public class ThreePredicateWordDriver {public static void main(String[] args){`)
	for _, kind := range []string{"calls", "field"} {
		for mask := 0; mask < 8; mask++ {
			for fail := -1; fail < 3; fail++ {
				fmt.Fprintf(&driver, `ThreePredicateWordOwner.mask=%d;ThreePredicateWordOwner.fail=%d;ThreePredicateWordOwner.allow=%t;ThreePredicateWordOwner.trace="";try{System.out.print(ThreePredicateWordOwner.%s()+":");}catch(RuntimeException caught){System.out.print((caught==ThreePredicateWordOwner.SAME)+":");}System.out.println(ThreePredicateWordOwner.trace);`, mask, fail, mask&2 == 0, kind)
				// Independent truth table plus the reachable predicate prefix; it never
				// invokes any decompiler graph/reduction/type helper.
				reached := []int{0}
				if mask&1 != 0 {
					if kind == "calls" {
						reached = append(reached, 1)
					}
					if mask&2 != 0 {
						reached = append(reached, 2)
					}
				}
				var trace strings.Builder
				thrown := false
				for _, p := range reached {
					trace.WriteString([]string{"A;", "B;", "C;"}[p])
					if p == fail {
						thrown = true
						break
					}
				}
				if thrown {
					fmt.Fprintf(&want, "true:%s\n", trace.String())
				} else {
					outcome := "N;M;N;"
					score := 0
					if mask == 7 {
						outcome = "Y;M;Y;"
						score = 34
					}
					fmt.Fprintf(&want, "%d:%s%s\n", score, trace.String(), outcome)
				}
			}
		}
	}
	driver.WriteString("}}")
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			of := filepath.Join(original, "ThreePredicateWordOwner.java")
			df := filepath.Join(original, "ThreePredicateWordDriver.java")
			if err := os.WriteFile(of, []byte(owner), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(df, []byte(driver.String()), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, of, df).CombinedOutput(); err != nil {
				t.Fatalf("original %v\n%s", err, out)
			}
			if got := t04RunJava(t, java, original, "ThreePredicateWordDriver"); got != want.String() {
				t.Fatalf("independent64-case original oracle got%q want%q", got, want.String())
			}
			raw, err := os.ReadFile(filepath.Join(original, "ThreePredicateWordOwner.class"))
			if err != nil {
				t.Fatal(err)
			}
			resolve := func(n string) ([]byte, bool) {
				b, e := os.ReadFile(filepath.Join(original, filepath.FromSlash(n)+".class"))
				return b, e == nil
			}
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				t.Run(string(mode), func(t *testing.T) {
					var result DecompileResult
					var err error
					if mode == "legacy" {
						result.Source, err = DecompileWithResolver(raw, resolve)
					} else {
						result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					}
					if err != nil || len(result.StubMethods) > 0 {
						t.Fatalf("reconstruction %v %v", err, result.StubMethods)
					}
					rebuilt := t.TempDir()
					f := filepath.Join(rebuilt, "ThreePredicateWordOwner.java")
					if err := os.WriteFile(f, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, f).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt %v\n%s\n%s", err, out, result.Source)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "ThreePredicateWordDriver"); got != want.String() {
						t.Fatalf("producer order/once/captured consumers changed got%q want%q\n%s", got, want.String(), result.Source)
					}
				})
			}
		})
	}
}

// Match the entire enclosing consumer, not a logical child substring. Exact
// original member/branch proofs live beside the native source assertions.
func reviewedCanonicalPredicateIf(body, predicate string, nonzero bool) (int, int, bool) {
	compact := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	strip := func(s string) string { return s }
	strip = func(s string) string {
		for len(s) > 1 && s[0] == '(' && s[len(s)-1] == ')' {
			depth, encloses := 0, true
			for i, r := range s {
				if r == '(' {
					depth++
				}
				if r == ')' {
					depth--
				}
				if depth == 0 && i < len(s)-1 {
					encloses = false
					break
				}
			}
			if !encloses {
				break
			}
			s = s[1 : len(s)-1]
		}
		return s
	}
	split := func(s, operator string) (string, string, bool) {
		depth := 0
		for i := 0; i+len(operator) <= len(s); i++ {
			if s[i] == '(' {
				depth++
			}
			if s[i] == ')' {
				depth--
			}
			if depth == 0 && strings.HasPrefix(s[i:], operator) {
				return s[:i], s[i+len(operator):], true
			}
		}
		return "", "", false
	}
	var equivalent func(string, string) bool
	equivalent = func(got, want string) bool {
		got, want = strip(got), strip(want)
		for _, operator := range []string{"||", "&&", "=="} {
			wa, wb, has := split(want, operator)
			if has {
				ga, gb, ok := split(got, operator)
				return ok && equivalent(ga, wa) && equivalent(gb, wb)
			}
		}
		return got == want
	}
	matches := func(expression string) bool {
		expression = strip(compact(expression))
		if nonzero && equivalent(expression, predicate) {
			return true
		}
		if !nonzero && strings.HasPrefix(expression, "!") && equivalent(expression[1:], predicate) {
			return true
		}
		if _, _, ok := split(expression, "||"); ok {
			return false
		}
		if _, _, ok := split(expression, "&&"); ok {
			return false
		}
		operator := "=="
		if nonzero {
			operator = "!="
		}
		left, right, ok := split(expression, operator)
		if !ok {
			return false
		}
		if strip(right) == "false" {
			return equivalent(left, predicate)
		}
		if strip(right) != "0" {
			return false
		}
		condition, arms, ok := split(strip(left), "?")
		if !ok {
			return false
		}
		trueArm, falseArm, ok := split(arms, ":")
		return ok && strip(trueArm) == "1" && strip(falseArm) == "0" && equivalent(condition, predicate)
	}
	for start := 0; start < len(body); {
		i := strings.Index(body[start:], "if (")
		if i < 0 {
			return 0, 0, false
		}
		i += start
		open := i + 3
		depth, end := 0, -1
		for j := open; j < len(body); j++ {
			if body[j] == '(' {
				depth++
			}
			if body[j] == ')' {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			return 0, 0, false
		}
		if matches(body[open+1 : end]) {
			return i, end + 1, true
		}
		start = end + 1
	}
	return 0, 0, false
}

func TestReviewedCanonicalPredicateRejectsWrongGroupingAndWordConsumer(t *testing.T) {
	const predicate = "left||right&&suffix"
	for _, body := range []string{"if ((left || (right && suffix)) == false) {}", "if (!((left) || ((right) && (suffix)))) {}", "if (((left || (right && suffix)) ? 1 : 0) == 0) {}"} {
		if _, _, ok := reviewedCanonicalPredicateIf(body, predicate, false); !ok {
			t.Fatalf("valid canonical consumer rejected: %s", body)
		}
	}
	for _, body := range []string{"if (left || (right && suffix) == false) {}", "if ((left || right) && suffix == false) {}", "if ((left || (right && suffix)) == 0) {}", "if (((left || (right && suffix)) ? 2 : 3) == 0) {}", "if ((right || (left && suffix)) == false) {}"} {
		if _, _, ok := reviewedCanonicalPredicateIf(body, predicate, false); ok {
			t.Fatalf("wrong grouping/order or bool-vs-word consumer accepted: %s", body)
		}
	}
}
