package jarwar

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/utils"
)

// ArchiveOptions controls archive decompilation. Structured keeps original
// nested archives separate from their expanded source and resource views.
// Context, Limits, and Work are shared by every nested archive layer.
type ArchiveOptions struct {
	Structured bool
	Context    context.Context
	Limits     javaclassparser.Limits
	Work       *javaclassparser.WorkBudget
	// MaxAnalysisUpdates bounds per-method reaching-definition work. Zero keeps
	// the default policy; large positive values scale graph caps.
	MaxAnalysisUpdates int
	TargetRelease      int
}

// DecompileArchiveOptions is the explicit name for ArchiveOptions.
type DecompileArchiveOptions = ArchiveOptions

// ArchiveEntryReport records one deterministic structured-export output.
// Paths are slash-separated and relative to the destination directory.
type ArchiveEntryReport struct {
	Kind       string `json:"kind"`
	SourcePath string `json:"source_path"`
	OutputPath string `json:"output_path"`
	Archive    string `json:"archive,omitempty"`
	Size       int64  `json:"size"`
}

// NestedArchiveReport records the provenance and preserved bytes of a nested
// archive. Path uses !/ between archive layers; Parent is empty for the input
// archive's direct children.
type NestedArchiveReport struct {
	Path       string `json:"path"`
	Parent     string `json:"parent,omitempty"`
	OutputPath string `json:"output_path"`
	Depth      int    `json:"depth"`
	Size       int64  `json:"size"`
}

// ArchiveReport summarizes a structured archive export. Slice order is stable:
// lexical archive traversal order, depth first.
type ArchiveReport struct {
	Entries  []ArchiveEntryReport  `json:"entries"`
	Archives []NestedArchiveReport `json:"archives"`
}

// DecompileArchiveReport is the explicit name for ArchiveReport.
type DecompileArchiveReport = ArchiveReport

// Decompile 反编译一个 jar包或者 class
// 返回值是反编译后的 java 文件路径
// 参数:
//   - from: 待反编译的 jar/war/class 文件路径
//   - to: 反编译结果输出的目录或文件路径
//
// 返回值:
//   - error: 反编译失败时返回错误
//
// Example:
// ```
// // 无法本地验证: 需要磁盘上真实存在的 jar/war/class 文件(请替换为真实路径)
// err = java.Decompile("test.jar", "test/"); die(err)   // 反编译 jar 到目录
// err = java.Decompile("test.war", "test/"); die(err)   // 反编译 war 到目录
// err = java.Decompile("a.class", "a.java"); die(err)   // 反编译单个 class 到文件
// ```
func AutoDecompile(from, to string) error {
	return AutoDecompileWithAnalysisBudget(from, to, 0)
}

// AutoDecompileWithAnalysisBudget is AutoDecompile with an explicit per-method
// reaching-definition/graph budget for large generated methods.
func AutoDecompileWithAnalysisBudget(from, to string, maxAnalysisUpdates int) error {
	if maxAnalysisUpdates < 0 {
		return fmt.Errorf("negative analysis budget: %d", maxAnalysisUpdates)
	}
	return autoDecompileFlat(from, to, maxAnalysisUpdates)
}

// autoDecompileFlat writes the historical flat directory layout.
func autoDecompileFlat(from, to string, maxAnalysisUpdates int) error {
	fromLower := strings.ToLower(from)
	switch {
	case strings.HasSuffix(fromLower, ".jar"), strings.HasSuffix(fromLower, ".war"),
		strings.HasSuffix(fromLower, ".ear"), strings.HasSuffix(fromLower, ".zip"):
		jar, err := NewWithAnalysisBudget(from, maxAnalysisUpdates)
		if err != nil {
			return utils.Errorf("create archive failed: %v", err)
		}
		defer jar.Close()
		return jar.DumpToLocalFileSystem(to)
	case strings.HasSuffix(fromLower, ".class"):
		raw, err := os.ReadFile(from)
		if err != nil {
			return utils.Errorf("read class file failed: %v", err)
		}
		var decompiled string
		if maxAnalysisUpdates > 0 {
			result, derr := javaclassparser.DecompileWithOptions(raw, javaclassparser.DecompileOptions{
				Mode:               javaclassparser.Compatibility,
				MaxAnalysisUpdates: maxAnalysisUpdates,
			})
			if derr != nil {
				return utils.Errorf("decompile class failed: %v", derr)
			}
			if result.Source == "" {
				return utils.Errorf("decompile class produced no source (status=%s)", result.Status)
			}
			decompiled = result.Source
		} else {
			stm, err := javaclassparser.Parse(raw)
			if err != nil {
				return utils.Errorf("decompile class failed: %v", err)
			}
			decompiled, err = stm.Dump()
			if err != nil {
				return utils.Errorf("dump class file failed: %v", err)
			}
		}

		// 检查并确保输出文件有 .java 后缀
		if !strings.HasSuffix(to, ".java") {
			to = to + ".java"
		}

		if err := os.WriteFile(to, []byte(decompiled), 0644); err != nil {
			return utils.Errorf("write java file failed: %v", err)
		}
		return nil
	}
	return utils.Errorf("unknown file type: %v", from)
}

// AutoDecompileWithOptions decompiles an archive and returns an export report.
// The zero-value options preserve AutoDecompile behavior. Set Structured to
// produce separate sources, resources, and original nested-archive trees.
func AutoDecompileWithOptions(from, to string, options ArchiveOptions) (ArchiveReport, error) {
	if options.MaxAnalysisUpdates < 0 {
		return ArchiveReport{}, fmt.Errorf("negative analysis budget: %d", options.MaxAnalysisUpdates)
	}
	if !options.Structured {
		return ArchiveReport{}, autoDecompileFlat(from, to, options.MaxAnalysisUpdates)
	}
	return decompileArchiveStructured(from, to, options)
}
