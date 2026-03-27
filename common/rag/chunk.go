package rag

import (
	"strings"
	"unicode/utf8"
)

// ChunkConfig Chunk 切分配置
type ChunkConfig struct {
	ChunkSize    int // 每个 chunk 的字符数
	ChunkOverlap int // chunk 之间的重叠字符数
}

// DefaultChunkConfig 默认配置
func DefaultChunkConfig() *ChunkConfig {
	return &ChunkConfig{
		ChunkSize:    800,  // 约 800 字符
		ChunkOverlap: 100,  // 重叠 100 字符
	}
}

// SplitTextIntoChunks 将文本切分成多个 chunk
// 核心思路：滑动窗口切分，保留 overlap 以保持上下文连贯性
func SplitTextIntoChunks(text string, config *ChunkConfig) []string {
	if config == nil {
		config = DefaultChunkConfig()
	}

	// 按行分割文本（保留段落结构）
	lines := strings.Split(text, "\n")

	var chunks []string
	var currentChunk strings.Builder
	currentSize := 0

	for _, line := range lines {
		lineSize := utf8.RuneCountInString(line)

		// 如果当前 chunk + 新行超过限制，保存当前 chunk
		if currentSize+lineSize > config.ChunkSize && currentSize > 0 {
			chunks = append(chunks, currentChunk.String())

			// 保留 overlap 部分
			overlapText := getLastNChars(currentChunk.String(), config.ChunkOverlap)
			currentChunk.Reset()
			currentChunk.WriteString(overlapText)
			currentSize = utf8.RuneCountInString(overlapText)
		}

		// 添加当前行
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
			currentSize++
		}
		currentChunk.WriteString(line)
		currentSize += lineSize
	}

	// 保存最后一个 chunk
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

// getLastNChars 获取字符串的最后 N 个字符
func getLastNChars(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[len(runes)-n:])
}
