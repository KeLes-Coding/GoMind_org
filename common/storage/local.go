package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage 本地磁盘存储实现
type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) *LocalStorage {
	return &LocalStorage{basePath: basePath}
}

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
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
