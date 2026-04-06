package milvusstore

import (
	commonMilvus "GopherAI/common/milvus"
	"GopherAI/common/vectorstore"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

var (
	defaultStore vectorstore.Store
	storeMu      sync.Mutex
)

// Store 是基于 Milvus 的 VectorStore 实现。
// 第一阶段采用单 collection 方案，因此这里只需要维护 collectionName 和向量维度。
type Store struct {
	collectionName string
	dimension      int
}

// NewStore 返回默认的 Milvus VectorStore。
// 这里做了进程内复用，避免在高频查询下重复初始化 collection 和客户端。
func NewStore(ctx context.Context, dimension int) (*Store, error) {
	storeMu.Lock()
	defer storeMu.Unlock()

	if existing, ok := defaultStore.(*Store); ok && existing != nil && existing.dimension == dimension {
		return existing, nil
	}

	store := &Store{
		collectionName: commonMilvus.CollectionName(),
		dimension:      dimension,
	}
	if err := store.EnsureCollection(ctx, dimension); err != nil {
		return nil, err
	}

	defaultStore = store
	return store, nil
}

// EnsureCollection 确保当前 Store 对应的 collection 可用。
func (s *Store) EnsureCollection(ctx context.Context, dimension int) error {
	cli, err := commonMilvus.GetClient(ctx)
	if err != nil {
		return err
	}
	return commonMilvus.EnsureCollection(ctx, cli, dimension)
}

// UpsertDocuments 将标准化 chunk 文档批量写入 Milvus。
// 这里使用 Upsert 而不是 Insert，是为了让 reindex / 重试场景更容易保持幂等。
func (s *Store) UpsertDocuments(ctx context.Context, docs []vectorstore.Document) error {
	if len(docs) == 0 {
		return nil
	}

	cli, err := commonMilvus.GetClient(ctx)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(docs))
	fileIDs := make([]string, 0, len(docs))
	fileVersions := make([]int64, 0, len(docs))
	fileNames := make([]string, 0, len(docs))
	storageKeys := make([]string, 0, len(docs))
	contentSHA256s := make([]string, 0, len(docs))
	chunkIndexes := make([]int64, 0, len(docs))
	totalChunks := make([]int64, 0, len(docs))
	ownerIDs := make([]int64, 0, len(docs))
	kbIDs := make([]string, 0, len(docs))
	statuses := make([]string, 0, len(docs))
	contents := make([]string, 0, len(docs))
	vectors := make([][]float32, 0, len(docs))

	for _, doc := range docs {
		ids = append(ids, doc.ID)
		fileIDs = append(fileIDs, metadataString(doc.MetaData, commonMilvus.FieldFileID))
		fileVersions = append(fileVersions, metadataInt64(doc.MetaData, commonMilvus.FieldFileVersion))
		fileNames = append(fileNames, metadataString(doc.MetaData, commonMilvus.FieldFileName))
		storageKeys = append(storageKeys, metadataString(doc.MetaData, commonMilvus.FieldStorageKey))
		contentSHA256s = append(contentSHA256s, metadataString(doc.MetaData, commonMilvus.FieldContentSHA256))
		chunkIndexes = append(chunkIndexes, metadataInt64(doc.MetaData, commonMilvus.FieldChunkIndex))
		totalChunks = append(totalChunks, metadataInt64(doc.MetaData, commonMilvus.FieldTotalChunks))
		ownerIDs = append(ownerIDs, metadataInt64(doc.MetaData, commonMilvus.FieldOwnerID))
		kbIDs = append(kbIDs, metadataString(doc.MetaData, commonMilvus.FieldKBID))
		statuses = append(statuses, metadataString(doc.MetaData, commonMilvus.FieldStatus))
		contents = append(contents, doc.Content)
		vectors = append(vectors, doc.Vector)
	}

	_, err = cli.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(s.collectionName).
		WithVarcharColumn(commonMilvus.FieldChunkID, ids).
		WithVarcharColumn(commonMilvus.FieldFileID, fileIDs).
		WithInt64Column(commonMilvus.FieldFileVersion, fileVersions).
		WithVarcharColumn(commonMilvus.FieldFileName, fileNames).
		WithVarcharColumn(commonMilvus.FieldStorageKey, storageKeys).
		WithVarcharColumn(commonMilvus.FieldContentSHA256, contentSHA256s).
		WithInt64Column(commonMilvus.FieldChunkIndex, chunkIndexes).
		WithInt64Column(commonMilvus.FieldTotalChunks, totalChunks).
		WithInt64Column(commonMilvus.FieldOwnerID, ownerIDs).
		WithVarcharColumn(commonMilvus.FieldKBID, kbIDs).
		WithVarcharColumn(commonMilvus.FieldStatus, statuses).
		WithVarcharColumn(commonMilvus.FieldContent, contents).
		WithFloatVectorColumn(commonMilvus.FieldVector, s.dimension, vectors))
	if err != nil {
		return fmt.Errorf("upsert documents to milvus failed: %w", err)
	}

	return nil
}

// Search 执行一次基于向量的主检索，并把结果统一转换成项目内部结构。
// 当前只取首个 query vector 的结果集，因为上层 RAG 查询一次只发一个问题向量。
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) ([]vectorstore.SearchResult, error) {
	if len(req.Vector) == 0 {
		return nil, fmt.Errorf("query vector is required")
	}

	cli, err := commonMilvus.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	option := milvusclient.NewSearchOption(s.collectionName, topK, []entity.Vector{entity.FloatVector(req.Vector)}).
		WithANNSField(commonMilvus.FieldVector).
		WithOutputFields(
			commonMilvus.FieldContent,
			commonMilvus.FieldFileID,
			commonMilvus.FieldFileVersion,
			commonMilvus.FieldFileName,
			commonMilvus.FieldStorageKey,
			commonMilvus.FieldContentSHA256,
			commonMilvus.FieldChunkID,
			commonMilvus.FieldChunkIndex,
			commonMilvus.FieldTotalChunks,
			commonMilvus.FieldOwnerID,
			commonMilvus.FieldKBID,
			commonMilvus.FieldStatus,
		)

	if expr := buildFilterExpr(req.Filter); expr != "" {
		option = option.WithFilter(expr)
	}

	resultSets, err := cli.Search(ctx, option)
	if err != nil {
		return nil, fmt.Errorf("search milvus failed: %w", err)
	}
	if len(resultSets) == 0 {
		return nil, nil
	}

	resultSet := resultSets[0]
	results := make([]vectorstore.SearchResult, 0, resultSet.ResultCount)
	for i := 0; i < resultSet.ResultCount; i++ {
		id, err := resultSet.IDs.GetAsString(i)
		if err != nil {
			return nil, fmt.Errorf("read milvus result id failed: %w", err)
		}

		meta := map[string]any{
			commonMilvus.FieldFileID:        columnValue(resultSet, commonMilvus.FieldFileID, i),
			commonMilvus.FieldFileVersion:   columnValue(resultSet, commonMilvus.FieldFileVersion, i),
			commonMilvus.FieldFileName:      columnValue(resultSet, commonMilvus.FieldFileName, i),
			commonMilvus.FieldStorageKey:    columnValue(resultSet, commonMilvus.FieldStorageKey, i),
			commonMilvus.FieldContentSHA256: columnValue(resultSet, commonMilvus.FieldContentSHA256, i),
			commonMilvus.FieldChunkID:       columnValue(resultSet, commonMilvus.FieldChunkID, i),
			commonMilvus.FieldChunkIndex:    columnValue(resultSet, commonMilvus.FieldChunkIndex, i),
			commonMilvus.FieldTotalChunks:   columnValue(resultSet, commonMilvus.FieldTotalChunks, i),
			commonMilvus.FieldOwnerID:       columnValue(resultSet, commonMilvus.FieldOwnerID, i),
			commonMilvus.FieldKBID:          columnValue(resultSet, commonMilvus.FieldKBID, i),
			commonMilvus.FieldStatus:        columnValue(resultSet, commonMilvus.FieldStatus, i),
			"distance":                      scoreToDistance(resultSet.Scores, i),
		}

		results = append(results, vectorstore.SearchResult{
			ID:       id,
			Content:  valueAsString(columnValue(resultSet, commonMilvus.FieldContent, i)),
			Score:    scoreAt(resultSet.Scores, i),
			MetaData: meta,
		})
	}

	return results, nil
}

// DeleteByFileID 删除某个 file_id 对应的全部 chunk。
// 这是第一阶段保住“删除文件 / reindex 不脏召回”的关键治理动作之一。
func (s *Store) DeleteByFileID(ctx context.Context, fileID string) error {
	if strings.TrimSpace(fileID) == "" {
		return fmt.Errorf("file id is required")
	}

	cli, err := commonMilvus.GetClient(ctx)
	if err != nil {
		return err
	}

	_, err = cli.Delete(ctx, milvusclient.NewDeleteOption(s.collectionName).
		WithExpr(fmt.Sprintf(`%s == "%s"`, commonMilvus.FieldFileID, escapeExprValue(fileID))))
	if err != nil {
		return fmt.Errorf("delete milvus documents by file id failed: %w", err)
	}
	return nil
}

// HasFileVersion 用 limit=1 的轻量查询判断某个 file_id + version 是否已正式入库。
func (s *Store) HasFileVersion(ctx context.Context, fileID string, version int) (bool, error) {
	if strings.TrimSpace(fileID) == "" || version <= 0 {
		return false, fmt.Errorf("file id and version are required")
	}

	cli, err := commonMilvus.GetClient(ctx)
	if err != nil {
		return false, err
	}

	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(s.collectionName).
		WithFilter(fmt.Sprintf(`%s == "%s" && %s == %d`,
			commonMilvus.FieldFileID,
			escapeExprValue(fileID),
			commonMilvus.FieldFileVersion,
			version)).
		WithOutputFields(commonMilvus.FieldChunkID).
		WithLimit(1))
	if err != nil {
		return false, fmt.Errorf("query milvus file version failed: %w", err)
	}

	column := rs.GetColumn(commonMilvus.FieldChunkID)
	return column != nil && column.Len() > 0, nil
}

// buildFilterExpr 把项目内部过滤结构翻译成 Milvus 可执行的标量过滤表达式。
func buildFilterExpr(filter vectorstore.SearchFilter) string {
	parts := make([]string, 0, 4)
	if filter.OwnerID > 0 {
		parts = append(parts, fmt.Sprintf(`%s == %d`, commonMilvus.FieldOwnerID, filter.OwnerID))
	}
	if filter.Status != "" {
		parts = append(parts, fmt.Sprintf(`%s == "%s"`, commonMilvus.FieldStatus, escapeExprValue(filter.Status)))
	}
	if filter.KBID != "" {
		parts = append(parts, fmt.Sprintf(`%s == "%s"`, commonMilvus.FieldKBID, escapeExprValue(filter.KBID)))
	}
	if filter.FileID != "" {
		parts = append(parts, fmt.Sprintf(`%s == "%s"`, commonMilvus.FieldFileID, escapeExprValue(filter.FileID)))
	}
	if filter.StorageKey != "" {
		parts = append(parts, fmt.Sprintf(`%s == "%s"`, commonMilvus.FieldStorageKey, escapeExprValue(filter.StorageKey)))
	}
	return strings.Join(parts, " && ")
}

// escapeExprValue 对字符串过滤值做最小必要转义，避免表达式被意外截断。
func escapeExprValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

// columnValue 从 Milvus 结果集中安全读取一个字段值。
func columnValue(resultSet milvusclient.ResultSet, field string, index int) any {
	column := resultSet.GetColumn(field)
	if column == nil || index >= column.Len() {
		return nil
	}
	value, err := column.Get(index)
	if err != nil {
		return nil
	}
	return value
}

// scoreAt 安全读取 Milvus 返回的原始相似度分数。
func scoreAt(scores []float32, index int) float32 {
	if index < 0 || index >= len(scores) {
		return 0
	}
	return scores[index]
}

func scoreToDistance(scores []float32, index int) float64 {
	// Milvus 在 COSINE 指标下返回更高分更相近，这里转换为“越小越好”的统一距离语义，
	// 以保持当前 RAG 层的排序逻辑不必跟底层存储实现耦合。
	return 1 - float64(scoreAt(scores, index))
}

// metadataString / metadataInt64 用于宽松读取 map[string]any 中的字段，
// 让 Milvus 返回类型差异不会扩散到上层 RAG 逻辑里。
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
	case []byte:
		return string(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int:
		return strconv.Itoa(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return fmt.Sprintf("%v", raw)
}

// valueAsString 用于读取 content 等文本字段。
func valueAsString(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprintf("%v", raw)
	}
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
	case int64:
		return value
	case int32:
		return int64(value)
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case float32:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}
