package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestEnrichDeepOfficeCitations(t *testing.T) {
	tmpDir := t.TempDir()
	bindingsPath := filepath.Join(tmpDir, "evidence-bindings.json")
	bindings := map[string]interface{}{
		"bindings": []map[string]interface{}{
			{
				"binding_id":        "binding-1",
				"knowledge_id":      "knowledge-1",
				"knowledge_base_id": "kb-1",
				"weknora_chunk_id":  "chunk-1",
				"source_document": map[string]interface{}{
					"title":       "BMT설명회자료-260513",
					"uri":         "nas://Downloads/BMT설명회자료-260513.pdf",
					"checksum":    "sha256:document",
					"source_type": "nas_document",
				},
				"source_locator": map[string]interface{}{
					"start_at": 0,
					"end_at":   100,
				},
				"parser_grounding": map[string]interface{}{
					"parser_provider": "deep_parser",
					"parser_run_id":   "run-1",
					"elements": []map[string]interface{}{
						{
							"element_id":   "p1-e1",
							"page":         1,
							"layout_order": 0,
							"bbox":         []float64{0.1, 0.2, 0.3, 0.4},
						},
					},
					"page_images": []map[string]interface{}{
						{
							"page":        1,
							"url":         "local://7/page-1.png",
							"storage_url": "local://7/page-1.png",
						},
					},
				},
				"source_text_hash": "sha256:source-1",
				"chunk_text_hash":  "sha256:chunk-1",
			},
			{
				"binding_id":        "binding-2",
				"knowledge_id":      "knowledge-1",
				"knowledge_base_id": "kb-1",
				"weknora_chunk_id":  "chunk-2",
				"source_document": map[string]interface{}{
					"title":       "BMT설명회자료-260513",
					"uri":         "nas://Downloads/BMT설명회자료-260513.pdf",
					"checksum":    "sha256:document",
					"source_type": "nas_document",
				},
				"source_locator": map[string]interface{}{
					"start_at": 90,
					"end_at":   180,
				},
				"parser_grounding": map[string]interface{}{
					"parser_provider": "deep_parser",
					"parser_run_id":   "run-1",
					"elements": []map[string]interface{}{
						{
							"element_id":   "p2-e1",
							"page":         2,
							"layout_order": 0,
							"bbox":         []float64{0.5, 0.6, 0.7, 0.8},
						},
					},
					"page_images": []map[string]interface{}{
						{
							"page":        2,
							"url":         "local://7/page-2.png",
							"storage_url": "local://7/page-2.png",
						},
					},
				},
				"source_text_hash": "sha256:source-2",
				"chunk_text_hash":  "sha256:chunk-2",
			},
		},
	}
	raw, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("marshal bindings: %v", err)
	}
	if err := os.WriteFile(bindingsPath, raw, 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	t.Setenv(deepOfficeEvidenceBindingsEnv, bindingsPath)
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	result := &types.SearchResult{
		ID:              "chunk-1",
		SubChunkID:      []string{"chunk-2"},
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
		GraphEvidence: map[string]interface{}{
			"match_type":      "graph",
			"source_chunk_id": "chunk-1",
			"paths": []map[string]interface{}{
				{
					"path": []map[string]string{
						{"node": "BMT"},
						{"relation": "평가"},
						{"node": "인식정확도"},
					},
				},
			},
		},
	}
	enrichDeepOfficeCitations(context.Background(), []*types.SearchResult{result})

	card := result.DeepOfficeCitation
	if card == nil {
		t.Fatal("expected citation card")
	}
	if got := card["evidence_status"]; got != "complete" {
		t.Fatalf("expected complete evidence status, got %v", got)
	}

	sourceDocument := card["source_document"].(map[string]interface{})
	if got := sourceDocument["uri"]; got != "nas://Downloads/BMT설명회자료-260513.pdf" {
		t.Fatalf("unexpected source uri: %v", got)
	}

	sourceLocator := card["source_locator"].(map[string]interface{})
	if got := sourceLocator["start_at"]; got != 0 {
		t.Fatalf("unexpected start_at: %v", got)
	}
	if got := sourceLocator["end_at"]; got != 180 {
		t.Fatalf("unexpected end_at: %v", got)
	}

	parserGrounding := card["parser_grounding"].(map[string]interface{})
	pages := parserGrounding["pages"].([]int)
	if len(pages) != 2 || pages[0] != 1 || pages[1] != 2 {
		t.Fatalf("unexpected pages: %v", pages)
	}
	elements := parserGrounding["elements"].([]map[string]interface{})
	if len(elements) != 2 {
		t.Fatalf("expected 2 parser elements, got %d", len(elements))
	}
	bboxes := parserGrounding["bboxes"].([]map[string]interface{})
	if len(bboxes) != 2 {
		t.Fatalf("expected 2 bboxes, got %d", len(bboxes))
	}
	pageImages := parserGrounding["page_images"].([]map[string]interface{})
	if len(pageImages) != 2 {
		t.Fatalf("expected 2 page images, got %d", len(pageImages))
	}

	chunkEvidence := card["chunk_evidence"].([]map[string]interface{})
	if len(chunkEvidence) != 2 {
		t.Fatalf("expected 2 chunk evidence rows, got %d", len(chunkEvidence))
	}
	if got := chunkEvidence[0]["chunk_id"]; got != "chunk-1" {
		t.Fatalf("unexpected chunk evidence id: %v", got)
	}
	if got := chunkEvidence[0]["content_preview"]; got != "" {
		t.Fatalf("expected primary chunk content preview to be empty in this fixture, got %v", got)
	}
	chunkPages := chunkEvidence[1]["pages"].([]int)
	if len(chunkPages) != 1 || chunkPages[0] != 2 {
		t.Fatalf("unexpected chunk pages: %v", chunkPages)
	}

	graphEvidence := card["graph_evidence"].(map[string]interface{})
	if got := graphEvidence["match_type"]; got != "graph" {
		t.Fatalf("unexpected graph evidence match type: %v", got)
	}
	paths := graphEvidence["paths"].([]map[string]interface{})
	if len(paths) != 1 {
		t.Fatalf("expected 1 graph path, got %d", len(paths))
	}
}

func TestEnrichDeepOfficeCitationsDerivesSummaryFromParentBinding(t *testing.T) {
	tmpDir := t.TempDir()
	bindingsPath := filepath.Join(tmpDir, "evidence-bindings.json")
	bindings := map[string]interface{}{
		"bindings": []map[string]interface{}{
			{
				"binding_id":        "binding-parent",
				"knowledge_id":      "knowledge-1",
				"knowledge_base_id": "kb-1",
				"weknora_chunk_id":  "chunk-1",
				"source_document": map[string]interface{}{
					"title":       "BMT설명회자료-260513",
					"uri":         "nas://Downloads/BMT설명회자료-260513.pdf",
					"checksum":    "sha256:document",
					"source_type": "nas_document",
				},
				"source_locator": map[string]interface{}{
					"start_at":   0,
					"end_at":     100,
					"chunk_type": "text",
				},
				"parser_grounding": map[string]interface{}{
					"parser_provider": "deep_parser",
					"parser_run_id":   "run-1",
					"elements": []map[string]interface{}{
						{
							"element_id":   "p1-e1",
							"page":         1,
							"layout_order": 0,
							"bbox":         []float64{0.1, 0.2, 0.3, 0.4},
						},
					},
				},
				"source_text_hash": "sha256:source-1",
				"chunk_text_hash":  "sha256:chunk-1",
			},
		},
	}
	raw, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("marshal bindings: %v", err)
	}
	if err := os.WriteFile(bindingsPath, raw, 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	t.Setenv(deepOfficeEvidenceBindingsEnv, bindingsPath)
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	result := &types.SearchResult{
		ID:              "summary-1",
		Content:         "# Summary\nBMT 주요 평가 항목 요약",
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
		ChunkIndex:      7,
		ChunkType:       types.ChunkTypeSummary,
		ParentChunkID:   "chunk-1",
	}
	enrichDeepOfficeCitations(context.Background(), []*types.SearchResult{result})

	card := result.DeepOfficeCitation
	if card == nil {
		t.Fatal("expected citation card")
	}
	if got := card["evidence_status"]; got != "complete" {
		t.Fatalf("expected complete evidence status, got %v", got)
	}
	chunkEvidence := card["chunk_evidence"].([]map[string]interface{})
	if len(chunkEvidence) != 1 {
		t.Fatalf("expected 1 chunk evidence row, got %d", len(chunkEvidence))
	}
	if got := chunkEvidence[0]["chunk_id"]; got != "summary-1" {
		t.Fatalf("unexpected derived chunk id: %v", got)
	}
	if got := chunkEvidence[0]["parent_chunk_id"]; got != "chunk-1" {
		t.Fatalf("unexpected parent chunk id: %v", got)
	}
	if got := chunkEvidence[0]["is_synthetic"]; got != true {
		t.Fatalf("expected synthetic evidence marker, got %v", got)
	}
	sourceLocator := card["source_locator"].(map[string]interface{})
	if got := sourceLocator["chunk_type"]; got != types.ChunkTypeSummary {
		t.Fatalf("unexpected derived chunk type: %v", got)
	}
}

func TestEnrichDeepOfficeCitationsFromChunkMetadataWithoutEnv(t *testing.T) {
	t.Setenv(deepOfficeEvidenceBindingsEnv, "")
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	chunkMetadata := mustJSON(t, map[string]interface{}{
		deepOfficeChunkMetadataKey: map[string]interface{}{
			"binding_id":        "binding-inline",
			"knowledge_id":      "knowledge-1",
			"knowledge_base_id": "kb-1",
			"weknora_chunk_id":  "chunk-1",
			"chunk_index":       3,
			"chunk_version":     1,
			"source_document": map[string]interface{}{
				"title":       "업로드 문서",
				"uri":         "nas://manual/upload.pdf",
				"checksum":    "sha256:document",
				"source_type": "nas_document",
			},
			"source_locator": map[string]interface{}{
				"start_at":   10,
				"end_at":     30,
				"chunk_type": "text",
			},
			"parser_grounding": map[string]interface{}{
				"parser_provider": "deep_parser",
				"parser_run_id":   "run-inline",
				"elements": []map[string]interface{}{
					{"element_id": "p2-e1", "page": 2, "layout_order": 0, "bbox": []float64{0.2, 0.3, 0.4, 0.5}},
				},
			},
			"source_text_hash": "sha256:source-inline",
			"chunk_text_hash":  "sha256:chunk-inline",
		},
	})

	result := &types.SearchResult{
		ID:              "chunk-1",
		Content:         "BMT 인식정확도 평가",
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
		ChunkMetadata:   chunkMetadata,
	}
	enrichDeepOfficeCitations(context.Background(), []*types.SearchResult{result})

	card := result.DeepOfficeCitation
	if card == nil {
		t.Fatal("expected citation card")
	}
	if got := card["evidence_status"]; got != "complete" {
		t.Fatalf("expected complete evidence status, got %v", got)
	}
	sourceDocument := card["source_document"].(map[string]interface{})
	if got := sourceDocument["uri"]; got != "nas://manual/upload.pdf" {
		t.Fatalf("unexpected source uri: %v", got)
	}
	parserGrounding := card["parser_grounding"].(map[string]interface{})
	pages := parserGrounding["pages"].([]int)
	if len(pages) != 1 || pages[0] != 2 {
		t.Fatalf("unexpected pages: %v", pages)
	}
}

func TestEnrichDeepOfficeCitationsFromChunkMetadataWhenBindingsFileMissing(t *testing.T) {
	t.Setenv(deepOfficeEvidenceBindingsEnv, filepath.Join(t.TempDir(), "missing-evidence-bindings.json"))
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	chunkMetadata := mustJSON(t, map[string]interface{}{
		deepOfficeChunkMetadataKey: map[string]interface{}{
			"knowledge_id":      "knowledge-1",
			"knowledge_base_id": "kb-1",
			"weknora_chunk_id":  "chunk-1",
			"source_document": map[string]interface{}{
				"title":       "업로드 문서",
				"uri":         "nas://manual/upload.pdf",
				"checksum":    "sha256:document",
				"source_type": "nas_document",
			},
			"source_locator": map[string]interface{}{"start_at": 0, "end_at": 20},
			"parser_grounding": map[string]interface{}{
				"parser_provider": "deep_parser",
				"parser_run_id":   "run-inline",
				"elements": []map[string]interface{}{
					{"element_id": "p1-e1", "page": 1, "bbox": []float64{0.1, 0.2, 0.3, 0.4}},
				},
			},
		},
	})

	result := &types.SearchResult{
		ID:              "chunk-1",
		Content:         "BMT 인식정확도 평가",
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
		ChunkMetadata:   chunkMetadata,
	}
	enrichDeepOfficeCitations(context.Background(), []*types.SearchResult{result})

	card := result.DeepOfficeCitation
	if card == nil {
		t.Fatal("expected citation card from inline metadata despite missing bindings file")
	}
	if got := card["evidence_status"]; got != "complete" {
		t.Fatalf("expected complete evidence status, got %v", got)
	}
}

func TestHydrateDeepOfficeCitationFallsBackToSharedChunkLookup(t *testing.T) {
	t.Setenv(deepOfficeEvidenceBindingsEnv, "")
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	chunkMetadata := mustJSON(t, map[string]interface{}{
		deepOfficeChunkMetadataKey: map[string]interface{}{
			"binding_id":        "binding-shared",
			"knowledge_id":      "knowledge-shared",
			"knowledge_base_id": "kb-shared",
			"weknora_chunk_id":  "chunk-shared",
			"chunk_index":       10,
			"source_document": map[string]interface{}{
				"title":       "부산AI 한국딥러닝 전처리 수행 계획서",
				"uri":         "nas://busan-ai/plan.pdf",
				"checksum":    "sha256:document-shared",
				"source_type": "nas_document",
			},
			"source_locator": map[string]interface{}{
				"start_at":    400,
				"end_at":      540,
				"chunk_index": 10,
			},
			"parser_grounding": map[string]interface{}{
				"parser_provider": "deep_parser",
				"parser_run_id":   "run-shared",
				"elements": []map[string]interface{}{
					{
						"element_id":   "p4-e2",
						"page":         4,
						"layout_order": 2,
						"bbox":         []float64{0.1, 0.2, 0.3, 0.4},
					},
				},
				"page_images": []map[string]interface{}{
					{
						"page":        4,
						"url":         "local://shared/page-4.png",
						"storage_url": "local://shared/page-4.png",
					},
				},
			},
			"source_text_hash": "sha256:source-shared",
			"chunk_text_hash":  "sha256:chunk-shared",
		},
	})

	repo := &fakeDeepOfficeChunkLookupRepository{
		sharedChunks: []*types.Chunk{
			{
				ID:              "chunk-shared",
				TenantID:        77,
				KnowledgeID:     "knowledge-shared",
				KnowledgeBaseID: "kb-shared",
				Content:         "부산광역시 BUSAN METROPOLITAN CITY 작업 현황",
				ChunkIndex:      10,
				ChunkType:       types.ChunkTypeText,
				Metadata:        chunkMetadata,
			},
		},
	}
	store := &deepOfficeCitationStore{byChunkID: map[string]*deepOfficeEvidenceBinding{}}
	result := &types.SearchResult{
		ID:              "chunk-shared",
		KnowledgeID:     "knowledge-shared",
		KnowledgeBaseID: "kb-shared",
		ChunkIndex:      10,
		Content:         "부산광역시 BUSAN METROPOLITAN CITY 작업 현황",
	}

	hydrateDeepOfficeCitationStoreFromChunks(context.Background(), store, []*types.SearchResult{result}, repo, 1)
	enrichDeepOfficeCitationsWithStore([]*types.SearchResult{result}, store)

	if repo.scopedCalls != 1 {
		t.Fatalf("expected tenant-scoped lookup first, got %d", repo.scopedCalls)
	}
	if repo.sharedCalls != 1 {
		t.Fatalf("expected shared fallback lookup, got %d", repo.sharedCalls)
	}
	card := result.DeepOfficeCitation
	if card == nil {
		t.Fatal("expected citation card from shared chunk metadata")
	}
	parserGrounding := card["parser_grounding"].(map[string]interface{})
	pages := parserGrounding["pages"].([]int)
	if len(pages) != 1 || pages[0] != 4 {
		t.Fatalf("expected page 4 grounding, got %v", pages)
	}
	pageImages := parserGrounding["page_images"].([]map[string]interface{})
	if len(pageImages) != 1 {
		t.Fatalf("expected page image grounding, got %d", len(pageImages))
	}
}

func TestAddDeepOfficeChunkBindingsCompletesMergedInlineSubChunks(t *testing.T) {
	store := &deepOfficeCitationStore{byChunkID: map[string]*deepOfficeEvidenceBinding{}}

	titleMetadata := mustJSON(t, map[string]interface{}{
		deepOfficeChunkMetadataKey: map[string]interface{}{
			"knowledge_id":      "knowledge-1",
			"knowledge_base_id": "kb-1",
			"weknora_chunk_id":  "chunk-title",
			"chunk_index":       0,
			"source_document": map[string]interface{}{
				"title":       "BMT 설명회 자료",
				"uri":         "local://bmt.pdf",
				"source_type": "nas_document",
			},
			"source_locator": map[string]interface{}{"start_at": 0, "end_at": 23, "chunk_index": 0},
			"parser_grounding": map[string]interface{}{
				"parser_provider": "deep_parser",
				"elements":        []map[string]interface{}{},
			},
		},
	})
	bodyMetadata := mustJSON(t, map[string]interface{}{
		deepOfficeChunkMetadataKey: map[string]interface{}{
			"knowledge_id":      "knowledge-1",
			"knowledge_base_id": "kb-1",
			"weknora_chunk_id":  "chunk-body",
			"chunk_index":       1,
			"source_document": map[string]interface{}{
				"title":       "BMT 설명회 자료",
				"uri":         "local://bmt.pdf",
				"source_type": "nas_document",
			},
			"source_locator": map[string]interface{}{"start_at": 23, "end_at": 128, "chunk_index": 1},
			"parser_grounding": map[string]interface{}{
				"parser_provider": "deep_parser",
				"elements": []map[string]interface{}{
					{
						"element_id":   "p1-e1",
						"page":         1,
						"layout_order": 0,
						"bbox":         []float64{0.1, 0.2, 0.3, 0.4},
					},
				},
			},
		},
	})

	addDeepOfficeChunkBindings(store, []*types.Chunk{
		{
			ID:              "chunk-title",
			KnowledgeID:     "knowledge-1",
			KnowledgeBaseID: "kb-1",
			ChunkIndex:      0,
			ChunkType:       types.ChunkTypeText,
			Metadata:        titleMetadata,
		},
		{
			ID:              "chunk-body",
			KnowledgeID:     "knowledge-1",
			KnowledgeBaseID: "kb-1",
			Content:         "BMT 수행 TASK 설명",
			ChunkIndex:      1,
			ChunkType:       types.ChunkTypeText,
			Metadata:        bodyMetadata,
		},
	})

	result := &types.SearchResult{
		ID:              "chunk-title",
		SubChunkID:      []string{"chunk-body"},
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
	}
	card := store.cardForResult(result)
	if card == nil {
		t.Fatal("expected citation card")
	}
	if got := card["evidence_status"]; got != "complete" {
		t.Fatalf("expected complete evidence status, got %v", got)
	}
	sourceLocator := card["source_locator"].(map[string]interface{})
	if got, ok := intFromInterface(sourceLocator["end_at"]); !ok || got != 128 {
		t.Fatalf("expected merged source locator end_at 128, got %v", sourceLocator["end_at"])
	}
	parserGrounding := card["parser_grounding"].(map[string]interface{})
	elements := parserGrounding["elements"].([]map[string]interface{})
	if len(elements) != 1 {
		t.Fatalf("expected subchunk parser element to be merged, got %d", len(elements))
	}
	bboxes := parserGrounding["bboxes"].([]map[string]interface{})
	if len(bboxes) != 1 {
		t.Fatalf("expected subchunk bbox to be merged, got %d", len(bboxes))
	}
	chunkEvidence := card["chunk_evidence"].([]map[string]interface{})
	if len(chunkEvidence) != 2 {
		t.Fatalf("expected two chunk evidence rows, got %d", len(chunkEvidence))
	}
}

func TestBuildDeepOfficeChunkMetadataMapsGroundingRange(t *testing.T) {
	knowledge := &types.Knowledge{
		ID:              "knowledge-1",
		TenantID:        7,
		KnowledgeBaseID: "kb-1",
		Title:           "BMT 설명회 자료",
		FileName:        "bmt.pdf",
		FileHash:        "sha256:document",
		UpdatedAt:       time.Date(2026, 5, 13, 9, 0, 0, 0, time.UTC),
	}
	chunk := &types.Chunk{
		ID:              "chunk-1",
		TenantID:        7,
		KnowledgeID:     "knowledge-1",
		KnowledgeBaseID: "kb-1",
		Content:         "BMT 평가 항목",
		ChunkIndex:      1,
		StartAt:         10,
		EndAt:           30,
		ChunkType:       types.ChunkTypeText,
	}
	metadata := map[string]string{
		"docreader_adapter":    "deep-office-docreader-adapter/v1",
		"deep_parser_provider": "deep_parser",
		"parser_run_id":        "run-1",
		"source_type":          "nas_document",
		"source_uri":           "nas://bmt.pdf",
		"source_title":         "BMT 설명회 자료",
		"file_content_sha256":  "sha256:document",
		"grounding_map": `[{"element_id":"p1-e1","page":1,"layout_order":0,"char_start":0,"char_end":12,"bbox":[0.1,0.2,0.3,0.4]},
			{"element_id":"p1-e2","page":1,"layout_order":1,"char_start":12,"char_end":24,"bbox":[0.2,0.3,0.4,0.5]},
			{"element_id":"p2-e1","page":2,"layout_order":0,"char_start":40,"char_end":50,"bbox":[0.5,0.6,0.7,0.8]}]`,
		"deep_office_page_images": `[{"page":1,"url":"local://7/page-1.png","storage_url":"local://7/page-1.png"},
			{"page":2,"url":"local://7/page-2.png","storage_url":"local://7/page-2.png"}]`,
	}

	chunkMetadata := buildDeepOfficeChunkMetadata(metadata, knowledge, chunk)
	if len(chunkMetadata) == 0 {
		t.Fatal("expected Deep Office chunk metadata")
	}
	metadataMap, err := chunkMetadata.Map()
	if err != nil {
		t.Fatalf("parse chunk metadata: %v", err)
	}
	evidence := metadataMap[deepOfficeChunkMetadataKey].(map[string]interface{})
	sourceDocument := evidence["source_document"].(map[string]interface{})
	if got := sourceDocument["uri"]; got != "nas://bmt.pdf" {
		t.Fatalf("unexpected source uri: %v", got)
	}
	parserGrounding := evidence["parser_grounding"].(map[string]interface{})
	elements := parserGrounding["elements"].([]interface{})
	if len(elements) != 2 {
		t.Fatalf("expected 2 overlapping parser elements, got %d", len(elements))
	}
	bboxes := parserGrounding["bboxes"].([]interface{})
	if len(bboxes) != 2 {
		t.Fatalf("expected 2 overlapping parser bboxes, got %d", len(bboxes))
	}
	pageImages := parserGrounding["page_images"].([]interface{})
	if len(pageImages) != 1 {
		t.Fatalf("expected 1 overlapping page image, got %d", len(pageImages))
	}
}

func TestEnrichDeepOfficeCitationsWithoutEnvIsNoop(t *testing.T) {
	t.Setenv(deepOfficeEvidenceBindingsEnv, "")
	resetDeepOfficeCitationStoreForTest()
	t.Cleanup(resetDeepOfficeCitationStoreForTest)

	result := &types.SearchResult{ID: "chunk-1"}
	enrichDeepOfficeCitations(context.Background(), []*types.SearchResult{result})

	if result.DeepOfficeCitation != nil {
		t.Fatalf("expected no citation card, got %#v", result.DeepOfficeCitation)
	}
}

func mustJSON(t *testing.T, payload map[string]interface{}) types.JSON {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return types.JSON(raw)
}

type fakeDeepOfficeChunkLookupRepository struct {
	scopedChunks []*types.Chunk
	sharedChunks []*types.Chunk
	scopedCalls  int
	sharedCalls  int
}

func (f *fakeDeepOfficeChunkLookupRepository) ListChunksByID(
	_ context.Context,
	_ uint64,
	ids []string,
) ([]*types.Chunk, error) {
	f.scopedCalls++
	return filterFakeChunksByID(f.scopedChunks, ids), nil
}

func (f *fakeDeepOfficeChunkLookupRepository) ListChunksByIDOnly(
	_ context.Context,
	ids []string,
) ([]*types.Chunk, error) {
	f.sharedCalls++
	return filterFakeChunksByID(f.sharedChunks, ids), nil
}

func filterFakeChunksByID(chunks []*types.Chunk, ids []string) []*types.Chunk {
	allowed := map[string]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	var out []*types.Chunk
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if _, ok := allowed[chunk.ID]; ok {
			out = append(out, chunk)
		}
	}
	return out
}
