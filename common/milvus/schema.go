package milvus

import (
	"GopherAI/config"
	"strings"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

const (
	// 下面这组字段名是第一阶段在 Milvus 中固定下来的 RAG 主数据字段。
	// 它们尽量与当前 common/rag 中使用的元数据语义保持一致，减少上层改造面。
	FieldChunkID       = "chunk_id"
	FieldFileID        = "file_id"
	FieldFileVersion   = "file_version"
	FieldFileName      = "file_name"
	FieldStorageKey    = "storage_key"
	FieldContentSHA256 = "content_sha256"
	FieldChunkIndex    = "chunk_index"
	FieldTotalChunks   = "total_chunks"
	FieldOwnerID       = "owner_id"
	FieldKBID          = "kb_id"
	FieldStatus        = "status"
	FieldContent       = "content"
	FieldVector        = "vector"
)

// CollectionName 返回当前 RAG 主 collection 名称。
// 若业务配置未显式指定，则回退到默认值 rag_chunks。
func CollectionName() string {
	name := strings.TrimSpace(config.GetConfig().MilvusConfig.Collection)
	if name == "" {
		return "rag_chunks"
	}
	return name
}

// Dimension 返回当前向量维度。
// 第一优先级取 milvusConfig.dimension，第二优先级沿用 rag embedding 的维度配置。
func Dimension() int {
	if dim := config.GetConfig().MilvusConfig.Dimension; dim > 0 {
		return dim
	}
	if dim := config.GetConfig().RagModelConfig.RagDimension; dim > 0 {
		return dim
	}
	return 1024
}

// NewCollectionSchema 构造第一阶段统一使用的 collection schema。
// 当前先采用单 collection 方案，把 chunk 文本、向量和最关键的过滤字段放在同一张表里。
func NewCollectionSchema(dimension int) *entity.Schema {
	return entity.NewSchema().
		WithField(entity.NewField().
			WithName(FieldChunkID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(255).
			WithIsPrimaryKey(true)).
		WithField(entity.NewField().
			WithName(FieldFileID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(64)).
		WithField(entity.NewField().
			WithName(FieldFileVersion).
			WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().
			WithName(FieldFileName).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(512)).
		WithField(entity.NewField().
			WithName(FieldStorageKey).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(1024)).
		WithField(entity.NewField().
			WithName(FieldContentSHA256).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(128)).
		WithField(entity.NewField().
			WithName(FieldChunkIndex).
			WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().
			WithName(FieldTotalChunks).
			WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().
			WithName(FieldOwnerID).
			WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().
			WithName(FieldKBID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(64)).
		WithField(entity.NewField().
			WithName(FieldStatus).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(64)).
		WithField(entity.NewField().
			WithName(FieldContent).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(65535)).
		WithField(entity.NewField().
			WithName(FieldVector).
			WithDataType(entity.FieldTypeFloatVector).
			WithDim(int64(dimension)))
}

// NewCollectionOption 统一封装 collection 创建参数和向量索引配置。
// 第一阶段优先保证主链路可用，因此这里先采用 AutoIndex + COSINE 的保守方案。
func NewCollectionOption(name string, dimension int) milvusclient.CreateCollectionOption {
	schema := NewCollectionSchema(dimension)
	vectorIndex := milvusclient.NewCreateIndexOption(name, FieldVector, index.NewAutoIndex(entity.COSINE)).
		WithIndexName(FieldVector)
	return milvusclient.NewCreateCollectionOption(name, schema).
		WithIndexOptions(vectorIndex)
}
