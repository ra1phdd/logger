package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	workingDirOnce sync.Once
	workingDir     string
)

func sourceFromPC(pc uintptr) slog.Source {
	if pc == 0 {
		return slog.Source{}
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return slog.Source{}
	}

	file, line := fn.FileLine(pc)
	return slog.Source{
		Function: fn.Name(),
		File:     file,
		Line:     line,
	}
}

func sourceLocation(file string, line int) string {
	if file == "" {
		return "unknown"
	}

	path := file
	if wd := cachedWorkingDir(); wd != "" {
		if rel, err := filepath.Rel(wd, file); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}

	return fmt.Sprintf("%s:%d", filepath.ToSlash(path), line)
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
	for i := 2; i < 20; i++ {
		pc, file, _, ok := runtime.Caller(i)
		if !ok {
			continue
		}
		if isLoggerSource(file) {
			continue
		}
		return pc
	}

	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return 0
	}
	return pc
}

func isLoggerSource(file string) bool {
	path := filepath.ToSlash(file)
	return strings.Contains(path, "/pkg/logger/")
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
