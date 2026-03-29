package storage

import (
	"GopherAI/config"
	"fmt"
	"strings"
	"sync"
)

var (
	globalStorage Storage
	storageOnce   sync.Once
	storageErr    error
)

// GetStorage returns the configured storage implementation.
// The default remains local storage so a single machine can still run directly.
func GetStorage() (Storage, error) {
	storageOnce.Do(func() {
		cfg := config.GetConfig().StorageConfig
		provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
		switch provider {
		case "", "local":
			basePath := strings.TrimSpace(cfg.BasePath)
			if basePath == "" {
				basePath = "uploads"
			}
			globalStorage = NewLocalStorage(basePath)
		case "minio", "s3", "oss":
			globalStorage, storageErr = NewObjectStorage(cfg)
		default:
			storageErr = fmt.Errorf("unsupported storage provider: %s", cfg.Provider)
		}
	})

	return globalStorage, storageErr
}
