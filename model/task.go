package model

// VectorizeTask 向量化任务
type VectorizeTask struct {
	FileID  string `json:"file_id"`  // 文件ID
	Version int    `json:"version"`  // 文件版本（用于幂等）
}

// 任务队列名称
const (
	QueueVectorize = "file.vectorize" // 向量化任务队列
)
