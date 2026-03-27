package rag

import (
	"GopherAI/common/mysql"
	"GopherAI/common/redis"
	"GopherAI/dao"
	redisPkg "GopherAI/common/redis"
	"GopherAI/config"
	"context"
	"fmt"
	"os"
	"path/filepath"

	embeddingArk "github.com/cloudwego/eino-ext/components/embedding/ark"
	redisIndexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisRetriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	redisCli "github.com/redis/go-redis/v9"
)

type RAGIndexer struct {
	embedding embedding.Embedder
	indexer   *redisIndexer.Indexer
}

type RAGQuery struct {
	embedding embedding.Embedder
	retriever retriever.Retriever
}

// 构建知识库索引
// 专业说法：文本解析、文本切块、向量化、存储向量
// 通俗理解：把“人能读的文档”，转换成“AI 能按语义搜索的格式”，并存起来
func NewRAGIndexer(filename, embeddingModel string) (*RAGIndexer, error) {

	// 用于控制整个初始化流程（超时 / 取消等），这里先用默认背景即可
	ctx := context.Background()

	// 从环境变量中读取调用向量模型所需的 API Key
	apiKey := os.Getenv("OPENAI_API_KEY")

	// 向量的维度大小（等于向量模型输出的数字个数）
	// Redis 在创建向量索引时必须提前知道这个值
	dimension := config.GetConfig().RagModelConfig.RagDimension

	// 1. 配置并创建“向量生成器”（Embedding）
	// 可以理解为：找一个“翻译官”，
	// 专门负责把文本翻译成 AI 能理解的“向量表示”
	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: config.GetConfig().RagModelConfig.RagBaseUrl, // 向量模型服务地址
		APIKey:  apiKey,                                       // 鉴权信息
		Model:   embeddingModel,                               // 使用哪个向量模型
	}

	// 创建向量生成器实例
	// 后续所有文本的“向量化”都会通过它完成
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// ===============================
	// 2. 初始化 Redis 中的向量索引结构
	// ===============================
	// 可以理解为：先在 Redis 里建好“仓库”，
	// 告诉它以后要存向量，并且每个向量的维度是多少
	if err := redisPkg.InitRedisIndex(ctx, filename, dimension); err != nil {
		return nil, fmt.Errorf("failed to init redis index: %w", err)
	}

	// 获取 Redis 客户端，用于后续数据写入
	rdb := redisPkg.Rdb

	// ===============================
	// 3. 配置索引器（定义：文档如何被存进 Redis）
	// ===============================
	indexerConfig := &redisIndexer.IndexerConfig{
		Client:    rdb,                                     // Redis 客户端
		KeyPrefix: redis.GenerateIndexNamePrefix(filename), // 不同知识库使用不同前缀，避免冲突
		BatchSize: 10,                                      // 批量处理文档，提高写入效率

		// 定义：一段文档（Document）在 Redis 中该如何存储
		DocumentToHashes: func(ctx context.Context, doc *schema.Document) (*redisIndexer.Hashes, error) {

			// 从文档的元数据中取出来源信息（例如文件名、URL）
			source := ""
			if s, ok := doc.MetaData["source"].(string); ok {
				source = s
			}

			// 构造 Redis 中实际存储的数据结构（Hash）
			return &redisIndexer.Hashes{
				// Redis Key，一般由“知识库名 + 文档块 ID”组成
				Key: fmt.Sprintf("%s:%s", filename, doc.ID),

				// Redis Hash 中的字段
				Field2Value: map[string]redisIndexer.FieldValue{
					// content：原始文本内容
					// EmbedKey 表示：该字段需要先做向量化，
					// 生成的向量会存入名为 "vector" 的字段中
					"content": {Value: doc.Content, EmbedKey: "vector"},

					// metadata：一些辅助信息，不参与向量计算
					"metadata": {Value: source},
				},
			}, nil
		},
	}

	// 将“向量生成器”交给索引器
	// 这样索引器在写入文本时，可以自动完成向量计算
	indexerConfig.Embedding = embedder

	// ===============================
	// 4. 创建最终可用的索引器实例
	// ===============================
	// 此时索引器已经具备：
	// - 文本 → 向量 的能力
	// - 向量写入 Redis 的能力
	idx, err := redisIndexer.NewIndexer(ctx, indexerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create indexer: %w", err)
	}

	// 返回一个封装好的 RAGIndexer，
	// 后续只需要调用它，就可以把文档加入知识库
	return &RAGIndexer{
		embedding: embedder,
		indexer:   idx,
	}, nil
}

// IndexFile 读取文件内容并创建向量索引（已升级：支持 chunk 切分）
func (r *RAGIndexer) IndexFile(ctx context.Context, filePath string) error {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 切分成多个 chunk
	chunks := SplitTextIntoChunks(string(content), DefaultChunkConfig())

	// 将每个 chunk 转换为文档
	docs := make([]*schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		doc := &schema.Document{
			ID:      fmt.Sprintf("chunk_%d", i), // chunk ID
			Content: chunk,
			MetaData: map[string]any{
				"source":      filePath,
				"chunk_index": i,
				"total_chunks": len(chunks),
			},
		}
		docs = append(docs, doc)
	}

	// 批量存储文档（会自动进行向量化）
	_, err = r.indexer.Store(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to store documents: %w", err)
	}

	return nil
}

// DeleteIndex 删除指定文件的知识库索引（静态方法，不依赖实例）
func DeleteIndex(ctx context.Context, filename string) error {
	if err := redisPkg.DeleteRedisIndex(ctx, filename); err != nil {
		return fmt.Errorf("failed to delete redis index: %w", err)
	}
	return nil
}

// NewRAGQuery 创建 RAG 查询器（用于向量检索和问答）
// 已升级：从"按目录猜文件"改为"按用户ID查询所有ready文件"
func NewRAGQuery(ctx context.Context, userID int64) (*RAGQuery, error) {
	cfg := config.GetConfig()
	apiKey := os.Getenv("OPENAI_API_KEY")

	// 创建 embedding 模型
	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: cfg.RagModelConfig.RagBaseUrl,
		APIKey:  apiKey,
		Model:   cfg.RagModelConfig.RagEmbeddingModel,
	}
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// TODO: 这里暂时返回基础查询器，后续需要根据用户的ready文件动态构建retriever
	// 第一阶段先保持兼容，等多文件完全落地后再重构检索逻辑
	return &RAGQuery{
		embedding: embedder,
		retriever: nil, // 暂时为空，需要在实际检索时动态创建
	}, nil
}

// RetrieveDocuments 检索相关文档（从用户所有 ready 文件中检索）
func (r *RAGQuery) RetrieveDocuments(ctx context.Context, query string) ([]*schema.Document, error) {
	// 这里暂时使用旧逻辑兼容，后续需要重构为多文件检索
	if r.retriever != nil {
		return r.retriever.Retrieve(ctx, query)
	}
	return nil, fmt.Errorf("retriever not initialized")
}

// RetrieveFromUserFiles 从用户所有 ready 文件中检索文档（新增方法）
func (r *RAGQuery) RetrieveFromUserFiles(ctx context.Context, userID int64, query string) ([]*schema.Document, error) {
	// 1. 查询用户所有 ready 状态的文件
	fileDAO := dao.NewFileDAO(mysql.DB)
	files, err := fileDAO.GetReadyFilesByOwner(ctx, userID)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no ready files found for user")
	}

	// 2. 从所有文件中检索（简化版：只从第一个文件检索）
	// TODO: 后续优化为多文件并行检索并合并结果
	firstFile := files[0]
	storageFileName := filepath.Base(firstFile.StorageKey)

	return r.RetrieveFromFile(ctx, query, storageFileName)
}

// RetrieveFromFile 从指定文件检索文档（新增方法，支持多文件场景）
func (r *RAGQuery) RetrieveFromFile(ctx context.Context, query, storageFileName string) ([]*schema.Document, error) {
	rdb := redisPkg.Rdb
	indexName := redis.GenerateIndexName(storageFileName)

	retrieverConfig := &redisRetriever.RetrieverConfig{
		Client:       rdb,
		Index:        indexName,
		Dialect:      2,
		ReturnFields: []string{"content", "metadata", "distance"},
		TopK:         5,
		VectorField:  "vector",
		DocumentConverter: func(ctx context.Context, doc redisCli.Document) (*schema.Document, error) {
			resp := &schema.Document{
				ID:       doc.ID,
				Content:  "",
				MetaData: map[string]any{},
			}
			for field, val := range doc.Fields {
				if field == "content" {
					resp.Content = val
				} else {
					resp.MetaData[field] = val
				}
			}
			return resp, nil
		},
	}
	retrieverConfig.Embedding = r.embedding

	rtr, err := redisRetriever.NewRetriever(ctx, retrieverConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create retriever: %w", err)
	}

	return rtr.Retrieve(ctx, query)
}

// BuildRAGPrompt 构建包含检索文档的提示词
func BuildRAGPrompt(query string, docs []*schema.Document) string {
	if len(docs) == 0 {
		return query
	}

	contextText := ""
	for i, doc := range docs {
		contextText += fmt.Sprintf("[文档 %d]: %s\n\n", i+1, doc.Content)
	}

	prompt := fmt.Sprintf(`基于以下参考文档回答用户的问题。如果文档中没有相关信息，请说明无法找到相关信息。

参考文档：
%s

用户问题：%s

请提供准确、完整的回答：`, contextText, query)

	return prompt
}
