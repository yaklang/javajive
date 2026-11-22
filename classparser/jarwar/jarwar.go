package jarwar

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/utils"
)

type JarWar struct {
	// License 存储 JAR/WAR 文件中的许可证信息
	// 通常从 META-INF/LICENSE 文件中读取
	License    string
	ManifestMF JarManifest
	fs         *javaclassparser.JarFS

	bestEffort  bool
	writtenFold map[string]int

	// 存储反编译失败的文件列表
	failedDecompiledFiles map[string]struct{}
	failedFilesLock       sync.RWMutex
}

func NewFromJarFS(fs *javaclassparser.JarFS) (*JarWar, error) {
	// Check jar or war
	if entries, err := fs.ReadDir("WEB-INF/"); err == nil && len(entries) > 0 {
		return &JarWar{
			fs:                    fs,
			failedDecompiledFiles: make(map[string]struct{}),
		}, nil
	} else if manifest, err := fs.ZipFS.ReadFile("META-INF/MANIFEST.MF"); err == nil && len(manifest) > 0 {
		if strings.Contains(string(manifest), "Manifest-Version") {
			result := &JarWar{
				ManifestMF:            ParseJarManifest(string(manifest)),
				fs:                    fs,
				failedDecompiledFiles: make(map[string]struct{}),
			}
			if license, err := fs.ZipFS.ReadFile("META-INF/LICENSE"); err == nil {
				result.License = string(license)
			}

			return result, nil
		}
	}
	// Plain ZIP/JAR/WAR/EAR without standard manifest metadata — still allow decompilation.
	return &JarWar{
		fs:                    fs,
		failedDecompiledFiles: make(map[string]struct{}),
	}, nil
}

func New(compressedFile string) (*JarWar, error) {
	return NewWithAnalysisBudget(compressedFile, 0)
}

// NewWithAnalysisBudget opens an archive with an explicit per-method
// reaching-definition budget. Positive budgets above 1,000,000 opt into the
// large-method graph scaling.
func NewWithAnalysisBudget(compressedFile string, maxAnalysisUpdates int) (*JarWar, error) {
	if utils.GetFirstExistedFile(compressedFile) == "" {
		return nil, fmt.Errorf("file not existed: %s", compressedFile)
	}
	if isJImage(compressedFile) {
		return nil, fmt.Errorf("JImage input is supported by AutoDecompile/DecompileArchive, not New: %s", compressedFile)
	}
	fs, err := javaclassparser.NewJarFSFromLocalWithAnalysisBudget(compressedFile, 0, maxAnalysisUpdates, nil)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create jar fs")
	}
	fsIns, err := NewFromJarFS(fs)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create jarwar")
	}
	return fsIns, nil
}

// Close releases the underlying archive file handle held by the JarFS/ZipFS.
// This is important on Windows, where an open file cannot be deleted by callers.
func (j *JarWar) Close() error {
	if j.fs != nil {
		return j.fs.Close()
	}
	return nil
}

func (j *JarWar) GetStructDump() string {
	return filesys.DumpTreeView(j.fs)
}

type dumpTask struct {
	visiblePath string
	entryPath   string
	outPath     string
	isClass     bool
}

func (j *JarWar) DumpToLocalFileSystem(dir string) error {
	if utils.GetFirstExistedPath(dir) == "" {
		log.Info("output directory not existed, create it, os.MkdirAll")
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return utils.Wrap(err, "os.MkdirAll failed")
		}
	}

	var tasks []dumpTask
	if err := j.collectDumpTasks(dir, ".", &tasks); err != nil {
		return fmt.Errorf("archive file system dump failed: %w", err)
	}
	if len(tasks) == 0 {
		return nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	if len(tasks) <= 1 || workers == 1 {
		for _, task := range tasks {
			if err := j.executeDumpTask(dir, task); err != nil {
				return fmt.Errorf("archive file system dump failed: %w", err)
			}
		}
		return nil
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	var nextIdx atomic.Int64
	var failed atomic.Bool
	var errMu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				if failed.Load() {
					return
				}
				idx := int(nextIdx.Add(1) - 1)
				if idx >= len(tasks) {
					return
				}
				if err := j.executeDumpTask(dir, tasks[idx]); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					failed.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return fmt.Errorf("archive file system dump failed: %w", firstErr)
	}
	return nil
}

func (j *JarWar) recordFailedFile(visiblePath string) {
	j.failedFilesLock.Lock()
	if j.failedDecompiledFiles == nil {
		j.failedDecompiledFiles = make(map[string]struct{})
	}
	j.failedDecompiledFiles[visiblePath] = struct{}{}
	j.failedFilesLock.Unlock()
}

func (j *JarWar) bestEffortOutputPath(rel string) string {
	sanitized := sanitizeBestEffortPath(rel)
	j.failedFilesLock.Lock()
	defer j.failedFilesLock.Unlock()
	if j.writtenFold == nil {
		j.writtenFold = make(map[string]int)
	}
	return disambiguateCaseFoldPath(sanitized, j.writtenFold)
}

func (j *JarWar) collectDumpTasks(root, directory string, tasks *[]dumpTask) error {
	entries, err := j.fs.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read archive directory %q: %w", directory, err)
	}
	for _, entry := range entries {
		entryPath := filepath.ToSlash(filepath.Join(directory, entry.Name()))
		info, statErr := j.fs.Stat(entryPath)
		if statErr != nil {
			if j.bestEffort {
				log.Warnf("stat archive entry %q failed: %v", entryPath, statErr)
				j.recordFailedFile(entryPath)
				continue
			}
			return fmt.Errorf("stat archive entry %q: %w", entryPath, statErr)
		}
		if info.IsDir() {
			outDir := entryPath
			if j.bestEffort {
				outDir = sanitizeBestEffortPath(entryPath)
			}
			target, joinErr := SafeJoin(root, outDir)
			if joinErr != nil {
				if j.bestEffort {
					log.Warnf("resolve archive directory %q failed: %v", entryPath, joinErr)
					j.recordFailedFile(entryPath)
					continue
				}
				return fmt.Errorf("resolve archive directory %q: %w", entryPath, joinErr)
			}
			if mkdirErr := os.MkdirAll(target, 0o755); mkdirErr != nil {
				if j.bestEffort {
					log.Warnf("create archive directory %q failed: %v", entryPath, mkdirErr)
					j.recordFailedFile(entryPath)
					continue
				}
				return fmt.Errorf("create archive directory %q: %w", entryPath, mkdirErr)
			}
			if walkErr := j.collectDumpTasks(root, entryPath, tasks); walkErr != nil {
				return walkErr
			}
			continue
		}

		actualPath := entryPath
		if filepath.Ext(actualPath) == ".java" {
			if _, err := j.fs.ReadRawFile(actualPath); err != nil {
				actualPath = strings.TrimSuffix(actualPath, ".java") + ".class"
			}
		}
		isClass := filepath.Ext(actualPath) == ".class"
		outPath := entryPath
		if isClass {
			outPath = strings.TrimSuffix(actualPath, ".class") + ".java"
		}
		if j.bestEffort {
			outPath = j.bestEffortOutputPath(outPath)
		}
		*tasks = append(*tasks, dumpTask{
			visiblePath: entryPath,
			entryPath:   actualPath,
			outPath:     outPath,
			isClass:     isClass,
		})
	}
	return nil
}

func (j *JarWar) executeDumpTask(root string, task dumpTask) error {
	if task.isClass {
		originalRaw, err := j.fs.ReadRawFile(task.entryPath)
		if err != nil {
			if j.bestEffort {
				log.Warnf("read archive entry %q failed: %v", task.entryPath, err)
				j.recordFailedFile(task.visiblePath)
				return nil
			}
			return fmt.Errorf("read archive entry %q: %w", task.entryPath, err)
		}
		decompiled, err := j.fs.ReadFile(task.entryPath)
		if err != nil {
			j.recordFailedFile(task.visiblePath)
			if j.bestEffort {
				log.Warnf("decompile archive entry %q failed: %v", task.entryPath, err)
				return nil
			}
			return fmt.Errorf("decompile archive entry %q: %w", task.entryPath, err)
		}
		if err := writeFlatFile(root, task.outPath, decompiled); err != nil {
			if j.bestEffort {
				log.Warnf("write archive entry %q failed: %v", task.entryPath, err)
				j.recordFailedFile(task.visiblePath)
				return nil
			}
			return fmt.Errorf("write archive entry %q: %w", task.entryPath, err)
		}
		lines := strings.Count(string(decompiled), "\n") + 1
		log.Infof("Decompiled [%v] - Original size: %d bytes, Decompiled size: %d bytes, Lines of code: %d",
			task.visiblePath, len(originalRaw), len(decompiled), lines)
		return nil
	}

	raw, err := j.fs.ReadRawFile(task.entryPath)
	if err != nil {
		if j.bestEffort {
			log.Warnf("read archive entry %q failed: %v", task.entryPath, err)
			j.recordFailedFile(task.visiblePath)
			return nil
		}
		return fmt.Errorf("read archive entry %q: %w", task.entryPath, err)
	}
	if err := writeFlatFile(root, task.outPath, raw); err != nil {
		if j.bestEffort {
			log.Warnf("write archive entry %q failed: %v", task.entryPath, err)
			j.recordFailedFile(task.visiblePath)
			return nil
		}
		return fmt.Errorf("write archive entry %q: %w", task.entryPath, err)
	}
	return nil
}

func writeFlatFile(root, name string, data []byte) error {
	target, err := SafeJoin(root, name)
	if err != nil {
		return err
	}
	parent, err := SafeJoin(root, filepath.ToSlash(filepath.Dir(name)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func sanitizeBestEffortPath(rel string) string {
	slash := strings.ReplaceAll(filepath.ToSlash(rel), "\\", "/")
	slash = strings.TrimPrefix(slash, "./")
	parts := strings.Split(slash, "/")
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg == "" || seg == "." || seg == ".." {
			out = append(out, seg)
			continue
		}
		out = append(out, sanitizeBestEffortSegment(seg))
	}
	return strings.Join(out, "/")
}

func sanitizeBestEffortSegment(seg string) string {
	var b strings.Builder
	b.Grow(len(seg))
	for _, r := range seg {
		if r < 0x20 || strings.ContainsRune(`<>:"|?*`, r) {
			fmt.Fprintf(&b, "_u%04x", r)
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	if len(s) > 0 && (s[len(s)-1] == '.' || s[len(s)-1] == ' ') {
		end := len(s)
		for end > 0 && (s[end-1] == '.' || s[end-1] == ' ') {
			end--
		}
		var tb strings.Builder
		tb.WriteString(s[:end])
		for i := end; i < len(s); i++ {
			fmt.Fprintf(&tb, "_u%04x", s[i])
		}
		s = tb.String()
	}
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	stem := s
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		stem = s[:dot]
	}
	if isWindowsReservedDeviceName(stem) {
		if dot := strings.IndexByte(s, '.'); dot >= 0 {
			s = stem + "_" + s[dot:]
		} else {
			s += "_"
		}
	}
	return s
}

func isWindowsReservedDeviceName(stem string) bool {
	if stem == "" {
		return false
	}
	upper := strings.ToUpper(stem)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		d := upper[3]
		return d >= '1' && d <= '9'
	}
	return false
}

func disambiguateCaseFoldPath(rel string, used map[string]int) string {
	if used == nil {
		return rel
	}
	lower := strings.ToLower(rel)
	if used[lower] == 0 {
		used[lower] = 1
		return rel
	}
	dir, base := path.Split(rel)
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s__case%d/%s", dir, n, base)
		candLower := strings.ToLower(candidate)
		if used[candLower] == 0 {
			used[candLower] = 1
			return candidate
		}
	}
}
