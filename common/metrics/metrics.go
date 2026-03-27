package metrics

import (
	"sync/atomic"
)

// FileMetrics 文件模块监控指标
type FileMetrics struct {
	TotalUploaded   atomic.Int64 // 总上传数
	TotalVectorized atomic.Int64 // 总向量化成功数
	TotalFailed     atomic.Int64 // 总失败数
	Uploading       atomic.Int64 // 当前上传中
	Vectorizing     atomic.Int64 // 当前向量化中
}

var File = &FileMetrics{}

// IncrUploaded 增加上传计数
func (m *FileMetrics) IncrUploaded() {
	m.TotalUploaded.Add(1)
}

// IncrVectorized 增加向量化成功计数
func (m *FileMetrics) IncrVectorized() {
	m.TotalVectorized.Add(1)
}

// IncrFailed 增加失败计数
func (m *FileMetrics) IncrFailed() {
	m.TotalFailed.Add(1)
}

// SetUploading 设置上传中数量
func (m *FileMetrics) SetUploading(n int64) {
	m.Uploading.Store(n)
}

// SetVectorizing 设置向量化中数量
func (m *FileMetrics) SetVectorizing(n int64) {
	m.Vectorizing.Store(n)
}

// Snapshot 获取当前指标快照
func (m *FileMetrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"total_uploaded":   m.TotalUploaded.Load(),
		"total_vectorized": m.TotalVectorized.Load(),
		"total_failed":     m.TotalFailed.Load(),
		"uploading":        m.Uploading.Load(),
		"vectorizing":      m.Vectorizing.Load(),
	}
}
