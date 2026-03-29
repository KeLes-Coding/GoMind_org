package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage 是单机默认实现。
// 设计目标很明确：即使没有对象存储，项目也必须能在一台机器上直接运行。
type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) *LocalStorage {
	return &LocalStorage{basePath: basePath}
}

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	// local 模式下依然沿用统一的 storage key 语义，
	// 只是把它映射到 basePath 下的相对文件路径。
	fullPath := filepath.Join(s.basePath, key)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create dir failed: %w", err)
	}

	dst, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, reader)
	return err
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// local provider 没有对象存储直链这一层，因此继续由应用节点做安全流式下载。
	fullPath := filepath.Join(s.basePath, key)
	return os.Open(fullPath)
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(s.basePath, key)
	return os.Remove(fullPath)
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	fullPath := filepath.Join(s.basePath, key)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
