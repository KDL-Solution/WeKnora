package chatpipeline

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/searchutil"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginSearch implements search functionality for chat pipeline
type PluginSearchEntity struct {
	graphRepo     interfaces.RetrieveGraphRepository
	chunkRepo     interfaces.ChunkRepository
	knowledgeRepo interfaces.KnowledgeRepository
}

// NewPluginSearchEntity creates a new plugin search entity
func NewPluginSearchEntity(
	eventManager *EventManager,
	graphRepository interfaces.RetrieveGraphRepository,
	chunkRepository interfaces.ChunkRepository,
	knowledgeRepository interfaces.KnowledgeRepository,
) *PluginSearchEntity {
	res := &PluginSearchEntity{
		graphRepo:     graphRepository,
		chunkRepo:     chunkRepository,
		knowledgeRepo: knowledgeRepository,
	}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the list of event types this plugin responds to
func (p *PluginSearchEntity) ActivationEvents() []types.EventType {
	return []types.EventType{types.ENTITY_SEARCH}
}

// OnEvent processes triggered events
func (p *PluginSearchEntity) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	entity := chatManage.Entity
	if len(entity) == 0 {
		logger.Infof(ctx, "No entity found")
		return next()
	}

	// Use EntityKBIDs (knowledge bases with ExtractConfig enabled)
	knowledgeBaseIDs := chatManage.EntityKBIDs
	// Use EntityKnowledge (KnowledgeID -> KnowledgeBaseID mapping for graph-enabled files)
	entityKnowledge := chatManage.EntityKnowledge

	if len(knowledgeBaseIDs) == 0 && len(entityKnowledge) == 0 {
		logger.Warnf(ctx, "No knowledge base IDs or knowledge IDs with ExtractConfig enabled for entity search")
		return next()
	}

	// Parallel search across multiple knowledge bases and individual files
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allNodes []*types.GraphNode
	var allRelations []*types.GraphRelation

	// If specific KnowledgeIDs are provided, search by individual files
	if len(entityKnowledge) > 0 {
		logger.Infof(ctx, "Searching entities across %d knowledge file(s)", len(entityKnowledge))
		for knowledgeID, kbID := range entityKnowledge {
			wg.Add(1)
			go func(knowledgeBaseID, knowledgeID string) {
				defer wg.Done()

				graph, err := p.graphRepo.SearchNode(ctx, types.NameSpace{
					KnowledgeBase: knowledgeBaseID,
					Knowledge:     knowledgeID,
				}, entity)
				if err != nil {
					logger.Errorf(ctx, "Failed to search entity in Knowledge %s: %v", knowledgeID, err)
					return
				}

				logger.Infof(
					ctx,
					"Knowledge %s entity search result count: %d nodes, %d relations",
					knowledgeID,
					len(graph.Node),
					len(graph.Relation),
				)

				mu.Lock()
				allNodes = append(allNodes, graph.Node...)
				allRelations = append(allRelations, graph.Relation...)
				mu.Unlock()
			}(kbID, knowledgeID)
		}
	} else {
		// Otherwise, search by knowledge base
		logger.Infof(ctx, "Searching entities across %d knowledge base(s): %v", len(knowledgeBaseIDs), knowledgeBaseIDs)
		for _, kbID := range knowledgeBaseIDs {
			wg.Add(1)
			go func(knowledgeBaseID string) {
				defer wg.Done()

				graph, err := p.graphRepo.SearchNode(ctx, types.NameSpace{KnowledgeBase: knowledgeBaseID}, entity)
				if err != nil {
					logger.Errorf(ctx, "Failed to search entity in KB %s: %v", knowledgeBaseID, err)
					return
				}

				logger.Infof(
					ctx,
					"KB %s entity search result count: %d nodes, %d relations",
					knowledgeBaseID,
					len(graph.Node),
					len(graph.Relation),
				)

				mu.Lock()
				allNodes = append(allNodes, graph.Node...)
				allRelations = append(allRelations, graph.Relation...)
				mu.Unlock()
			}(kbID)
		}
	}

	wg.Wait()

	// Merge graph data
	chatManage.GraphResult = &types.GraphData{
		Node:     allNodes,
		Relation: allRelations,
	}
	logger.Infof(ctx, "Total entity search result: %d nodes, %d relations", len(allNodes), len(allRelations))
	graphEvidenceByChunkID := buildGraphEvidenceByChunkID(chatManage.Entity, chatManage.GraphResult)

	chunkIDs := filterSeenChunk(ctx, chatManage.GraphResult, chatManage.SearchResult)
	if len(chunkIDs) == 0 {
		logger.Infof(ctx, "No new chunk found")
		return next()
	}
	chunks, err := p.chunkRepo.ListChunksByID(ctx, types.MustTenantIDFromContext(ctx), chunkIDs)
	if err != nil {
		logger.Errorf(ctx, "Failed to list chunks, session_id: %s, error: %v", chatManage.SessionID, err)
		return next()
	}
	knowledgeIDs := []string{}
	for _, chunk := range chunks {
		knowledgeIDs = append(knowledgeIDs, chunk.KnowledgeID)
	}
	knowledges, err := p.knowledgeRepo.GetKnowledgeBatch(
		ctx,
		types.MustTenantIDFromContext(ctx),
		knowledgeIDs,
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to list knowledge, session_id: %s, error: %v", chatManage.SessionID, err)
		return next()
	}

	knowledgeMap := map[string]*types.Knowledge{}
	for _, knowledge := range knowledges {
		knowledgeMap[knowledge.ID] = knowledge
	}
	var entityResults []*types.SearchResult
	for _, chunk := range chunks {
		searchResult := chunk2SearchResult(chunk, knowledgeMap[chunk.KnowledgeID])
		if evidence := graphEvidenceByChunkID[chunk.ID]; len(evidence) > 0 {
			searchResult.GraphEvidence = evidence
		}
		entityResults = append(entityResults, searchResult)
	}
	searchutil.EnrichSearchResultsImageInfo(ctx, p.chunkRepo, types.MustTenantIDFromContext(ctx), entityResults)
	chatManage.SearchResult = append(chatManage.SearchResult, entityResults...)
	// remove duplicate results
	chatManage.SearchResult = removeDuplicateResults(chatManage.SearchResult)
	if len(chatManage.SearchResult) == 0 {
		logger.Infof(ctx, "No new search result, session_id: %s", chatManage.SessionID)
		return ErrSearchNothing
	}
	logger.Infof(
		ctx,
		"search entity result count: %d, session_id: %s",
		len(chatManage.SearchResult),
		chatManage.SessionID,
	)
	return next()
}

// filterSeenChunk filters seen chunks from the graph
func filterSeenChunk(ctx context.Context, graph *types.GraphData, searchResult []*types.SearchResult) []string {
	seen := map[string]bool{}
	for _, chunk := range searchResult {
		seen[chunk.ID] = true
	}
	logger.Infof(ctx, "filterSeenChunk: seen count: %d", len(seen))

	chunkIDs := []string{}
	for _, node := range graph.Node {
		for _, chunkID := range node.Chunks {
			if seen[chunkID] {
				continue
			}
			seen[chunkID] = true
			chunkIDs = append(chunkIDs, chunkID)
		}
	}
	logger.Infof(ctx, "filterSeenChunk: new chunkIDs count: %d", len(chunkIDs))
	return chunkIDs
}

// chunk2SearchResult converts a chunk to a search result
func chunk2SearchResult(chunk *types.Chunk, knowledge *types.Knowledge) *types.SearchResult {
	return &types.SearchResult{
		ID:                chunk.ID,
		Content:           chunk.Content,
		KnowledgeID:       chunk.KnowledgeID,
		ChunkIndex:        chunk.ChunkIndex,
		KnowledgeTitle:    knowledge.Title,
		StartAt:           chunk.StartAt,
		EndAt:             chunk.EndAt,
		Seq:               chunk.ChunkIndex,
		Score:             1.0,
		MatchType:         types.MatchTypeGraph,
		Metadata:          knowledge.GetMetadata(),
		ChunkType:         string(chunk.ChunkType),
		ParentChunkID:     chunk.ParentChunkID,
		ImageInfo:         chunk.ImageInfo,
		KnowledgeFilename: knowledge.FileName,
		KnowledgeSource:   knowledge.Source,
		KnowledgeChannel:  knowledge.Channel,
		ChunkMetadata:     chunk.Metadata,
		KnowledgeBaseID:   knowledge.KnowledgeBaseID,
	}
}

func buildGraphEvidenceByChunkID(queryEntities []string, graph *types.GraphData) map[string]map[string]interface{} {
	if graph == nil || len(graph.Node) == 0 {
		return nil
	}

	nodeByName := make(map[string]*types.GraphNode)
	chunkToNodeNames := make(map[string]map[string]struct{})
	for _, node := range graph.Node {
		if node == nil || node.Name == "" {
			continue
		}
		nodeByName[node.Name] = node
		for _, chunkID := range node.Chunks {
			if chunkID == "" {
				continue
			}
			if _, ok := chunkToNodeNames[chunkID]; !ok {
				chunkToNodeNames[chunkID] = make(map[string]struct{})
			}
			chunkToNodeNames[chunkID][node.Name] = struct{}{}
		}
	}
	if len(chunkToNodeNames) == 0 {
		return nil
	}

	out := make(map[string]map[string]interface{}, len(chunkToNodeNames))
	for chunkID, nodeNames := range chunkToNodeNames {
		matchedNodes := graphEvidenceNodes(nodeNames, nodeByName)
		relationships := graphEvidenceRelations(nodeNames, graph.Relation)
		supportChunkIDs := graphEvidenceSupportChunkIDs(nodeNames, relationships, nodeByName)

		out[chunkID] = map[string]interface{}{
			"match_type":        "graph",
			"query_entities":    uniqueStrings(queryEntities),
			"source_chunk_id":   chunkID,
			"matched_nodes":     matchedNodes,
			"relationships":     relationships,
			"paths":             graphEvidencePaths(relationships),
			"support_chunk_ids": supportChunkIDs,
		}
	}
	return out
}

func graphEvidenceNodes(
	nodeNames map[string]struct{},
	nodeByName map[string]*types.GraphNode,
) []map[string]interface{} {
	names := sortedSetKeys(nodeNames)
	nodes := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		node := nodeByName[name]
		if node == nil {
			continue
		}
		nodes = append(nodes, map[string]interface{}{
			"name":              node.Name,
			"attributes":        uniqueStrings(node.Attributes),
			"support_chunk_ids": uniqueStrings(node.Chunks),
		})
	}
	return nodes
}

func graphEvidenceRelations(
	nodeNames map[string]struct{},
	relations []*types.GraphRelation,
) []map[string]interface{} {
	seen := make(map[string]struct{})
	items := make([]map[string]interface{}, 0)
	for _, rel := range relations {
		if rel == nil || rel.Node1 == "" || rel.Node2 == "" {
			continue
		}
		if _, ok := nodeNames[rel.Node1]; !ok {
			if _, ok := nodeNames[rel.Node2]; !ok {
				continue
			}
		}
		key := rel.Node1 + "\x00" + rel.Type + "\x00" + rel.Node2
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, map[string]interface{}{
			"source":   rel.Node1,
			"relation": rel.Type,
			"target":   rel.Node2,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return graphRelationSortKey(items[i]) < graphRelationSortKey(items[j])
	})
	return items
}

func graphEvidencePaths(relations []map[string]interface{}) []map[string]interface{} {
	paths := make([]map[string]interface{}, 0, len(relations))
	for _, rel := range relations {
		source, _ := rel["source"].(string)
		relation, _ := rel["relation"].(string)
		target, _ := rel["target"].(string)
		if source == "" || target == "" {
			continue
		}
		paths = append(paths, map[string]interface{}{
			"path": []map[string]string{
				{"node": source},
				{"relation": relation},
				{"node": target},
			},
		})
	}
	return paths
}

func graphEvidenceSupportChunkIDs(
	nodeNames map[string]struct{},
	relations []map[string]interface{},
	nodeByName map[string]*types.GraphNode,
) []string {
	relatedNodeNames := make(map[string]struct{}, len(nodeNames))
	for name := range nodeNames {
		relatedNodeNames[name] = struct{}{}
	}
	for _, rel := range relations {
		if source, _ := rel["source"].(string); source != "" {
			relatedNodeNames[source] = struct{}{}
		}
		if target, _ := rel["target"].(string); target != "" {
			relatedNodeNames[target] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	for _, name := range sortedSetKeys(relatedNodeNames) {
		node := nodeByName[name]
		if node == nil {
			continue
		}
		for _, chunkID := range node.Chunks {
			if chunkID != "" {
				seen[chunkID] = struct{}{}
			}
		}
	}
	return sortedSetKeys(seen)
}

func graphRelationSortKey(item map[string]interface{}) string {
	return strings.Join([]string{
		stringFromMap(item, "source"),
		stringFromMap(item, "relation"),
		stringFromMap(item, "target"),
	}, "\x00")
}

func stringFromMap(item map[string]interface{}, key string) string {
	value, _ := item[key].(string)
	return value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sortedSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
