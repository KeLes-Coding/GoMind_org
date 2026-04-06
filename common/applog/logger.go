package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultLogPath      = "logs/gomind.log"
	defaultMaxSizeMB    = 20
	defaultFileModePerm = 0o755

	CategoryAIHelper = "aihelper"
	CategoryMCP      = "mcp"
	CategoryHTTP     = "http"
	CategoryGorm     = "gorm"
)

type Config struct {
	Path      string
	MaxSizeMB int
}

type rotatingFileWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

var (
	setupOnce         sync.Once
	fullWriter        io.Writer = os.Stderr
	userWriter        io.Writer = os.Stdout
	rotateError       error
	userLogger        = log.New(os.Stdout, "", log.LstdFlags)
	categoryWriters   = map[string]io.Writer{}
	categoryLoggers   = map[string]*log.Logger{}
	defaultCategories = []string{CategoryAIHelper, CategoryMCP, CategoryHTTP, CategoryGorm}
)

func Setup(cfg Config) error {
	setupOnce.Do(func() {
		path := cfg.Path
		if path == "" {
			path = defaultLogPath
		}

		maxSizeMB := cfg.MaxSizeMB
		if maxSizeMB <= 0 {
			maxSizeMB = defaultMaxSizeMB
		}

		writer, err := newRotatingFileWriter(path, int64(maxSizeMB)*1024*1024)
		if err != nil {
			rotateError = err
			fullWriter = os.Stderr
			userWriter = os.Stdout
			return
		}

		fullWriter = writer
		userWriter = io.MultiWriter(os.Stdout, writer)
		log.SetOutput(fullWriter)
		log.SetFlags(log.LstdFlags)
		userLogger.SetOutput(userWriter)
		userLogger.SetFlags(log.LstdFlags)

		logDir := filepath.Dir(path)
		for _, category := range defaultCategories {
			categoryPath := filepath.Join(logDir, fmt.Sprintf("%s.log", category))
			categoryWriter, err := newRotatingFileWriter(categoryPath, int64(maxSizeMB)*1024*1024)
			if err != nil {
				if rotateError == nil {
					rotateError = err
				}
				categoryWriters[category] = fullWriter
				categoryLoggers[category] = log.New(fullWriter, "", log.LstdFlags)
				continue
			}

			writer := io.MultiWriter(fullWriter, categoryWriter)
			categoryWriters[category] = writer
			categoryLoggers[category] = log.New(writer, "", log.LstdFlags)
		}
	})

	return rotateError
}

func FullWriter() io.Writer {
	return fullWriter
}

func UserWriter() io.Writer {
	return userWriter
}

func Userf(format string, args ...interface{}) {
	userLogger.Printf(format, args...)
}

func CategoryWriter(category string) io.Writer {
	if writer, ok := categoryWriters[category]; ok {
		return writer
	}
	return fullWriter
}

func UserCategoryWriter(category string) io.Writer {
	if writer, ok := categoryWriters[category]; ok {
		return io.MultiWriter(os.Stdout, writer)
	}
	return userWriter
}

func Categoryf(category, format string, args ...interface{}) {
	if logger, ok := categoryLoggers[category]; ok {
		logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func newRotatingFileWriter(path string, maxBytes int64) (*rotatingFileWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), defaultFileModePerm); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return &rotatingFileWriter{
		path:     path,
		maxBytes: maxBytes,
		file:     file,
		size:     info.Size(),
	}, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.reopen(); err != nil {
			return 0, err
		}
	}

	if w.maxBytes > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.trimOldLogs(len(p)); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFileWriter) trimOldLogs(incomingBytes int) error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}

	existing, err := os.ReadFile(w.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existing = nil
	}

	keepBytes := w.maxBytes - int64(incomingBytes)
	if keepBytes < 0 {
		keepBytes = 0
	}
	if int64(len(existing)) > keepBytes {
		existing = existing[len(existing)-int(keepBytes):]
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	w.file = file
	w.size = 0
	if len(existing) == 0 {
		return nil
	}

	n, err := w.file.Write(existing)
	w.size = int64(n)
	return nil
}

func (w *rotatingFileWriter) reopen() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}

	w.file = file
	w.size = info.Size()
	return nil
}
