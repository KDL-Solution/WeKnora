package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildGraphEvidenceByChunkID(t *testing.T) {
	graph := &types.GraphData{
		Node: []*types.GraphNode{
			{Name: "BMT", Chunks: []string{"chunk-1"}, Attributes: []string{"평가"}},
			{Name: "인식정확도", Chunks: []string{"chunk-1", "chunk-2"}, Attributes: []string{"성능지표"}},
			{Name: "체크박스", Chunks: []string{"chunk-3"}, Attributes: []string{"출력항목"}},
		},
		Relation: []*types.GraphRelation{
			{Node1: "BMT", Type: "평가한다", Node2: "인식정확도"},
			{Node1: "인식정확도", Type: "포함한다", Node2: "체크박스"},
		},
	}

	evidenceByChunkID := buildGraphEvidenceByChunkID([]string{"BMT"}, graph)
	evidence := evidenceByChunkID["chunk-1"]
	if evidence == nil {
		t.Fatal("expected graph evidence for chunk-1")
	}
	if got := evidence["match_type"]; got != "graph" {
		t.Fatalf("unexpected match_type: %v", got)
	}

	nodes := evidence["matched_nodes"].([]map[string]interface{})
	if len(nodes) != 2 {
		t.Fatalf("expected 2 matched nodes, got %d", len(nodes))
	}
	if nodes[0]["name"] != "BMT" || nodes[1]["name"] != "인식정확도" {
		t.Fatalf("unexpected matched node order: %#v", nodes)
	}

	relationships := evidence["relationships"].([]map[string]interface{})
	if len(relationships) != 2 {
		t.Fatalf("expected 2 graph relationships, got %d", len(relationships))
	}
	paths := evidence["paths"].([]map[string]interface{})
	if len(paths) != 2 {
		t.Fatalf("expected 2 graph paths, got %d", len(paths))
	}

	supportChunkIDs := evidence["support_chunk_ids"].([]string)
	if len(supportChunkIDs) != 3 {
		t.Fatalf("expected support chunks from adjacent graph nodes, got %v", supportChunkIDs)
	}
}

func TestRemoveDuplicateResultsPreservesGraphEvidence(t *testing.T) {
	vectorResult := &types.SearchResult{
		ID:        "chunk-1",
		Content:   "same content",
		MatchType: types.MatchTypeEmbedding,
	}
	graphResult := &types.SearchResult{
		ID:        "chunk-1",
		Content:   "same content",
		MatchType: types.MatchTypeGraph,
		GraphEvidence: map[string]interface{}{
			"match_type":      "graph",
			"source_chunk_id": "chunk-1",
		},
	}

	results := removeDuplicateResults([]*types.SearchResult{vectorResult, graphResult})
	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}
	if results[0].GraphEvidence == nil {
		t.Fatal("expected graph evidence to be preserved on vector result")
	}
	if got := results[0].GraphEvidence["source_chunk_id"]; got != "chunk-1" {
		t.Fatalf("unexpected source_chunk_id: %v", got)
	}
}
