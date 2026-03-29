package rag

import (
	"GopherAI/model"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestDeduplicateDocuments 验证多文件检索结果合并后的去重逻辑。
// 这个测试覆盖两个关键场景：
// 1. 同一个文档 ID 在不同索引结果里重复出现时，只保留一份；
// 2. 如果某些结果没有稳定 ID，则退化为按内容去重。
func TestDeduplicateDocuments(t *testing.T) {
	docs := []*schema.Document{
		{ID: "doc-1", Content: "alpha"},
		{ID: "doc-1", Content: "alpha duplicate"},
		{Content: "beta"},
		{Content: "beta"},
		{ID: "doc-2", Content: "gamma"},
	}

	result := deduplicateDocuments(docs)
	if len(result) != 3 {
		t.Fatalf("expected 3 documents after dedup, got %d", len(result))
	}
	if result[0].ID != "doc-1" {
		t.Fatalf("expected first document to keep original doc-1, got %q", result[0].ID)
	}
	if result[1].Content != "beta" {
		t.Fatalf("expected second document to be beta fallback dedup result, got %q", result[1].Content)
	}
	if result[2].ID != "doc-2" {
		t.Fatalf("expected third document to be doc-2, got %q", result[2].ID)
	}
}

// TestDocumentDistance 验证距离解析逻辑可以兼容多种返回类型。
// 这样即使底层 retriever 在不同版本下把 distance 解析成 string / float / int，
// 上层排序逻辑仍然可以稳定工作。
func TestDocumentDistance(t *testing.T) {
	cases := []struct {
		name string
		doc  *schema.Document
		want float64
	}{
		{
			name: "float64",
			doc:  &schema.Document{MetaData: map[string]any{"distance": 0.25}},
			want: 0.25,
		},
		{
			name: "string",
			doc:  &schema.Document{MetaData: map[string]any{"distance": "0.75"}},
			want: 0.75,
		},
		{
			name: "missing",
			doc:  &schema.Document{MetaData: map[string]any{}},
			want: 1e18,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := documentDistance(tc.doc)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

// TestBuildChunkMeta 验证升级后的 chunk 元数据是否已经完整携带文件资产信息。
// 这能保证后续引用、排障、版本治理至少在写入阶段已经有了可靠的数据基础。
func TestBuildChunkMeta(t *testing.T) {
	indexer := &RAGIndexer{
		fileMeta: &IndexedFileMeta{
			FileID:        "file-123",
			FileVersion:   3,
			FileName:      "resume.md",
			StorageKey:    "user/1/file-123.md",
			ContentSHA256: "abc123",
			OwnerID:       7,
			KBID:          "kb-9",
		},
	}

	meta := indexer.buildChunkMeta("user/1/file-123.md", 2, 5)
	if got := meta["chunk_id"]; got != "file-123:v3:chunk:2" {
		t.Fatalf("expected chunk_id to include file id and version, got %v", got)
	}
	if got := meta["file_name"]; got != "resume.md" {
		t.Fatalf("expected file_name to be preserved, got %v", got)
	}
	if got := meta["owner_id"]; got != int64(7) {
		t.Fatalf("expected owner_id 7, got %v", got)
	}
}

// TestFormatDocumentSource 验证引用来源文本里能带出文件名、片段位置和版本。
// 这样新的提示词拼装逻辑就不再只是“塞内容”，而是已经具备最小可解释性。
func TestFormatDocumentSource(t *testing.T) {
	doc := &schema.Document{
		MetaData: map[string]any{
			"file_name":    "面试题.md",
			"chunk_index":  1,
			"total_chunks": 4,
			"file_version": 2,
		},
	}

	got := formatDocumentSource(doc)
	want := "面试题.md，第 2/4 段，版本 v2"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestBuildRetrieverFilterQuery 验证统一检索入口会把 owner/status/kb 统一翻译成过滤表达式。
// 这能保证 P1 引入共享索引后，权限边界不是靠上层逐文件枚举维持，而是索引层原生收口。
func TestBuildRetrieverFilterQuery(t *testing.T) {
	scope := RetrievalScope{
		OwnerID: 42,
		Status:  "ready",
		KBID:    "kb-1",
	}

	got := buildRetrieverFilterQuery(scope)
	want := "@owner_id:[42 42] @status:{ready} @kb_id:{kb\\-1}"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestBuildSharedIndexDocumentKey 验证共享索引下优先使用 chunk_id 作为稳定文档 key。
// 这样同一份文件重建索引时，key 语义会始终锚定在 file_id + version + chunk_index，而不是依赖文件名。
func TestBuildSharedIndexDocumentKey(t *testing.T) {
	doc := &schema.Document{
		ID: "doc-1",
		MetaData: map[string]any{
			"chunk_id": "file-1:v2:chunk:3",
		},
	}

	got := buildSharedIndexDocumentKey("legacy-name.txt", doc)
	if got != "file-1:v2:chunk:3" {
		t.Fatalf("expected chunk_id based key, got %q", got)
	}
}

// TestFinalizeRetrievedDocuments 验证统一索引和旧索引兜底最终都会复用同一套收口逻辑。
// 这里重点覆盖“先去重、再按距离排序、最后稳定裁剪 TopK”的行为。
func TestFinalizeRetrievedDocuments(t *testing.T) {
	docs := []*schema.Document{
		{ID: "doc-3", MetaData: map[string]any{"distance": 0.7}},
		{ID: "doc-1", MetaData: map[string]any{"distance": 0.2}},
		{ID: "doc-1", MetaData: map[string]any{"distance": 0.1}},
		{ID: "doc-2", MetaData: map[string]any{"distance": 0.5}},
	}

	got := finalizeRetrievedDocuments(docs)
	if len(got) != 3 {
		t.Fatalf("expected 3 documents after finalize, got %d", len(got))
	}
	if got[0].ID != "doc-1" || got[1].ID != "doc-2" || got[2].ID != "doc-3" {
		t.Fatalf("unexpected document order after finalize: %q, %q, %q", got[0].ID, got[1].ID, got[2].ID)
	}
}

// TestBuildActiveFileVersionMap 验证 ready 文件快照会被收口成 file_id -> 当前版本。
// 这样统一检索后续只需要看这一份快照，就能判断某个 chunk 是否仍属于当前有效版本。
func TestBuildActiveFileVersionMap(t *testing.T) {
	files := []*model.FileAsset{
		{ID: "file-1", Version: 2, Status: model.FileStatusReady, KBID: "kb-a"},
		{ID: "file-2", Version: 1, Status: model.FileStatusReady, KBID: ""},
	}

	got := buildActiveFileVersionMap(files)
	if len(got) != 2 {
		t.Fatalf("expected 2 active files, got %d", len(got))
	}
	if got["file-1"].Version != 2 || got["file-1"].KBID != "kb-a" {
		t.Fatalf("unexpected snapshot for file-1: %+v", got["file-1"])
	}
}

// TestFilterRetrievedDocumentsByActiveFiles 验证统一检索结果会被 file_id + version 白名单再次过滤。
// 这能保证即使共享索引里混入旧版本 chunk，最终进入 prompt 的仍然只会是当前版本。
func TestFilterRetrievedDocumentsByActiveFiles(t *testing.T) {
	activeFiles := map[string]ActiveFileVersion{
		"file-1": {Version: 3, Status: model.FileStatusReady, KBID: "kb-1"},
	}

	docs := []*schema.Document{
		{
			ID: "keep",
			MetaData: map[string]any{
				"file_id":      "file-1",
				"file_version": 3,
				"status":       model.FileStatusReady,
				"kb_id":        "kb-1",
			},
		},
		{
			ID: "drop-old-version",
			MetaData: map[string]any{
				"file_id":      "file-1",
				"file_version": 2,
				"status":       model.FileStatusReady,
				"kb_id":        "kb-1",
			},
		},
		{
			ID: "drop-other-file",
			MetaData: map[string]any{
				"file_id":      "file-2",
				"file_version": 1,
				"status":       model.FileStatusReady,
			},
		},
	}

	got := filterRetrievedDocumentsByActiveFiles(docs, activeFiles)
	if len(got) != 1 {
		t.Fatalf("expected 1 filtered document, got %d", len(got))
	}
	if got[0].ID != "keep" {
		t.Fatalf("expected only current version document to remain, got %q", got[0].ID)
	}
}
