package storage

import (
	"GopherAI/config"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectStorage 是 S3 兼容对象存储实现。
// 它覆盖 MinIO / S3 / OSS 这类后端，让多实例部署时可以共享同一份文件本体。
type ObjectStorage struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewObjectStorage(cfg config.StorageConfig) (*ObjectStorage, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	bucket := strings.TrimSpace(cfg.Bucket)
	if endpoint == "" {
		return nil, fmt.Errorf("storage endpoint is required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("storage bucket is required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}

	storage := &ObjectStorage{
		client: client,
		bucket: bucket,
		prefix: normalizeObjectPrefix(cfg.ObjectPrefix),
	}

	if cfg.AutoCreate {
		exists, err := client.BucketExists(context.Background(), bucket)
		if err != nil {
			return nil, err
		}
		if !exists {
			if err := client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
				return nil, err
			}
		}
	}

	return storage, nil
}

func normalizeObjectPrefix(prefix string) string {
	trimmed := strings.Trim(strings.TrimSpace(prefix), "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/"
}

func (s *ObjectStorage) objectKey(key string) string {
	trimmed := strings.TrimLeft(strings.TrimSpace(key), "/")
	return s.prefix + trimmed
}

func (s *ObjectStorage) Upload(ctx context.Context, key string, reader io.Reader) error {
	objectKey := s.objectKey(key)
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, reader, -1, minio.PutObjectOptions{})
	return err
}

func (s *ObjectStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	objectKey := s.objectKey(key)
	return s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
}

func (s *ObjectStorage) Delete(ctx context.Context, key string) error {
	objectKey := s.objectKey(key)
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}

func (s *ObjectStorage) Exists(ctx context.Context, key string) (bool, error) {
	objectKey := s.objectKey(key)
	_, err := s.client.StatObject(ctx, s.bucket, objectKey, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
		return false, nil
	}
	return false, err
}

func (s *ObjectStorage) PresignDownload(ctx context.Context, key string, fileName string, contentType string, expiry time.Duration) (string, error) {
	// 对象存储场景下优先生成短时下载直链，
	// 这样客户端直接访问对象存储即可，应用节点不必转发大文件流量。
	objectKey := s.objectKey(key)
	reqParams := make(url.Values)
	if contentType != "" {
		reqParams.Set("response-content-type", contentType)
	}
	if fileName != "" {
		reqParams.Set("response-content-disposition", buildAttachmentDisposition(fileName))
	}

	signedURL, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, expiry, reqParams)
	if err != nil {
		return "", err
	}
	return signedURL.String(), nil
}

func (s *ObjectStorage) PresignUpload(ctx context.Context, key string, expiry time.Duration) (*PresignedUpload, error) {
	// 预签名上传用于“客户端直传对象存储”模式：
	// 1. 应用服务只负责鉴权、写元数据、发签名；
	// 2. 文件流量直接进对象存储，不再穿过应用节点；
	// 3. 更适合大文件和多实例场景。
	objectKey := s.objectKey(key)
	signedURL, err := s.client.PresignedPutObject(ctx, s.bucket, objectKey, expiry)
	if err != nil {
		return nil, err
	}
	return &PresignedUpload{
		URL:     signedURL.String(),
		Method:  "PUT",
		Headers: map[string]string{},
	}, nil
}

func buildAttachmentDisposition(fileName string) string {
	// 使用 RFC 5987 风格的 filename*，避免中文文件名在浏览器下载时乱码。
	return "attachment; filename*=UTF-8''" + url.PathEscape(fileName)
}
