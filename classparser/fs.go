package javaclassparser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/javajive/internal/filesys"
	fi "github.com/yaklang/javajive/internal/filesys/filesys_interface"
	"github.com/yaklang/javajive/internal/inputkind"
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/memfile"
	"github.com/yaklang/javajive/internal/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

type JarFS struct {
	jarReleaseMu        sync.Mutex
	jarReleaseSelection *jarReleaseSelection
	jarReleaseBinding   *filesys.ZipFS
	jarReleaseRole      jarReleaseRole
	jarPhysicalRelease  int

	// Immutable compilation profile, distinct from Multi-Release lookup.
	targetSourceVersion       int
	maxAnalysisUpdates        int
	sourceCompiler            SourceCompilerProfile
	sourceFieldMetadata       *sourceFieldMetadataAuthority
	sourceConstructorLowering *sourceConstructorLoweringAuthority
	sourceLambdaMetadata      *sourceLambdaMetadataAuthority
	sourceOwnership           *sourceOwnershipCache
	releaseViewsMu            sync.Mutex
	releaseViews              map[int]*JarFS
	nativeMembersMu           sync.Mutex
	nativeMembersIndex        nativeMemberIndex
	nativeMembersCache        map[string]*nativeMemberCacheEntry
	nativeMemberTransactions  map[string]*nativeMemberTransaction
	nativeMembersBytes        int64
	nativeAnonymousMu         sync.Mutex
	nativeAnonymousCache      map[string]*nativeAnonymousCacheEntry
	nativeAnonymousBytes      int64
	*filesys.ZipFS
	jarCache             *utils.SafeMapWithKey[string, *filesys.UnifiedFS]
	recursiveParse       bool // 是否递归解析嵌套的jar文件，默认为true
	archive              *archiveFSState
	declarationResolver  func(string) ([]byte, bool)
	retainUnprovedSource bool
}

// SetRetainUnprovedSource keeps original members when a javac source certificate
// fails. Nested archives created afterwards inherit it. The default refuses.
func (z *JarFS) SetRetainUnprovedSource(enabled bool) {
	if z != nil {
		z.retainUnprovedSource = enabled
	}
}

// RetainsUnprovedSource reports the analysis-source policy for this archive.
func (z *JarFS) RetainsUnprovedSource() bool {
	return z != nil && z.retainUnprovedSource
}

var _ fs.FS = (*JarFS)(nil)
var _ fs.ReadFileFS = (*JarFS)(nil)
var _ fs.ReadDirFS = (*JarFS)(nil)

// ClassDecompileStatus describes one archive class-entry recovery attempt.
// Fallback is a contained class-level failure represented by diagnostic source;
// request resource and cancellation failures are returned as errors instead.
type ClassDecompileStatus string

const (
	ClassDecompileComplete ClassDecompileStatus = "complete"
	ClassDecompilePartial  ClassDecompileStatus = "partial"
	ClassDecompileFallback ClassDecompileStatus = "fallback"
)

// ClassDecompileOutcome is the typed result at the JarFS class-entry boundary.
// Source preserves the historical ReadFile bytes, while StubMethods and
// Diagnostics expose degradation evidence to structured archive exporters.
type ClassDecompileOutcome struct {
	Source      []byte
	Status      ClassDecompileStatus
	StubMethods []string
	Diagnostics []DecompileDiagnostic
}

func NewJarFSFromLocal(path string) (*JarFS, error) {
	return NewJarFSFromLocalWithResolver(path, nil)
}

// NewJarFSFromLocalWithResolver reads additional declaration bytes without
// adding them to the target's source units. Target bytes always take precedence.
// The resolver must remain stable, and be safe for concurrent reads, throughout
// the filesystem's lifetime. Missing or invalid declarations remain unproved.
func NewJarFSFromLocalWithResolver(path string, resolve func(string) ([]byte, bool)) (*JarFS, error) {
	return NewJarFSFromLocalWithSourceVersion(path, 0, resolve)
}

// NewJarFSFromLocalWithSourceVersion selects the intended javac --release for
// generated source. Zero retains per-class inference. The source target must
// be supplied when it differs from the original classfile version: nestmate
// compilers do not regenerate Java8 access bridges and their marker classes.
// This is independent of which Multi-Release archive entries are selected.
func NewJarFSFromLocalWithSourceVersion(path string, target int, resolve func(string) ([]byte, bool)) (*JarFS, error) {
	return NewJarFSFromLocalWithCompilerProfile(path, target, "", resolve)
}

// SetAnalysisBudget applies a per-method reaching-definition budget to classes
// read from this archive. Nested archives opened afterwards inherit it.
func (z *JarFS) SetAnalysisBudget(maxAnalysisUpdates int) {
	if z != nil {
		z.maxAnalysisUpdates = maxAnalysisUpdates
	}
}

// NewJarFSFromLocalWithAnalysisBudget is NewJarFSFromLocalWithSourceVersion
// with an explicit per-method reaching-definition budget. A positive value
// above the public 1M default also scales the derived graph caps.
func NewJarFSFromLocalWithAnalysisBudget(path string, target int, maxAnalysisUpdates int, resolve func(string) ([]byte, bool)) (*JarFS, error) {
	z, err := NewJarFSFromLocalWithCompilerProfile(path, target, "", resolve)
	if err != nil {
		return nil, err
	}
	z.maxAnalysisUpdates = maxAnalysisUpdates
	return z, nil
}

// NewJarFSFromLocalWithCompilerProfile fixes the source language and compiler
// lowering for the filesystem's lifetime. It does not select a JDK executable;
// callers must rebuild using the chosen compiler profile.
func NewJarFSFromLocalWithCompilerProfile(path string, target int, compiler SourceCompilerProfile, resolve func(string) ([]byte, bool)) (*JarFS, error) {
	if target != 0 && (target < 8 || target > 21) {
		return nil, fmt.Errorf("unsupported target source version %d", target)
	}
	if err := compiler.validate(target); err != nil {
		return nil, err
	}
	zipFS, err := filesys.NewZipFSFromLocal(path)
	if err != nil {
		return nil, err
	}
	z := NewJarFS(zipFS)
	z.targetSourceVersion = target
	z.sourceCompiler = compiler.effective()
	z.declarationResolver = resolve
	return z, nil
}

// NewJarFSFromLocalWithOptions is like NewJarFSFromLocal but lets the caller
// control nested-jar recursion (matching SSA jar_recursive_parse semantics).
func NewJarFSFromLocalWithOptions(path string, recursiveParse bool) (*JarFS, error) {
	zipFS, err := filesys.NewZipFSFromLocal(path)
	if err != nil {
		return nil, err
	}
	return NewJarFSWithOptions(zipFS, recursiveParse), nil
}

func NewJarFS(zipFs *filesys.ZipFS) *JarFS {
	return NewJarFSWithOptions(zipFs, true) // 默认启用递归解析
}

func NewJarFSWithOptions(zipFs *filesys.ZipFS, recursiveParse bool) *JarFS {
	return &JarFS{
		sourceCompiler:  ModernJavac,
		ZipFS:           zipFs,
		sourceOwnership: &sourceOwnershipCache{},
		jarCache:        utils.NewSafeMapWithKey[string, *filesys.UnifiedFS](),
		recursiveParse:  recursiveParse,
		archive:         archiveStateFromZipFS(zipFs),
	}
}

// decompileClassOutcome decompiles a single .class entry from this jar and
// contains class-level failures. Beyond the per-class Dump it performs enum
// constant-body CROSS-CLASS folding (Bug V): javac lowers each constant-specific
// body `CONST { ...body... }` into a synthetic `Outer$N` subclass (ACC_ENUM,
// `extends Outer`). Decompiled per class that is uncompilable two ways -- the
// outer enum's constants do not override its abstract methods, and each
// `Outer$N` renders as illegal `class Outer$N extends <enum>`. Here, with the
// whole jar in hand, we (a) when decompiling the OUTER enum, inline each
// `Outer$N` body back into its constant via a sibling resolver over the zip, and
// (b) when decompiling a synthetic `Outer$N` itself, emit only a suppression
// marker -- its body now lives in the enum and javac regenerates the real
// `Outer$N.class` from the folded enum source on recompile. Gated by
// JDEC_NO_ENUM_FOLD so it can be disabled for load-bearing regression and to
// fall back to the exact per-class behavior.
func (z *JarFS) decompileClassOutcome(name string, data []byte) (outcome ClassDecompileOutcome, err error) {
	if _, bound := jdecenv.Current(); !bound {
		err = jdecenv.Run(jdecenv.CaptureProcess(), func() error {
			var inner error
			outcome, inner = z.decompileClassOutcome(name, data)
			return inner
		})
		return outcome, err
	}
	var work *workbudget.Budget
	decompile := func() (ClassDecompileOutcome, error) {
		return z.decompileClassOutcomeBound(name, data, &work)
	}
	return z.containClassDecompile(name, &work, decompile)
}

func (z *JarFS) decompileClassOutcomeBound(name string, data []byte, work **workbudget.Budget) (ClassDecompileOutcome, error) {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".class") && !strings.HasSuffix(lower, ".raw") {
		return ClassDecompileOutcome{Source: data, Status: ClassDecompileComplete}, nil
	}
	if strings.HasSuffix(lower, ".raw") && (len(data) < 4 || data[0] != 0xca || data[1] != 0xfe || data[2] != 0xba || data[3] != 0xbe) {
		return ClassDecompileOutcome{Source: data, Status: ClassDecompileComplete}, nil
	}
	cf, err := Parse(data)
	if err != nil {
		message := sanitizeClassText(err.Error())
		return ClassDecompileOutcome{
			Source: []byte(fmt.Sprintf("// decompile parse failed for %s: %s\n", sanitizeClassText(name), message)),
			Status: ClassDecompileFallback,
			Diagnostics: []DecompileDiagnostic{{
				Code:    "class_parse_failed",
				Message: message,
			}},
		}, nil
	}
	if err := ValidateSupportedClassMajor(cf.MajorVersion); err != nil {
		return classFallbackOutcome(name, "unsupported_classfile_version", err.Error()), nil
	}
	if jdecenv.Get("JDEC_NO_ENUM_FOLD") == "" {
		if isSyntheticEnumConstantSubclass(cf) {
			return ClassDecompileOutcome{Source: []byte(fmt.Sprintf(
				"// %s: synthetic enum constant-body subclass folded into enum %s (regenerated by javac on recompile)\n",
				strings.ReplaceAll(cf.GetClassName(), "/", "."),
				strings.ReplaceAll(cf.GetSupperClassName(), "/", "."))), Status: ClassDecompileComplete}, nil
		}
	}
	if release, logical, known := physicalClassNamespace(name); known && logical == cf.GetClassName()+".class" {
		if view := z.sourceReleaseView(release); view != nil {
			return view.decompileClassOutcome(logical, data)
		}
	}
	if path.Clean(name) == cf.GetClassName()+".class" {
		if source, owned := z.nativeMemberSource(cf); owned {
			z.noteNativeConstructorSource(cf, source)
			return ClassDecompileOutcome{Source: source, Status: ClassDecompileComplete}, nil
		}
		if source, owned := z.nativeAnonymousSource(cf); owned {
			z.noteNativeConstructorSource(cf, source)
			return ClassDecompileOutcome{Source: source, Status: ClassDecompileComplete}, nil
		}
	}
	// Supply original sibling bytes for certified source ownership and enum
	// constant-body reconstruction. Enum-switch syntax requires its original
	// typed protocol; a mapping-looking source fragment grants no permission.
	d := NewClassObjectDumper(cf)
	report := DecompileResult{Members: newMemberLedger(cf)}
	d.report = &report
	d.options.TargetSourceVersion = z.targetSourceVersion
	d.options.MaxAnalysisUpdates = z.maxAnalysisUpdates
	d.options.SourceCompiler = z.sourceCompiler
	d.options.sourceFieldMetadata = z.sourceFieldMetadata
	d.options.sourceConstructorLowering = z.sourceConstructorLowering
	d.options.sourceLambdaMetadata = z.sourceLambdaMetadata
	d.options.RetainUnprovedSource = z.retainUnprovedSource
	if path.Clean(name) == cf.GetClassName()+".class" {
		d.nativeMemberLookup = z.nativeMemberLookup
	}
	d.foldSiblingResolver = z.enumSiblingResolver()
	d.declarationResolver = z.declarationResolver
	if z.archive != nil && z.archive.budget != nil {
		d.Work = z.archive.budget.Work()
		d.options.Context = z.archive.ctx
		d.options.TargetRelease = z.archive.targetRelease
	}
	if d.Work == nil {
		d.Work = largeMethodWorkBudget(z.maxAnalysisUpdates)
	}
	*work = d.Work
	source, err := d.DumpClass()
	if err != nil {
		if isClassResourceError(err) {
			return ClassDecompileOutcome{}, err
		}
		message := sanitizeClassText(err.Error())
		report.Diagnostics = append(report.Diagnostics, DecompileDiagnostic{Code: "class_decompile_failed", Message: message})
		return ClassDecompileOutcome{
			Source:      []byte(fmt.Sprintf("// decompile dump failed for %s: %s\n", sanitizeClassText(name), message)),
			Status:      ClassDecompileFallback,
			StubMethods: report.StubMethods,
			Diagnostics: report.Diagnostics,
		}, nil
	}
	if resourceErr := z.classResourceError(d.Work); resourceErr != nil {
		return ClassDecompileOutcome{}, resourceErr
	}
	status := ClassDecompileComplete
	if len(report.StubMethods) > 0 || d.typeAnnosUnsupported || d.enumParameterMetadataUnsupported {
		status = ClassDecompilePartial
	}
	return ClassDecompileOutcome{
		Source:      []byte(source),
		Status:      status,
		StubMethods: report.StubMethods,
		Diagnostics: report.Diagnostics,
	}, nil
}

func (z *JarFS) containClassDecompile(name string, work **workbudget.Budget, decompile func() (ClassDecompileOutcome, error)) (outcome ClassDecompileOutcome, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var current *workbudget.Budget
			if work != nil {
				current = *work
			}
			if resourceErr := z.classResourceError(current); resourceErr != nil {
				outcome = ClassDecompileOutcome{}
				err = resourceErr
				return
			}
			message := fmt.Sprintf("decompile panic: %v", recovered)
			outcome = classFallbackOutcome(name, "class_decompile_panic", message)
			err = nil
		}
	}()
	return decompile()
}

func (z *JarFS) classResourceError(work *workbudget.Budget) error {
	if work != nil && work.Err() != nil {
		return work.Err()
	}
	if z != nil && z.archive != nil {
		if z.archive.budget != nil && z.archive.budget.Err() != nil {
			return z.archive.budget.Err()
		}
		if z.archive.ctx != nil && z.archive.ctx.Err() != nil {
			return z.archive.ctx.Err()
		}
	}
	return nil
}

func classFallbackOutcome(name, code, message string) ClassDecompileOutcome {
	message = sanitizeClassText(message)
	return ClassDecompileOutcome{
		Source: []byte(fmt.Sprintf("// decompile fallback for %s: %s\n", sanitizeClassText(name), message)),
		Status: ClassDecompileFallback,
		Diagnostics: []DecompileDiagnostic{{
			Code:    code,
			Message: message,
		}},
	}
}

// sanitizeClassText keeps fallback comments on one line and bounds external
// diagnostic text so malformed names or errors cannot inject source lines.
func sanitizeClassText(text string) string {
	runes := []rune(text)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r == '\n' || r == '\r' || r < 0x20 || r == 0x7f {
			r = ' '
		}
		out = append(out, r)
		if len(out) == 480 {
			break
		}
	}
	return strings.TrimSpace(string(out))
}

func isClassResourceError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var budgetErr *workbudget.Error
	return errors.As(err, &budgetErr) && budgetErr.Kind != workbudget.KindBudget
}

// enumSiblingResolver resolves a sibling class's RAW bytes by binary internal name (slash form, e.g.
// "com/google/common/math/LongMath$1") from the underlying zip, feeding enum constant-body folding.
func (z *JarFS) enumSiblingResolver() func(internalName string) ([]byte, bool) {
	return func(internalName string) ([]byte, bool) {
		raw, err := z.readJarEntry(internalName + ".class")
		if err != nil || len(raw) == 0 {
			return nil, false
		}
		return raw, true
	}
}

func (z *JarFS) ReadFile(name string) ([]byte, error) {
	if isArchivePath(name) {
		return z.readFileFromJar(name)
	}
	outcome, err := z.ReadClassOutcome(name)
	if err != nil {
		return nil, err
	}
	return outcome.Source, nil
}

// ReadClassOutcome reads and decompiles one archive entry with typed
// class-level degradation evidence. Non-class entries are returned complete.
func (z *JarFS) ReadClassOutcome(name string) (ClassDecompileOutcome, error) {
	if isArchivePath(name) {
		jarPath, internalPath, ok := parseArchivePath(name)
		if !ok {
			return ClassDecompileOutcome{}, os.ErrNotExist
		}
		unifiedFS, err := z.getNestedJarFS(jarPath)
		if err != nil {
			return ClassDecompileOutcome{}, err
		}
		nested, ok := unifiedFS.GetFileSystem().(*JarFS)
		if !ok {
			return ClassDecompileOutcome{}, fmt.Errorf("nested archive %q is not a JarFS", jarPath)
		}
		return nested.ReadClassOutcome(internalPath)
	}
	data, err := z.readJarEntry(name)
	if err != nil {
		return ClassDecompileOutcome{}, err
	}
	return z.decompileClassOutcome(name, data)
}

// ReadRawFile returns the original bytes for name without decompiling class
// entries. Nested archive paths such as "WEB-INF/lib/dep.jar/Foo.class" are
// resolved through every archive layer.
func (z *JarFS) ReadRawFile(name string) ([]byte, error) {
	if isArchivePath(name) {
		jarPath, internalPath, ok := parseArchivePath(name)
		if !ok {
			return nil, os.ErrNotExist
		}
		unifiedFS, err := z.getNestedJarFS(jarPath)
		if err != nil {
			return nil, err
		}
		nested, ok := unifiedFS.GetFileSystem().(*JarFS)
		if !ok {
			return nil, fmt.Errorf("nested archive %q is not a JarFS", jarPath)
		}
		return nested.ReadRawFile(internalPath)
	}
	return z.readJarEntry(name)
}

func (f *JarFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.Open(name)
}

func (z *JarFS) Open(name string) (fs.File, error) {
	raw, err := z.readJarEntry(name)
	if err != nil {
		return nil, err
	}
	outcome, err := z.decompileClassOutcome(name, raw)
	if err != nil {
		return nil, err
	}
	return memfile.New(outcome.Source), nil
}

func (z *JarFS) Stat(name string) (fs.FileInfo, error) {
	if isArchivePath(name) {
		return z.statInJar(name)
	}
	if isArchiveFile(name) {
		info, err := z.ZipFS.Stat(name)
		if err != nil {
			return nil, err
		}
		return &ArchiveFileInfo{
			fs:    info,
			name:  info.Name(),
			isDir: true,
		}, nil
	}
	info, err := z.ZipFS.Stat(name)
	if err != nil {
		return nil, err
	}
	return &ArchiveFileInfo{
		fs:    info,
		name:  info.Name(),
		isDir: info.IsDir(),
	}, nil
}

var (
	archiveFileSuffixes    = []string{".jar", ".war", ".ear", ".aar", ".par", ".jmod", ".zip"}
	jarLikeArchiveSuffixes = []string{".jar", ".war", ".ear", ".aar", ".par", ".jmod"}
)

func isArchiveFile(path string) bool {
	path = normalizeArchivePath(path)
	lowerPath := strings.ToLower(path)
	for _, ext := range archiveFileSuffixes {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func isArchivePath(name string) bool {
	name = normalizeArchivePath(name)
	lowerPath := strings.ToLower(name)
	for _, ext := range archiveFileSuffixes {
		if strings.Contains(lowerPath, ext+"/") || strings.Contains(lowerPath, ext+"!") {
			return true
		}
	}
	return false
}

func parseArchivePath(fullPath string) (archivePath, internalPath string, ok bool) {
	fullPath = normalizeArchivePath(fullPath)
	lowerPath := strings.ToLower(fullPath)

	for _, ext := range archiveFileSuffixes {
		if strings.Contains(lowerPath, ext+"/") || strings.Contains(lowerPath, ext+"!") {
			return parseJarOrZipPath(fullPath, ext)
		}
	}
	return "", "", false
}

func isJarLikeArchivePath(path string) bool {
	path = normalizeArchivePath(path)
	lowerPath := strings.ToLower(path)
	for _, ext := range jarLikeArchiveSuffixes {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func parseJarOrZipPath(fullPath, ext string) (archivePath, internalPath string, ok bool) {
	extWithSlash := ext + "/"
	extWithBang := ext + "!"

	idx := strings.Index(fullPath, extWithSlash)
	if idx == -1 {
		bangIdx := strings.Index(fullPath, extWithBang)
		if bangIdx == -1 {
			return "", "", false
		}
		archivePath = fullPath[:bangIdx+len(ext)]
		internalPath = strings.TrimPrefix(fullPath[bangIdx+len(extWithBang):], "/")
		return archivePath, internalPath, true
	}

	archivePath = fullPath[:idx+len(ext)]
	internalPath = strings.TrimPrefix(fullPath[idx+len(extWithSlash):], "/")
	return archivePath, internalPath, true
}

func normalizeArchivePath(name string) string {
	name = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(name)), "\\", "/")
	if name == "" || name == "." {
		return name
	}
	// Preserve absoluteness: the outermost archive of a nested path (e.g.
	// "/abs/app.war/WEB-INF/lib/foo.jar/Foo.class") is a real on-disk path that
	// must keep its leading slash so the host filesystem can open it. Archive
	// internal entries are slash-trimmed separately in parseJarOrZipPath, and
	// Windows drive paths ("C:/...") have no leading slash, so this only restores
	// the root slash for genuine Unix-absolute paths.
	isAbs := strings.HasPrefix(name, "/")
	name = path.Clean(name)
	name = strings.TrimLeft(name, "/")
	if name == "" {
		return "."
	}
	if isAbs {
		name = "/" + name
	}
	return name
}

func (z *JarFS) getNestedJarFS(jarPath string) (*filesys.UnifiedFS, error) {
	if _, err := z.boundJarReleaseSelection(); err != nil {
		return nil, err
	}
	// 如果禁用了递归解析，返回错误
	if !z.recursiveParse {
		return nil, os.ErrNotExist
	}

	key := z.archiveCacheKey(jarPath)
	if nestedJarFS, ok := z.jarCache.Get(key); ok {
		return nestedJarFS, nil
	}

	if err := z.chargeNestedArchive(); err != nil {
		return nil, err
	}

	jarContent, err := z.readJarEntry(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file: %s", jarPath)
	}
	jarContent = inputkind.StripJMODHeader(jarContent)

	nestedZipFS, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(jarContent), int64(len(jarContent)), z.nestedZipFSOptions()...)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	// 嵌套的jar也继承递归解析设置与共享预算
	nestedJarFS := NewJarFSWithOptions(nestedZipFS, z.recursiveParse)
	nestedJarFS.targetSourceVersion = z.targetSourceVersion
	nestedJarFS.maxAnalysisUpdates = z.maxAnalysisUpdates
	nestedJarFS.sourceCompiler = z.sourceCompiler
	nestedJarFS.retainUnprovedSource = z.retainUnprovedSource
	nestedJarFS.declarationResolver = z.declarationResolver
	if z.archive != nil && z.archive.active {
		nestedJarFS.archive = z.archive.nestedState()
	}
	unifiedFS := filesys.NewUnifiedFS(nestedJarFS,
		filesys.WithUnifiedFsExtMap(".class", ".java"),
	)
	z.jarCache.Set(key, unifiedFS)
	return unifiedFS, nil
}

func (z *JarFS) statInJar(name string) (fs.FileInfo, error) {
	jarPath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	info, err := unifiedFS.Stat(internalPath)
	if err != nil {
		if _, readErr := unifiedFS.ReadDir(internalPath); readErr == nil {
			return &ArchiveFileInfo{
				fs:    info,
				name:  name,
				isDir: true,
			}, nil
		}
		return nil, err
	}

	return &ArchiveFileInfo{
		fs:    info,
		name:  name,
		isDir: info.IsDir(),
	}, nil
}

func (z *JarFS) readDirFromJar(fullPath string) ([]fs.DirEntry, error) {
	jarPath, internalPath, ok := parseArchivePath(fullPath)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	return unifiedFS.ReadDir(internalPath)
}

func (z *JarFS) readFileFromJar(name string) ([]byte, error) {
	jarPath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	return unifiedFS.ReadFile(internalPath)
}

func (z *JarFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if isArchivePath(name) {
		return z.readDirFromJar(name)
	}
	if isArchiveFile(name) {
		info, err := z.ZipFS.Stat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			unifiedFS, err := z.getNestedJarFS(name)
			if err != nil {
				return nil, err
			}
			return unifiedFS.ReadDir(".")
		}
	}
	return z.ZipFS.ReadDir(name)
}

type ExpandedZipFS struct {
	jarReleaseMu        sync.Mutex
	jarReleaseSelection *jarReleaseSelection
	jarReleaseBinding   *filesys.ZipFS

	declarationsMu       sync.Mutex
	declarations         map[string]*archiveDeclarationScope
	underlying           fi.FileSystem
	zipFS                *filesys.ZipFS // optional container (zip root); nil for local directories
	recursiveParse       bool
	retainUnprovedSource bool
	maxAnalysisUpdates   int
	jarCache             *utils.SafeMapWithKey[string, *filesys.UnifiedFS]
	zipCache             *utils.SafeMapWithKey[string, *filesys.ZipFS]
	archive              *archiveFSState
}

var _ fi.FileSystem = (*ExpandedZipFS)(nil)

func NewExpandedZipFS(underlying fi.FileSystem, zipFS *filesys.ZipFS) *ExpandedZipFS {
	return NewExpandedZipFSWithOptions(underlying, zipFS, true)
}

func NewExpandedZipFSWithOptions(underlying fi.FileSystem, zipFS *filesys.ZipFS, recursiveParse bool) *ExpandedZipFS {
	return &ExpandedZipFS{
		underlying:     underlying,
		zipFS:          zipFS,
		recursiveParse: recursiveParse,
		jarCache:       utils.NewSafeMapWithKey[string, *filesys.UnifiedFS](),
		zipCache:       utils.NewSafeMapWithKey[string, *filesys.ZipFS](),
		archive:        archiveStateFromZipFS(zipFS),
	}
}

// SetAnalysisBudget applies a per-method reaching-definition budget to classes
// decompiled directly from this filesystem and to nested archives created
// afterwards. A positive value above 1,000,000 also scales graph caps.
func (e *ExpandedZipFS) SetAnalysisBudget(maxAnalysisUpdates int) {
	if e != nil {
		e.maxAnalysisUpdates = maxAnalysisUpdates
	}
}

func (e *ExpandedZipFS) getArchiveFS(archivePath string) (fi.FileSystem, error) {
	if isArchiveFile(archivePath) {
		return e.GetJarFS(archivePath)
	}
	return nil, os.ErrNotExist
}

func (e *ExpandedZipFS) GetJarFS(jarPath string) (*filesys.UnifiedFS, error) {
	if err := e.archive.checkJarSelectionWork(); err != nil {
		return nil, err
	}
	if e.zipFS != nil {
		if _, err := e.boundJarReleaseSelection(); err != nil {
			return nil, err
		}
	}
	key := e.archiveCacheKey(jarPath)
	if jarFS, ok := e.jarCache.Get(key); ok {
		return jarFS, nil
	}

	if err := e.chargeNestedArchive(); err != nil {
		return nil, err
	}

	jarContent, err := e.readSemanticArchiveBytes(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file: %s", jarPath)
	}
	jarContent = inputkind.StripJMODHeader(jarContent)

	zipFS, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(jarContent), int64(len(jarContent)), e.nestedZipFSOptions()...)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	jarFS := NewJarFSWithOptions(zipFS, e.recursiveParse)
	jarFS.retainUnprovedSource = e.retainUnprovedSource
	jarFS.maxAnalysisUpdates = e.maxAnalysisUpdates
	jarFS.declarationResolver = e.archiveDeclarationResolver(jarPath)
	if e.archive != nil && e.archive.active {
		jarFS.archive = e.archive.nestedState()
	}
	undiFS := filesys.NewUnifiedFS(jarFS,
		filesys.WithUnifiedFsExtMap(".class", ".java"),
	)
	e.jarCache.Set(key, undiFS)
	return undiFS, nil
}

func (e *ExpandedZipFS) getZipFS(zipPath string) (*filesys.ZipFS, error) {
	if err := e.archive.checkJarSelectionWork(); err != nil {
		return nil, err
	}
	if e.zipFS != nil {
		if _, err := e.boundJarReleaseSelection(); err != nil {
			return nil, err
		}
	}
	key := e.archiveCacheKey(zipPath)
	if zipFS, ok := e.zipCache.Get(key); ok {
		return zipFS, nil
	}

	if err := e.chargeNestedArchive(); err != nil {
		return nil, err
	}

	zipContent, err := e.readSemanticArchiveBytes(zipPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read zip file: %s", zipPath)
	}
	zipContent = inputkind.StripJMODHeader(zipContent)

	nestedZipFS, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(zipContent), int64(len(zipContent)), e.nestedZipFSOptions()...)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for nested zip: %s", zipPath)
	}

	e.zipCache.Set(key, nestedZipFS)
	return nestedZipFS, nil
}

func (e *ExpandedZipFS) expandArchive(archivePath string) ([]fs.DirEntry, error) {
	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	entries, err := archiveFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (e *ExpandedZipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if isArchivePath(name) {
		return e.readDirFromArchive(name)
	}
	if isArchiveFile(name) {
		_, err := e.underlying.Stat(name)
		if err != nil {
			return nil, err
		}
		return e.expandArchive(name)
	}
	entries, err := e.underlying.ReadDir(name)
	if err != nil {
		return nil, err
	}
	var expandedEntries []fs.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			expandedEntries = append(expandedEntries, entry)
			continue
		}
		expandedEntries = append(expandedEntries, entry)
	}
	return expandedEntries, nil
}

func (e *ExpandedZipFS) readDirFromArchive(fullPath string) ([]fs.DirEntry, error) {
	archivePath, internalPath, ok := parseArchivePath(fullPath)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	entries, err := archiveFS.ReadDir(internalPath)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (e *ExpandedZipFS) Stat(name string) (fs.FileInfo, error) {
	if isArchivePath(name) {
		return e.statInArchive(name)
	}
	if isArchiveFile(name) {
		info, err := e.underlying.Stat(name)
		if err != nil {
			return nil, err
		}
		return &ArchiveFileInfo{
			fs:    info,
			name:  info.Name(),
			isDir: true,
		}, nil
	}
	return e.underlying.Stat(name)
}

func (e *ExpandedZipFS) statInArchive(name string) (fs.FileInfo, error) {
	archivePath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	info, err := archiveFS.Stat(internalPath)
	if err != nil {
		if _, readErr := archiveFS.ReadDir(internalPath); readErr == nil {
			return &ArchiveFileInfo{
				fs:    info,
				name:  name,
				isDir: true,
			}, nil
		}
		return nil, err
	}

	return &ArchiveFileInfo{
		fs:    info,
		name:  name,
		isDir: info.IsDir(),
	}, nil
}

func (e *ExpandedZipFS) ReadFile(name string) ([]byte, error) {
	if isArchivePath(name) {
		return e.readFileFromArchive(name)
	}
	chosen, err := e.applyMultiRelease(name)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if e.zipFS != nil {
		return e.readSelectedJarEntry(chosen)
	}
	raw, err = e.underlying.ReadFile(chosen)
	if err != nil || !strings.EqualFold(path.Ext(name), ".class") {
		return raw, err
	}
	cf, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	d := NewClassObjectDumper(cf)
	d.options.RetainUnprovedSource = e.retainUnprovedSource
	d.options.MaxAnalysisUpdates = e.maxAnalysisUpdates
	d.Work = largeMethodWorkBudget(e.maxAnalysisUpdates)
	logical := cf.GetClassName() + ".class"
	if normalized := normalizeArchivePath(name); strings.HasSuffix(normalized, logical) {
		root := strings.TrimSuffix(normalized, logical)
		if root == "" {
			root = "."
		}
		d.declarationResolver = func(owner string) ([]byte, bool) {
			return e.resolveDeclarationScope("directory:"+root, e.underlying, root, owner)
		}
	}
	source, err := d.DumpClass()
	return []byte(source), err
}

func (e *ExpandedZipFS) readFileFromArchive(name string) ([]byte, error) {
	archivePath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	if jarFS, ok := archiveFS.(*filesys.UnifiedFS); ok {
		return jarFS.ReadFile(internalPath)
	}
	if zipFS, ok := archiveFS.(*filesys.ZipFS); ok {
		return zipFS.ReadFile(internalPath)
	}
	return nil, os.ErrNotExist
}

func (e *ExpandedZipFS) Join(elem ...string) string {
	return e.underlying.Join(elem...)
}

func (e *ExpandedZipFS) GetSeparators() rune {
	return e.underlying.GetSeparators()
}

func (e *ExpandedZipFS) IsAbs(name string) bool {
	return e.underlying.IsAbs(name)
}

func (e *ExpandedZipFS) Getwd() (string, error) {
	return e.underlying.Getwd()
}

func (e *ExpandedZipFS) Exists(path string) (bool, error) {
	return e.underlying.Exists(path)
}

func (e *ExpandedZipFS) Rename(old string, new string) error {
	return e.underlying.Rename(old, new)
}

func (e *ExpandedZipFS) Rel(base string, target string) (string, error) {
	return e.underlying.Rel(base, target)
}

func (e *ExpandedZipFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return e.underlying.WriteFile(name, data, perm)
}

func (e *ExpandedZipFS) Delete(name string) error {
	return e.underlying.Delete(name)
}

func (e *ExpandedZipFS) MkdirAll(name string, perm os.FileMode) error {
	return e.underlying.MkdirAll(name, perm)
}

func (e *ExpandedZipFS) ExtraInfo(path string) map[string]any {
	return e.underlying.ExtraInfo(path)
}

func (e *ExpandedZipFS) Base(p string) string {
	return e.underlying.Base(p)
}

func (e *ExpandedZipFS) PathSplit(s string) (string, string) {
	return e.underlying.PathSplit(s)
}

func (e *ExpandedZipFS) Ext(s string) string {
	return e.underlying.Ext(s)
}

func (e *ExpandedZipFS) Open(name string) (fs.File, error) {
	if isArchivePath(name) {
		data, err := e.readFileFromArchive(name)
		if err != nil {
			return nil, err
		}
		return memfile.New(data), nil
	}
	chosen, err := e.applyMultiRelease(name)
	if err != nil {
		return nil, err
	}
	if e.zipFS != nil {
		if info, statErr := e.zipFS.Stat(chosen); statErr == nil && info.IsDir() {
			return e.zipFS.Open(chosen)
		}
		raw, readErr := e.readSelectedJarEntry(chosen)
		if readErr != nil {
			return nil, readErr
		}
		return memfile.New(raw), nil
	}
	if strings.EqualFold(path.Ext(name), ".class") {
		data, readErr := e.ReadFile(name)
		if readErr != nil {
			return nil, readErr
		}
		return memfile.New(data), nil
	}
	return e.underlying.Open(chosen)
}

func (e *ExpandedZipFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if e.zipFS == nil && !isArchivePath(name) && !strings.EqualFold(path.Ext(name), ".class") {
		return e.underlying.OpenFile(name, flag, perm)
	}
	return e.Open(name)
}

type ArchiveFileInfo struct {
	fs    fs.FileInfo
	name  string
	isDir bool
}

func (a *ArchiveFileInfo) Name() string {
	return a.fs.Name()
}

func (a *ArchiveFileInfo) Size() int64 {
	return a.fs.Size()
}

func (a *ArchiveFileInfo) Mode() fs.FileMode {
	if a.isDir {
		return fs.ModeDir
	}
	return a.fs.Mode()
}

func (a *ArchiveFileInfo) ModTime() time.Time {
	return a.fs.ModTime()
}

func (a *ArchiveFileInfo) IsDir() bool {
	return a.isDir
}

func (a *ArchiveFileInfo) Sys() interface{} {
	return a.fs.Sys()
}
