package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	workingDirOnce sync.Once
	workingDir     string
	loggerDirOnce  sync.Once
	loggerDir      string
	sourceCache    sync.Map
	projectPCCache sync.Map
)

func sourceFromPC(pc uintptr) slog.Source {
	if pc == 0 {
		return slog.Source{}
	}
	if cached, ok := sourceCache.Load(pc); ok {
		return cached.(slog.Source)
	}

	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if frame.PC == 0 {
		return slog.Source{}
	}

	source := slog.Source{
		Function: frame.Function,
		File:     frame.File,
		Line:     frame.Line,
	}
	sourceCache.Store(pc, source)
	return source
}

func sourceLocation(file string, line int) string {
	return sourceLocationForRoot(file, line, cachedWorkingDir())
}

func sourceLocationForRoot(file string, line int, root string) string {
	if file == "" {
		return "unknown"
	}

	file = normalizeSourcePath(file)
	root = normalizeSourceRoot(root)
	if rel, ok := projectRelativePath(file, root); ok {
		file = rel
	} else if index := strings.Index(strings.ToLower(file), "/pkg/mod/"); index >= 0 {
		file = file[index+len("/pkg/mod/"):]
	} else if isAbsoluteSourcePath(file) {
		file = path.Base(file)
	}

	return fmt.Sprintf("%s:%d", file, line)
}

func normalizeSourcePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	unc := strings.HasPrefix(value, "//")
	driveRoot := isWindowsDriveRoot(value)
	value = path.Clean(value)
	if unc && !strings.HasPrefix(value, "//") {
		value = "/" + value
	}
	if driveRoot && !strings.HasSuffix(value, "/") {
		value += "/"
	}
	return value
}

func normalizeSourceRoot(value string) string {
	value = normalizeSourcePath(value)
	if value == "/" || isWindowsDriveRoot(value) {
		return value
	}
	return strings.TrimSuffix(value, "/")
}

func projectRelativePath(file, root string) (string, bool) {
	if root == "" || root == "." || !isAbsoluteSourcePath(file) || !isAbsoluteSourcePath(root) {
		return "", false
	}

	windows := isWindowsSourcePath(file) || isWindowsSourcePath(root) || strings.HasPrefix(file, "//") || strings.HasPrefix(root, "//")
	comparisonFile := file
	comparisonRoot := root
	if windows {
		comparisonFile = strings.ToLower(comparisonFile)
		comparisonRoot = strings.ToLower(comparisonRoot)
	}

	prefix := comparisonRoot
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(comparisonFile, prefix) {
		return "", false
	}
	return strings.TrimPrefix(file[len(root):], "/"), true
}

func isAbsoluteSourcePath(value string) bool {
	return strings.HasPrefix(value, "/") || isWindowsSourcePath(value)
}

func isWindowsSourcePath(value string) bool {
	return len(value) >= 3 && value[1] == ':' && value[2] == '/'
}

func isWindowsDriveRoot(value string) bool {
	return len(value) == 3 && isWindowsSourcePath(value)
}

func componentFromPath(file string) string {
	if file == "" {
		return "unknown"
	}

	dir := filepath.Base(filepath.Dir(file))
	if dir == "." || dir == string(filepath.Separator) {
		return "main"
	}
	return dir
}

func callerPC() uintptr {
	var pcs [32]uintptr
	count := runtime.Callers(2, pcs[:])
	for _, pc := range pcs[:count] {
		if isLoggerSource(sourceFromPC(pc).File) {
			continue
		}
		return pc
	}

	if count == len(pcs) {
		for _, pc := range stackPCsSlow() {
			if !isLoggerSource(sourceFromPC(pc).File) {
				return pc
			}
		}
	}
	if count == 0 {
		return 0
	}
	return pcs[0]
}

func projectCallerPC(original uintptr) uintptr {
	if isProjectPC(original) {
		return original
	}

	var pcs [32]uintptr
	count := runtime.Callers(2, pcs[:])
	for _, pc := range pcs[:count] {
		frame := sourceFromPC(pc)
		if strings.HasSuffix(frame.File, ".go") && isProjectSource(frame.File) && !isLoggerSource(frame.File) {
			return pc
		}
	}
	if count == len(pcs) {
		for _, pc := range stackPCsSlow() {
			frame := sourceFromPC(pc)
			if strings.HasSuffix(frame.File, ".go") && isProjectSource(frame.File) && !isLoggerSource(frame.File) {
				return pc
			}
		}
	}

	return original
}

func isProjectPC(pc uintptr) bool {
	if pc == 0 {
		return false
	}
	if cached, ok := projectPCCache.Load(pc); ok {
		return cached.(bool)
	}
	source := sourceFromPC(pc)
	project := isProjectSource(source.File) && !isLoggerSource(source.File)
	projectPCCache.Store(pc, project)
	return project
}

func stackPCsSlow() []uintptr {
	for size := 64; ; size *= 2 {
		pcs := make([]uintptr, size)
		count := runtime.Callers(3, pcs)
		if count < len(pcs) {
			return pcs[:count]
		}
	}
}

func isProjectSource(file string) bool {
	_, ok := projectRelativePath(normalizeSourcePath(file), normalizeSourceRoot(cachedWorkingDir()))
	return ok
}

func isLoggerSource(file string) bool {
	if file == "" || strings.HasSuffix(file, "_test.go") {
		return false
	}
	return filepath.Clean(filepath.Dir(file)) == cachedLoggerDir()
}

func cachedWorkingDir() string {
	workingDirOnce.Do(func() {
		wd, err := os.Getwd()
		if err == nil {
			workingDir = wd
		}
	})
	return workingDir
}

func cachedLoggerDir() string {
	loggerDirOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if ok {
			loggerDir = filepath.Clean(filepath.Dir(file))
		}
	})
	return loggerDir
}
