package rag

import (
	"GopherAI/common/mysql"
	"GopherAI/common/observability"
	"GopherAI/common/redis"
	redisPkg "GopherAI/common/redis"
	"GopherAI/common/storage"
	"GopherAI/config"
	"GopherAI/dao"
	"GopherAI/model"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	embeddingArk "github.com/cloudwego/eino-ext/components/embedding/ark"
	redisIndexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisRetriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	redisCli "github.com/redis/go-redis/v9"
)

// IndexedFileMeta 描述一份文件资产在 RAG 侧需要下沉到 chunk 的核心身份信息。
// 这轮升级的目标之一，就是让 chunk 不再只知道自己“来自某个文件名”，
// 而是明确知道自己属于哪条 file_asset、哪一个版本、对应什么稳定元数据。
type IndexedFileMeta struct {
	FileID        string
	FileVersion   int
	FileName      string
	StorageKey    string
	ContentSHA256 string
	OwnerID       int64
	KBID          string
}

type RAGIndexer struct {
	embedding embedding.Embedder
	indexer   *redisIndexer.Indexer
	fileMeta  *IndexedFileMeta
}

type RAGQuery struct {
	embedding embedding.Embedder
	retriever retriever.Retriever
	userID    int64
}

// RetrievalScope 描述一次统一检索请求所需要的权限边界。
// 当前先收口 owner_id / status / kb_id 这三个最关键的过滤维度，
// 后续做知识库正式打通时，可以在不改聊天入口的前提下继续往这里扩展。
type RetrievalScope struct {
	OwnerID int64
	Status  string
	KBID    string
}

// ActiveFileVersion 用于描述“当前用户现在允许被召回的那一版文件”。
// 统一索引下同一个 file_id 理论上只该保留当前版本，但为了给迁移期和异常场景兜底，
// 检索层仍然会基于这份快照做一次最终过滤，确保旧版本 chunk 不会漏进 prompt。
type ActiveFileVersion struct {
	Version int
	Status  string
	KBID    string
}

// NewRAGIndexerForFile 直接基于 file_asset 创建索引器。
// 这样 worker 做向量化时，可以把 file_id、version、file_name、sha256 等资产字段
// 一次性下沉到每个 chunk，后续检索、引用和 reindex 排障都更稳定。
func NewRAGIndexerForFile(file *model.FileAsset, embeddingModel string) (*RAGIndexer, error) {
	if file == nil {
		return nil, fmt.Errorf("file asset is required")
	}

	filename := filepath.Base(file.StorageKey)
	indexer, err := NewRAGIndexer(filename, embeddingModel)
	if err != nil {
		return nil, err
	}
	indexer.fileMeta = &IndexedFileMeta{
		FileID:        file.ID,
		FileVersion:   file.Version,
		FileName:      file.FileName,
		StorageKey:    file.StorageKey,
		ContentSHA256: file.SHA256,
		OwnerID:       file.OwnerID,
		KBID:          file.KBID,
	}
	return indexer, nil
}

// NewRAGIndexerWithPermission 创建带权限信息的索引器。
// 这里把 ownerID 和 kbID 挂到索引器上，是为了在切块入库时把权限元数据一并写入 Redis。
func NewRAGIndexerWithPermission(filename, embeddingModel string, ownerID int64, kbID string) (*RAGIndexer, error) {
	indexer, err := NewRAGIndexer(filename, embeddingModel)
	if err != nil {
		return nil, err
	}
	// 这里保留旧入口作为兼容层，避免其它调用点如果暂时没切换会直接失效。
	// 但新的主链路应该优先走 NewRAGIndexerForFile，把完整 file_asset 带进来。
	indexer.fileMeta = &IndexedFileMeta{
		FileName:   filename,
		StorageKey: filename,
		OwnerID:    ownerID,
		KBID:       kbID,
	}
	return indexer, nil
}

// NewRAGIndexer 构建文件索引器。
// 这一步做的事情可以概括为：
// 1. 准备 embedding 模型；
// 2. 初始化 Redis 向量索引；
// 3. 创建一个可以把文本块写入向量库的 indexer。
func NewRAGIndexer(filename, embeddingModel string) (*RAGIndexer, error) {
	ctx := context.Background()
	cfg := config.GetConfig()
	apiKey := strings.TrimSpace(cfg.RagModelConfig.RagEmbeddingAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.OpenAIConfig.APIKey)
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	baseURL := strings.TrimSpace(cfg.RagModelConfig.RagEmbeddingBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.RagModelConfig.RagBaseUrl)
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.OpenAIConfig.BaseURL)
	}
	dimension := cfg.RagModelConfig.RagDimension

	// embedding 模型负责把自然语言文本转换成向量表示。
	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   embeddingModel,
	}
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	// 新写入链路改为落到共享索引中，后续聊天检索就能通过 owner/status/kb 过滤统一召回。
	// 这里仍保留 filename 参与 key 构造，是为了兼容 file_id 缺失的极少数旧入口，避免 key 冲突。
	if err := redisPkg.InitUnifiedRAGIndex(ctx, dimension); err != nil {
		return nil, fmt.Errorf("failed to init unified redis index: %w", err)
	}

	rdb := redisPkg.Rdb
	indexerConfig := &redisIndexer.IndexerConfig{
		Client:    rdb,
		KeyPrefix: redis.GenerateUnifiedRAGIndexPrefix(),
		BatchSize: 10,
		DocumentToHashes: func(ctx context.Context, doc *schema.Document) (*redisIndexer.Hashes, error) {
			source := ""
			if s, ok := doc.MetaData["source"].(string); ok {
				source = s
			}

			return &redisIndexer.Hashes{
				Key: buildSharedIndexDocumentKey(filename, doc),
				Field2Value: map[string]redisIndexer.FieldValue{
					// content 字段会被先做 embedding，再把向量写入 vector 字段。
					"content": {Value: doc.Content, EmbedKey: "vector"},
					// metadata 用来保留来源等非向量化信息，方便检索结果回传时做解释和展示。
					"metadata": {Value: source},
					// 下面这些字段是这轮新增的“文件资产元数据”。
					// 设计目的不是单纯多存几个字段，而是让 chunk 在脱离数据库上下文时
					// 依然能被解释、被引用、被追踪、被按版本治理。
					"file_id":        {Value: normalizeTagValue(metadataString(doc.MetaData, "file_id"))},
					"file_version":   {Value: metadataInt(doc.MetaData, "file_version")},
					"file_name":      {Value: metadataString(doc.MetaData, "file_name")},
					"storage_key":    {Value: metadataString(doc.MetaData, "storage_key")},
					"content_sha256": {Value: normalizeTagValue(metadataString(doc.MetaData, "content_sha256"))},
					"chunk_id":       {Value: normalizeTagValue(metadataString(doc.MetaData, "chunk_id"))},
					"chunk_index":    {Value: metadataInt(doc.MetaData, "chunk_index")},
					"total_chunks":   {Value: metadataInt(doc.MetaData, "total_chunks")},
					"owner_id":       {Value: metadataInt64(doc.MetaData, "owner_id")},
					"kb_id":          {Value: normalizeTagValue(metadataString(doc.MetaData, "kb_id"))},
					"status":         {Value: normalizeTagValue(metadataString(doc.MetaData, "status"))},
				},
			}, nil
		},
	}
	indexerConfig.Embedding = embedder

	idx, err := redisIndexer.NewIndexer(ctx, indexerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create indexer: %w", err)
	}

	return &RAGIndexer{
		embedding: embedder,
		indexer:   idx,
	}, nil
}

// IndexFile 从本地文件路径读取内容并建立索引。
// 这个方法主要保留给仍然以本地文件路径驱动的场景使用。
func (r *RAGIndexer) IndexFile(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	return r.IndexContent(ctx, filePath, content)
}

// IndexReader 支持从任意 reader 建立索引。
// 这对对象存储尤其重要，因为 worker 可以直接读取存储流，而不必先把文件落回本地磁盘。
func (r *RAGIndexer) IndexReader(ctx context.Context, source string, reader io.Reader) error {
	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read file stream: %w", err)
	}
	return r.IndexContent(ctx, source, content)
}

// IndexContent 负责把原始文本切块后写入向量库。
func (r *RAGIndexer) IndexContent(ctx context.Context, source string, content []byte) error {
	chunks := SplitTextIntoChunks(string(content), DefaultChunkConfig())
	docs := make([]*schema.Document, 0, len(chunks))
	for i, chunk := range chunks {
		// 每个 chunk 都携带完整但克制的文件资产元数据。
		// 这样后续无论是回答引用、问题排障还是版本治理，
		// 都可以直接从检索结果里拿到足够的上下文，而不用再额外回表查库。
		chunkMeta := r.buildChunkMeta(source, i, len(chunks))
		doc := &schema.Document{
			ID:       metadataString(chunkMeta, "chunk_id"),
			Content:  chunk,
			MetaData: chunkMeta,
		}
		docs = append(docs, doc)
	}

	if _, err := r.indexer.Store(ctx, docs); err != nil {
		return fmt.Errorf("failed to store documents: %w", err)
	}
	return nil
}

// DeleteIndex 删除指定文件对应的 Redis 向量索引。
func DeleteIndex(ctx context.Context, filename string) error {
	if err := redisPkg.DeleteRedisIndex(ctx, filename); err != nil {
		return fmt.Errorf("failed to delete redis index: %w", err)
	}
	return nil
}

// NewRAGQuery 创建查询器。
// 这次升级把 userID 直接挂进查询器，是为了让上层聊天逻辑继续只调用一个统一入口，
// 而查询器内部则负责决定应该从哪些 ready 文件里检索。
func NewRAGQuery(ctx context.Context, userID int64) (*RAGQuery, error) {
	cfg := config.GetConfig()
	apiKey := strings.TrimSpace(cfg.RagModelConfig.RagEmbeddingAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.OpenAIConfig.APIKey)
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	baseURL := strings.TrimSpace(cfg.RagModelConfig.RagEmbeddingBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.RagModelConfig.RagBaseUrl)
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.OpenAIConfig.BaseURL)
	}

	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   cfg.RagModelConfig.RagEmbeddingModel,
	}
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	return &RAGQuery{
		embedding: embedder,
		retriever: nil,
		userID:    userID,
	}, nil
}

// RetrieveDocuments 是聊天层统一调用的检索入口。
// 当前策略分两步：
// 1. 如果兼容路径提前注入了 retriever，就沿用旧逻辑；
// 2. 否则按当前用户所有 ready 文件做动态多文件检索。
func (r *RAGQuery) RetrieveDocuments(ctx context.Context, query string) ([]*schema.Document, error) {
	if r.retriever != nil {
		return r.retriever.Retrieve(ctx, query)
	}
	return r.RetrieveFromUserFiles(ctx, r.userID, query)
}

// RetrieveFromUserFiles 从用户所有 ready 文件中检索文档。
// 这是把“文件资产系统”真正接进聊天链路的关键：
// 1. 不再只依赖单文件 retriever；
// 2. 不再只看第一份文件；
// 3. 会把多个文件的召回结果合并后统一排序。
func (r *RAGQuery) RetrieveFromUserFiles(ctx context.Context, userID int64, query string) ([]*schema.Document, error) {
	fileDAO := dao.NewFileDAO(mysql.DB)
	// P0/P1 收口后，这里仍然会先查一遍当前 ready 文件列表。
	// 但它的职责已经不是“逐文件创建 retriever”，而是：
	// 1. 给统一检索提供当前版本白名单；
	// 2. 在共享索引缺数据时驱动一次懒迁移/自愈。
	files, err := fileDAO.GetReadyFilesByOwner(ctx, userID)
	if err != nil || len(files) == 0 {
		observability.RecordRAGQuery(false, 0, 0)
		log.Printf("RAG no-hit: user_id=%d reason=no_ready_files db_err=%v", userID, err)
		return nil, fmt.Errorf("no ready files found for user")
	}

	activeFiles := buildActiveFileVersionMap(files)
	scope := RetrievalScope{
		OwnerID: userID,
		Status:  "ready",
	}

	docs, unifiedErr := r.RetrieveFromScope(ctx, query, scope)
	filteredDocs := filterRetrievedDocumentsByActiveFiles(docs, activeFiles)
	if len(filteredDocs) == 0 {
		// 统一索引是 P1 的正式主链路，因此这里不再退回旧的逐文件 retriever。
		// 如果共享索引没覆盖到历史 ready 文件，就直接做一次懒迁移/自愈，然后再重试统一检索。
		//
		// 这样做的收益是：
		// 1. 查询入口保持真正统一；
		// 2. 历史数据会被逐步补入共享索引；
		// 3. 一旦补成功，后续请求就不再需要再走兼容分支。
		repaired, repairErr := EnsureUnifiedIndexCoverage(ctx, files)
		if repairErr != nil {
			log.Printf("RAG unified index self-heal failed: user_id=%d repaired=%d err=%v", userID, repaired, repairErr)
		}
		if repaired > 0 || unifiedErr != nil {
			docs, unifiedErr = r.RetrieveFromScope(ctx, query, scope)
			filteredDocs = filterRetrievedDocumentsByActiveFiles(docs, activeFiles)
		}
	}

	if len(filteredDocs) == 0 {
		observability.RecordRAGQuery(false, 0, 0)
		log.Printf("RAG no-hit: user_id=%d reason=no_retrievable_documents ready_files=%d unified_err=%v", userID, len(files), unifiedErr)
		return nil, fmt.Errorf("no retrievable documents found for user")
	}

	finalDocs := finalizeRetrievedDocuments(filteredDocs)
	hitFileCount := countUniqueHitFiles(finalDocs)
	observability.RecordRAGQuery(true, len(finalDocs), hitFileCount)
	log.Printf("RAG hit: user_id=%d mode=unified hit_chunks=%d hit_files=%d query=%q", userID, len(finalDocs), hitFileCount, query)
	return finalDocs, nil
}

// RetrieveFromScope 通过统一共享索引执行一次带权限边界的检索。
// 这里的“统一”有两个含义：
// 1. 不再为每个 ready 文件单独创建 retriever；
// 2. 过滤条件统一收口在索引层，而不是散落到聊天层和合并层。
func (r *RAGQuery) RetrieveFromScope(ctx context.Context, query string, scope RetrievalScope) ([]*schema.Document, error) {
	filterQuery := buildRetrieverFilterQuery(scope)
	return r.retrieveFromIndex(ctx, query, redis.GenerateUnifiedRAGIndexName(), filterQuery)
}

// RetrieveFromFile 从指定文件对应的索引里检索文档。
func (r *RAGQuery) RetrieveFromFile(ctx context.Context, query, storageFileName string) ([]*schema.Document, error) {
	return r.retrieveFromIndex(ctx, query, redis.GenerateIndexName(storageFileName), "@status:{ready}")
}

func (r *RAGQuery) retrieveFromIndex(ctx context.Context, query, indexName, filterQuery string) ([]*schema.Document, error) {
	rdb := redisPkg.Rdb

	retrieverConfig := &redisRetriever.RetrieverConfig{
		Client:  rdb,
		Index:   indexName,
		Dialect: 2,
		// 这轮把关键文件资产元数据一并带回聊天层。
		// 这样 prompt 引用和后续质量治理就不再需要“先检索，再去数据库反查文件信息”。
		ReturnFields: []string{
			"content",
			"metadata",
			"file_id",
			"file_version",
			"file_name",
			"storage_key",
			"content_sha256",
			"chunk_id",
			"chunk_index",
			"total_chunks",
			"owner_id",
			"kb_id",
			"status",
			"distance",
		},
		TopK:        5,
		VectorField: "vector",
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

	retrieveOptions := make([]retriever.Option, 0, 1)
	if filterQuery != "" {
		retrieveOptions = append(retrieveOptions, redisRetriever.WithFilterQuery(filterQuery))
	}

	docs, err := rtr.Retrieve(ctx, query, retrieveOptions...)
	if err != nil {
		return nil, err
	}

	// 统一索引下虽然已经在查询条件里带了 status，但这里仍保留一次兜底过滤。
	// 原因是旧数据迁移期间，字段类型或历史值可能存在偏差，最终进入 prompt 的结果宁可少一点，也不能把非 ready 文档带进去。
	filtered := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		if metadataString(doc.MetaData, "status") == "ready" {
			filtered = append(filtered, doc)
		}
	}
	return filtered, nil
}

// deduplicateDocuments 用于在多索引结果合并后做一次轻量去重。
// 这样可以避免同一段内容重复进入 prompt，浪费上下文窗口。
func deduplicateDocuments(docs []*schema.Document) []*schema.Document {
	result := make([]*schema.Document, 0, len(docs))
	seen := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		key := doc.ID
		if key == "" {
			key = doc.Content
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, doc)
	}
	return result
}

// finalizeRetrievedDocuments 统一收口“去重、排序、裁剪 TopK”三件事。
// 这样无论结果来自统一索引，还是来自旧的逐文件兜底，最终输出口径都保持一致。
func finalizeRetrievedDocuments(docs []*schema.Document) []*schema.Document {
	finalDocs := deduplicateDocuments(docs)
	sort.SliceStable(finalDocs, func(i, j int) bool {
		return documentDistance(finalDocs[i]) < documentDistance(finalDocs[j])
	})

	const finalTopK = 5
	if len(finalDocs) > finalTopK {
		finalDocs = finalDocs[:finalTopK]
	}
	return finalDocs
}

// documentDistance 从检索元数据里提取距离值，并统一转换成 float64。
// 不同驱动返回的 distance 类型可能不同，这里做一层兜底，避免排序逻辑被类型差异打断。
func documentDistance(doc *schema.Document) float64 {
	if doc == nil || doc.MetaData == nil {
		return 1e18
	}
	raw, ok := doc.MetaData["distance"]
	if !ok {
		return 1e18
	}

	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	}
	return 1e18
}

// BuildRAGPrompt 把检索结果拼成提示词。
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

// BuildRAGPromptWithReferences 是这轮升级新增的提示词拼装入口。
// 它在原始 RAG prompt 的基础上，显式把来源信息和引用要求一起给到模型，
// 这样回答既能尽量基于资料，又能在输出里带上片段编号，提升可解释性。
func BuildRAGPromptWithReferences(query string, docs []*schema.Document) string {
	if len(docs) == 0 {
		return query
	}

	contextText := ""
	for i, doc := range docs {
		contextText += fmt.Sprintf("[参考片段 %d]\n来源：%s\n内容：%s\n\n", i+1, formatDocumentSource(doc), doc.Content)
	}

	return fmt.Sprintf(`请基于下面给出的参考片段回答用户问题。

回答要求：
1. 优先依据参考片段作答，不要编造参考片段中没有的信息。
2. 如果参考片段不足以支撑结论，请明确说明“参考资料中未提及”或“信息不足”。
3. 如果回答直接引用了某个片段的关键信息，请在对应句子后面标注片段编号，例如 [参考片段 2]。
4. 如果多个片段共同支持同一个结论，可以同时引用多个片段。

参考片段：
%s
用户问题：
%s

请给出准确、完整、尽量有引用依据的回答。`, contextText, query)
}

// countUniqueHitFiles 统计当前召回结果涉及了多少唯一文件。
// 这可以帮助我们判断检索到底是“集中命中一份资料”，还是“跨多份资料聚合得到结论”。
func countUniqueHitFiles(docs []*schema.Document) int {
	if len(docs) == 0 {
		return 0
	}

	seen := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		fileID := metadataString(doc.MetaData, "file_id")
		if fileID == "" {
			fileID = metadataString(doc.MetaData, "file_name")
		}
		if fileID == "" {
			continue
		}
		seen[fileID] = struct{}{}
	}
	return len(seen)
}

// buildChunkMeta 统一构建 chunk 元数据。
// 这层封装把“文件资产 -> chunk 元数据”的映射收口到一处，
// 后续继续扩字段、做统一索引、做版本过滤时，改动面会小很多。
func (r *RAGIndexer) buildChunkMeta(source string, chunkIndex, totalChunks int) map[string]any {
	chunkID := fmt.Sprintf("chunk_%d", chunkIndex)
	meta := map[string]any{
		"source":       source,
		"chunk_id":     chunkID,
		"chunk_index":  chunkIndex,
		"total_chunks": totalChunks,
		"status":       "ready",
	}

	if r.fileMeta == nil {
		return meta
	}

	if r.fileMeta.FileID != "" {
		chunkID = fmt.Sprintf("%s:v%d:chunk:%d", r.fileMeta.FileID, r.fileMeta.FileVersion, chunkIndex)
		meta["chunk_id"] = chunkID
	}
	meta["file_id"] = r.fileMeta.FileID
	meta["file_version"] = r.fileMeta.FileVersion
	meta["file_name"] = r.fileMeta.FileName
	meta["storage_key"] = r.fileMeta.StorageKey
	meta["content_sha256"] = r.fileMeta.ContentSHA256
	meta["owner_id"] = r.fileMeta.OwnerID
	meta["kb_id"] = r.fileMeta.KBID
	return meta
}

// formatDocumentSource 把检索结果里的元数据格式化成可直接给模型看的来源说明。
// 文件名、片段位置、版本号都尽量带上，方便模型引用，也方便人工排障。
func formatDocumentSource(doc *schema.Document) string {
	if doc == nil {
		return "未知来源"
	}

	fileName := metadataString(doc.MetaData, "file_name")
	if fileName == "" {
		fileName = metadataString(doc.MetaData, "metadata")
	}
	if fileName == "" {
		fileName = metadataString(doc.MetaData, "source")
	}
	if fileName == "" {
		fileName = "未知文件"
	}

	chunkIndex := metadataInt(doc.MetaData, "chunk_index")
	totalChunks := metadataInt(doc.MetaData, "total_chunks")
	fileVersion := metadataInt(doc.MetaData, "file_version")

	if totalChunks > 0 {
		return fmt.Sprintf("%s，第 %d/%d 段，版本 v%d", fileName, chunkIndex+1, totalChunks, atLeast(fileVersion, 1))
	}
	if fileVersion > 0 {
		return fmt.Sprintf("%s，版本 v%d", fileName, fileVersion)
	}
	return fileName
}

// metadataString / metadataInt / metadataInt64 统一负责宽松读取 MetaData。
// 这样即使旧数据和新数据在 string / int / int64 上有差异，
// 也不会把排序、来源展示和后续过滤逻辑打碎。
func metadataString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return ""
	}

	switch value := raw.(type) {
	case string:
		return value
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", raw)
}

func metadataInt(meta map[string]any, key string) int {
	if meta == nil {
		return 0
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return 0
	}

	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func metadataInt64(meta map[string]any, key string) int64 {
	if meta == nil {
		return 0
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return 0
	}

	switch value := raw.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

// normalizeTagValue 为 Redis TAG 字段提供统一的非空兜底值。
func normalizeTagValue(value string) string {
	if value == "" {
		return "__empty__"
	}
	return value
}

// buildSharedIndexDocumentKey 生成共享索引下每个 chunk 的稳定 hash key。
// 优先使用 chunk_id，是因为它已经带有 file_id + version + chunk_index 语义；
// 如果兼容旧入口时 chunk_id 不够稳定，再退化为 storage_key + doc.ID，避免共享 key 空间冲突。
func buildSharedIndexDocumentKey(filename string, doc *schema.Document) string {
	if doc == nil {
		return filename
	}

	chunkID := metadataString(doc.MetaData, "chunk_id")
	if chunkID != "" {
		return chunkID
	}

	sourceKey := metadataString(doc.MetaData, "storage_key")
	if sourceKey == "" {
		sourceKey = filename
	}
	if doc.ID == "" {
		return sourceKey
	}
	return fmt.Sprintf("%s:%s", sourceKey, doc.ID)
}

// buildRetrieverFilterQuery 把 owner/status/kb 维度统一翻译成 RediSearch 过滤表达式。
// 这样统一检索入口只要拿到 scope，就能直接拼出可执行的权限边界，而不是再依赖外层逐文件枚举。
func buildRetrieverFilterQuery(scope RetrievalScope) string {
	parts := make([]string, 0, 3)
	if scope.OwnerID > 0 {
		parts = append(parts, fmt.Sprintf("@owner_id:[%d %d]", scope.OwnerID, scope.OwnerID))
	}

	if scope.Status != "" {
		status := normalizeTagValue(scope.Status)
		parts = append(parts, fmt.Sprintf("@status:{%s}", escapeRedisTagValue(status)))
	}

	if scope.KBID != "" {
		parts = append(parts, fmt.Sprintf("@kb_id:{%s}", escapeRedisTagValue(normalizeTagValue(scope.KBID))))
	}
	return strings.Join(parts, " ")
}

// buildActiveFileVersionMap 把当前 ready 文件列表收口成 file_id -> 当前版本快照。
// 这一步既是统一检索的版本白名单，也是后续懒迁移检查统一索引覆盖度的依据。
func buildActiveFileVersionMap(files []*model.FileAsset) map[string]ActiveFileVersion {
	result := make(map[string]ActiveFileVersion, len(files))
	for _, file := range files {
		if file == nil || file.ID == "" {
			continue
		}
		result[file.ID] = ActiveFileVersion{
			Version: file.Version,
			Status:  file.Status,
			KBID:    file.KBID,
		}
	}
	return result
}

// filterRetrievedDocumentsByActiveFiles 只保留当前 file_asset 仍然认定为“当前版本”的 chunk。
// 这一步是 P0 的关键收口点：即使共享索引里因为历史遗留或异常状态混入了旧版本数据，
// 只要数据库里的当前版本快照是正确的，最终进入 prompt 的结果就仍然是安全的。
func filterRetrievedDocumentsByActiveFiles(docs []*schema.Document, activeFiles map[string]ActiveFileVersion) []*schema.Document {
	filtered := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		fileID := metadataString(doc.MetaData, "file_id")
		if fileID == "" {
			continue
		}

		active, ok := activeFiles[fileID]
		if !ok {
			continue
		}
		if metadataString(doc.MetaData, "status") != active.Status {
			continue
		}
		if metadataInt(doc.MetaData, "file_version") != active.Version {
			continue
		}
		if active.KBID != "" && metadataString(doc.MetaData, "kb_id") != active.KBID {
			continue
		}
		filtered = append(filtered, doc)
	}
	return filtered
}

// escapeRedisTagValue 对 TAG 查询值做最小必要转义。
// 统一检索现在开始依赖 file_id / kb_id / status 等 TAG 过滤，如果不做转义，
// UUID 连字符、知识库 ID 特殊字符都可能让过滤表达式解析出错。
func escapeRedisTagValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"-", "\\-",
		" ", "\\ ",
		",", "\\,",
		".", "\\.",
		"<", "\\<",
		">", "\\>",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"\"", "\\\"",
		"'", "\\'",
		":", "\\:",
		";", "\\;",
		"!", "\\!",
		"@", "\\@",
		"#", "\\#",
		"$", "\\$",
		"%", "\\%",
		"^", "\\^",
		"&", "\\&",
		"*", "\\*",
		"(", "\\(",
		")", "\\)",
		"+", "\\+",
		"=", "\\=",
		"~", "\\~",
		"/", "\\/",
		"|", "\\|",
	)
	return replacer.Replace(value)
}

// EnsureUnifiedIndexCoverage 确保当前 ready 文件已经被迁移/补入共享索引。
// 它不是每次查询都强制全量重建，而是按 file_id + version 做“缺哪补哪”的自愈：
// 1. 如果共享索引已有当前版本 chunk，直接跳过；
// 2. 如果没有，就从对象存储重读文件并补建共享索引。
func EnsureUnifiedIndexCoverage(ctx context.Context, files []*model.FileAsset) (int, error) {
	repaired := 0
	for _, file := range files {
		if file == nil || file.ID == "" {
			continue
		}

		exists, err := HasIndexedFileVersion(ctx, file.ID, file.Version)
		if err != nil {
			return repaired, err
		}
		if exists {
			continue
		}

		if err := SyncFileToUnifiedIndex(ctx, file); err != nil {
			return repaired, fmt.Errorf("sync file %s to unified index failed: %w", file.ID, err)
		}
		repaired++
	}
	return repaired, nil
}

// HasIndexedFileVersion 检查共享索引里是否已经存在某份文件当前版本的任意一个 chunk。
// 这里用 limit=1 的存在性判断即可，不需要把整份文件的全部 chunk 都查出来。
func HasIndexedFileVersion(ctx context.Context, fileID string, version int) (bool, error) {
	if fileID == "" || version <= 0 {
		return false, fmt.Errorf("file id and version are required")
	}

	query := fmt.Sprintf("@file_id:{%s} @file_version:[%d %d]", escapeRedisTagValue(normalizeTagValue(fileID)), version, version)
	keys, err := redisPkg.SearchDocumentKeysByQuery(ctx, redis.GenerateUnifiedRAGIndexName(), query, 1)
	if err != nil {
		if strings.Contains(err.Error(), "Unknown index name") {
			return false, nil
		}
		return false, fmt.Errorf("search indexed file version failed: %w", err)
	}
	return len(keys) > 0, nil
}

// SyncFileToUnifiedIndex 把当前 file_asset 的最新版本同步到共享索引。
// 它同时承担两类场景：
// 1. worker 正常向量化写入；
// 2. 查询时发现历史 ready 文件还没迁移到共享索引后的懒迁移/自愈。
func SyncFileToUnifiedIndex(ctx context.Context, file *model.FileAsset) error {
	if file == nil {
		return fmt.Errorf("file asset is required")
	}

	// 共享索引模式下，写当前版本前先删除同 file_id 的历史 chunk，
	// 这样无论是 reindex、失败重试，还是迁移自愈，都能保证最终只保留当前一版。
	if err := DeleteIndexedFileDocuments(ctx, file.ID); err != nil {
		return fmt.Errorf("delete old indexed documents failed: %w", err)
	}

	indexer, err := NewRAGIndexerForFile(file, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		return fmt.Errorf("create rag indexer failed: %w", err)
	}

	fileStorage, err := storage.GetStorage()
	if err != nil {
		return fmt.Errorf("get storage failed: %w", err)
	}

	reader, err := fileStorage.Download(ctx, file.StorageKey)
	if err != nil {
		return fmt.Errorf("download file from storage failed: %w", err)
	}
	defer reader.Close()

	if err := indexer.IndexReader(ctx, file.StorageKey, reader); err != nil {
		return fmt.Errorf("index file into unified index failed: %w", err)
	}

	// 旧的按文件索引到这里就不再是主链路了。
	// 同步完共享索引后顺手清理旧索引，可以让 P0 的历史治理逐步自然收敛。
	legacyIndexName := filepath.Base(file.StorageKey)
	if err := DeleteIndex(ctx, legacyIndexName); err != nil {
		log.Printf("Skip deleting legacy per-file index after unified sync: file_id=%s storage=%s err=%v", file.ID, legacyIndexName, err)
	}
	return nil
}

// DeleteIndexedFileDocuments 删除统一共享索引里某个 file_id 对应的全部 chunk 文档。
// 这一步是共享索引模式下避免脏召回的必要配套：删除文件、重建索引前，都要把旧 chunk 主动清掉。
func DeleteIndexedFileDocuments(ctx context.Context, fileID string) error {
	if fileID == "" {
		return fmt.Errorf("file id is required")
	}

	query := fmt.Sprintf("@file_id:{%s}", escapeRedisTagValue(normalizeTagValue(fileID)))
	keys, err := redisPkg.SearchDocumentKeysByQuery(ctx, redis.GenerateUnifiedRAGIndexName(), query, 1000)
	if err != nil {
		if strings.Contains(err.Error(), "Unknown index name") {
			// 共享索引还没建起来时，说明新链路数据尚不存在，此时删除动作直接视为成功。
			return nil
		}
		return fmt.Errorf("search indexed file documents failed: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	if err := redisPkg.DeleteKeys(ctx, keys); err != nil {
		return fmt.Errorf("delete indexed file documents failed: %w", err)
	}
	return nil
}

func atLeast(value, min int) int {
	if value < min {
		return min
	}
	return value
}
