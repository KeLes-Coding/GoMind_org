package storage

import (
	"context"
	"io"
	"time"
)

// Storage 抽象统一了文件本体的核心操作。
// 上层业务只依赖这组语义稳定的能力，而不直接感知底层是本地磁盘还是对象存储。
type Storage interface {
	// Upload 上传文件内容到给定 key。
	Upload(ctx context.Context, key string, reader io.Reader) error
	// Download 返回文件内容流。
	// local provider 会打开本地文件；对象存储 provider 会返回对象读取流。
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete 删除文件本体。
	Delete(ctx context.Context, key string) error
	// Exists 检查文件本体是否存在。
	Exists(ctx context.Context, key string) (bool, error)
}

// DownloadSigner 是“可选能力接口”，不是所有 Storage 实现都必须支持。
// 这样 local 仍然可以保持最简单的单机实现，而对象存储则可以额外暴露预签名下载能力。
type DownloadSigner interface {
	// PresignDownload 生成一个短时有效的下载地址。
	// fileName / contentType 会编码到响应头参数中，便于浏览器按预期下载。
	PresignDownload(ctx context.Context, key string, fileName string, contentType string, expiry time.Duration) (string, error)
}

// PresignedUpload 描述一次直传所需的最小信息。
// 当前对象存储只需要 URL 和 Method 即可，Headers 预留给未来接入更复杂签名方案。
type PresignedUpload struct {
	URL     string
	Method  string
	Headers map[string]string
}

// UploadSigner 是可选能力接口，用于支持“客户端直传对象存储”的上传模式。
// local provider 不需要实现它，只有对象存储 provider 才会暴露这个能力。
type UploadSigner interface {
	// PresignUpload 生成一个短时有效的上传地址。
	// 这里不直接绑定业务参数，只处理给定 key 的对象写入授权。
	PresignUpload(ctx context.Context, key string, expiry time.Duration) (*PresignedUpload, error)
}
