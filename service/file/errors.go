package file

import "errors"

var (
	ErrFileNotFound            = errors.New("file not found")
	ErrPermissionDenied        = errors.New("permission denied")
	ErrDirectUploadUnsupported = errors.New("direct upload unsupported")
	ErrUploadNotCompleted      = errors.New("upload not completed")
	ErrRetryNotAllowed         = errors.New("retry not allowed")
	ErrReindexNotAllowed       = errors.New("reindex not allowed")
)
