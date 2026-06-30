package javaclassparser

// 种子回归框架, 对应 CODEC_TODO 里引用的 TestDecompileSyntaxRegression。它遍历
// testdata/regression/ 下的每个 .class 种子, 反编译, 断言不 panic / 不报错, 并按同名
// .golden 文件里的「必须包含 / 必须不包含」规则校验产出源码。
//
// golden 文件格式 (每行一条规则, 可选; 不存在则只做「能反编译且不 panic」冒烟):
//
//	# 注释行
//	+substring      产出源码必须包含 substring
//	-substring      产出源码必须不包含 substring
//	+/regexp/       产出源码必须匹配正则 regexp
//	-/regexp/       产出源码必须不匹配正则 regexp
//
// 每个治本把它 pin 的真实 .class 放进 testdata/regression/, 配一个 .golden 锁住治本前后
// 的可见差异 (kill-switch 的 ON/OFF 断言由各自的 Test*IsLoadBearing 承重测试负责)。

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const regressionDir = "testdata/regression"

// goldenRule is one assertion parsed from a .golden file.
type goldenRule struct {
	mustContain bool   // true: must contain/match; false: must NOT
	isRegexp    bool   // true: pattern is a regexp; false: literal substring
	pattern     string // the literal or regexp source
	raw         string // original line, for error messages
}

func parseGolden(t *testing.T, path string) []goldenRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rules []goldenRule
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != '+' && line[0] != '-' {
			t.Fatalf("%s: golden rule must start with + or -: %q", path, line)
		}
		r := goldenRule{mustContain: line[0] == '+', raw: line}
		body := line[1:]
		if len(body) >= 2 && strings.HasPrefix(body, "/") && strings.HasSuffix(body, "/") {
			r.isRegexp = true
			r.pattern = body[1 : len(body)-1]
		} else {
			r.pattern = body
		}
		rules = append(rules, r)
	}
	return rules
}

func checkGolden(t *testing.T, name, src string, rules []goldenRule) {
	t.Helper()
	for _, r := range rules {
		var hit bool
		if r.isRegexp {
			re, err := regexp.Compile(r.pattern)
			if err != nil {
				t.Fatalf("%s: bad golden regexp %q: %v", name, r.pattern, err)
			}
			hit = re.MatchString(src)
		} else {
			hit = strings.Contains(src, r.pattern)
		}
		if r.mustContain && !hit {
			t.Errorf("%s: expected source to satisfy rule %q but it did not", name, r.raw)
		}
		if !r.mustContain && hit {
			t.Errorf("%s: expected source to NOT satisfy rule %q but it did", name, r.raw)
		}
	}
}

// decompileSeed decompiles a single seed class. Seeds are standalone units, so the plain Decompile
// path is used (no cross-class resolver); panics are recovered by Decompile and surface as errors.
func decompileSeed(data []byte) (string, error) {
	return Decompile(data)
}

// TestDecompileSyntaxRegression decompiles every seed and validates it does not panic/error and
// satisfies its golden rules. It skips cleanly when no seeds are present yet.
func TestDecompileSyntaxRegression(t *testing.T) {
	classes, err := filepath.Glob(filepath.Join(regressionDir, "*.class"))
	if err != nil {
		t.Fatalf("glob seeds: %v", err)
	}
	if len(classes) == 0 {
		t.Skipf("no regression seeds under %s yet", regressionDir)
	}
	sort.Strings(classes)
	for _, classPath := range classes {
		classPath := classPath
		name := strings.TrimSuffix(filepath.Base(classPath), ".class")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(classPath)
			if err != nil {
				t.Fatalf("read seed: %v", err)
			}
			src, err := decompileSeed(data)
			if err != nil {
				t.Fatalf("decompile %s failed: %v", name, err)
			}
			if strings.TrimSpace(src) == "" {
				t.Fatalf("decompile %s produced empty source", name)
			}
			rules := parseGolden(t, filepath.Join(regressionDir, name+".golden"))
			checkGolden(t, name, src, rules)
		})
	}
}
