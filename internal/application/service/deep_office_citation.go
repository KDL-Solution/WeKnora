package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const deepOfficeEvidenceBindingsEnv = "DEEP_OFFICE_EVIDENCE_BINDINGS_PATH"
const deepOfficeChunkMetadataKey = "deep_office_evidence"

type deepOfficeEvidenceFile struct {
	Bindings []deepOfficeEvidenceBinding `json:"bindings"`
}

type deepOfficeEvidenceBinding struct {
	BindingID           string                 `json:"binding_id"`
	ChunkIndex          int                    `json:"chunk_index"`
	ChunkTextHash       string                 `json:"chunk_text_hash"`
	ChunkVersion        int                    `json:"chunk_version"`
	KnowledgeBaseID     string                 `json:"knowledge_base_id"`
	KnowledgeID         string                 `json:"knowledge_id"`
	ParserGrounding     map[string]interface{} `json:"parser_grounding"`
	ParentChunkID       string                 `json:"parent_chunk_id,omitempty"`
	DerivedFromChunkIDs []string               `json:"derived_from_chunk_ids,omitempty"`
	IsSynthetic         bool                   `json:"is_synthetic,omitempty"`
	SourceDocument      map[string]interface{} `json:"source_document"`
	SourceLocator       map[string]interface{} `json:"source_locator"`
	SourceTextHash      string                 `json:"source_text_hash"`
	TenantID            string                 `json:"tenant_id"`
	WeknoraChunkID      string                 `json:"weknora_chunk_id"`
}

type deepOfficeCitationStore struct {
	byChunkID map[string]*deepOfficeEvidenceBinding
}

type deepOfficeChunkLookupRepository interface {
	ListChunksByID(ctx context.Context, tenantID uint64, ids []string) ([]*types.Chunk, error)
	ListChunksByIDOnly(ctx context.Context, ids []string) ([]*types.Chunk, error)
}

var (
	deepOfficeCitationOnce       sync.Once
	deepOfficeCitationStoreCache *deepOfficeCitationStore
	deepOfficeCitationErr        error
)

func enrichDeepOfficeCitations(ctx context.Context, results []*types.SearchResult) {
	if len(results) == 0 {
		return
	}

	store := deepOfficeCitationStoreForRequest(ctx)
	enrichDeepOfficeCitationsWithStore(results, store)
}

func enrichDeepOfficeCitationsWithChunkRepository(
	ctx context.Context,
	results []*types.SearchResult,
	chunkRepo interfaces.ChunkRepository,
	tenantID uint64,
) {
	if len(results) == 0 {
		return
	}

	store := deepOfficeCitationStoreForRequest(ctx)
	hydrateDeepOfficeCitationStoreFromChunks(ctx, store, results, chunkRepo, tenantID)
	enrichDeepOfficeCitationsWithStore(results, store)
}

func deepOfficeCitationStoreForRequest(ctx context.Context) *deepOfficeCitationStore {
	store, err := getDeepOfficeCitationStore()
	if err != nil {
		logger.Warnf(ctx, "Deep Office evidence bindings load failed: %v", err)
		store = nil
	}
	return cloneDeepOfficeCitationStore(store)
}

func enrichDeepOfficeCitationsWithStore(results []*types.SearchResult, store *deepOfficeCitationStore) {
	if store == nil {
		return
	}

	for _, result := range results {
		if result == nil {
			continue
		}
		card := store.cardForResult(result)
		if len(card) > 0 {
			result.DeepOfficeCitation = card
		}
	}
}

func cloneDeepOfficeCitationStore(source *deepOfficeCitationStore) *deepOfficeCitationStore {
	out := &deepOfficeCitationStore{byChunkID: map[string]*deepOfficeEvidenceBinding{}}
	if source == nil {
		return out
	}
	for chunkID, binding := range source.byChunkID {
		if chunkID == "" || binding == nil {
			continue
		}
		copied := *binding
		out.byChunkID[chunkID] = &copied
	}
	return out
}

func getDeepOfficeCitationStore() (*deepOfficeCitationStore, error) {
	deepOfficeCitationOnce.Do(func() {
		deepOfficeCitationStoreCache, deepOfficeCitationErr = loadDeepOfficeCitationStore()
	})
	return deepOfficeCitationStoreCache, deepOfficeCitationErr
}

func loadDeepOfficeCitationStore() (*deepOfficeCitationStore, error) {
	path := os.Getenv(deepOfficeEvidenceBindingsEnv)
	if path == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file deepOfficeEvidenceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}

	store := &deepOfficeCitationStore{byChunkID: make(map[string]*deepOfficeEvidenceBinding)}
	for i := range file.Bindings {
		binding := &file.Bindings[i]
		if binding.WeknoraChunkID == "" {
			continue
		}
		store.byChunkID[binding.WeknoraChunkID] = binding
	}
	return store, nil
}

func hydrateDeepOfficeCitationStoreFromChunks(
	ctx context.Context,
	store *deepOfficeCitationStore,
	results []*types.SearchResult,
	chunkRepo deepOfficeChunkLookupRepository,
	tenantID uint64,
) {
	if store == nil || chunkRepo == nil {
		return
	}

	var ids []string
	seen := make(map[string]struct{})
	resultsByChunkID := make(map[string][]*types.SearchResult)
	for _, result := range results {
		for _, id := range orderedReferenceChunkIDs(result) {
			if id == "" {
				continue
			}
			resultsByChunkID[id] = append(resultsByChunkID[id], result)
			if _, exists := store.byChunkID[id]; exists {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}

	found := make(map[string]struct{})
	if tenantID != 0 {
		chunks, err := chunkRepo.ListChunksByID(ctx, tenantID, ids)
		if err != nil {
			logger.Warnf(ctx, "Deep Office chunk evidence hydrate failed: %v", err)
		} else {
			addDeepOfficeChunkBindingsForResults(store, chunks, resultsByChunkID)
			for _, chunk := range chunks {
				if chunk != nil && chunk.ID != "" {
					found[chunk.ID] = struct{}{}
				}
			}
		}
	}

	var missing []string
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return
	}

	chunks, err := chunkRepo.ListChunksByIDOnly(ctx, missing)
	if err != nil {
		logger.Warnf(ctx, "Deep Office shared chunk evidence hydrate failed: %v", err)
		return
	}
	addDeepOfficeChunkBindingsForResults(store, chunks, resultsByChunkID)
}

func addDeepOfficeChunkBindings(store *deepOfficeCitationStore, chunks []*types.Chunk) {
	if store == nil {
		return
	}
	if store.byChunkID == nil {
		store.byChunkID = map[string]*deepOfficeEvidenceBinding{}
	}
	for _, chunk := range chunks {
		binding, ok := deepOfficeBindingFromChunkMetadata(chunk)
		if !ok || binding.WeknoraChunkID == "" {
			continue
		}
		store.byChunkID[binding.WeknoraChunkID] = binding
	}
}

func addDeepOfficeChunkBindingsForResults(
	store *deepOfficeCitationStore,
	chunks []*types.Chunk,
	resultsByChunkID map[string][]*types.SearchResult,
) {
	if store == nil {
		return
	}
	if store.byChunkID == nil {
		store.byChunkID = map[string]*deepOfficeEvidenceBinding{}
	}
	for _, chunk := range chunks {
		if chunk == nil || !deepOfficeChunkMatchesAnyResult(chunk, resultsByChunkID[chunk.ID]) {
			continue
		}
		binding, ok := deepOfficeBindingFromChunkMetadata(chunk)
		if !ok || binding.WeknoraChunkID == "" {
			continue
		}
		store.byChunkID[binding.WeknoraChunkID] = binding
	}
}

func (s *deepOfficeCitationStore) cardForResult(result *types.SearchResult) map[string]interface{} {
	chunkIDs := orderedReferenceChunkIDs(result)
	if len(chunkIDs) == 0 {
		return nil
	}

	var bindings []*deepOfficeEvidenceBinding
	var missingChunkIDs []string
	for _, chunkID := range chunkIDs {
		binding, ok := s.bindingForChunkID(chunkID, result)
		if !ok {
			missingChunkIDs = append(missingChunkIDs, chunkID)
			continue
		}
		bindings = append(bindings, binding)
	}
	if len(bindings) == 0 {
		return nil
	}

	sourceDocument := cloneInterfaceMap(bindings[0].SourceDocument)
	sourceLocator := mergeDeepOfficeSourceLocator(bindings, chunkIDs, missingChunkIDs, result)
	parserGrounding := mergeDeepOfficeParserGrounding(bindings)
	chunkEvidence := buildDeepOfficeChunkEvidence(bindings, result)

	status := "complete"
	if len(missingChunkIDs) > 0 {
		status = "partial"
	}

	card := map[string]interface{}{
		"evidence_status":  status,
		"source_document":  sourceDocument,
		"source_locator":   sourceLocator,
		"parser_grounding": parserGrounding,
		"chunk_evidence":   chunkEvidence,
		"hashes": map[string]interface{}{
			"source_text_hashes": uniqueStringsFromBindings(bindings, func(b *deepOfficeEvidenceBinding) string {
				return b.SourceTextHash
			}),
			"chunk_text_hashes": uniqueStringsFromBindings(bindings, func(b *deepOfficeEvidenceBinding) string {
				return b.ChunkTextHash
			}),
		},
	}
	if len(result.GraphEvidence) > 0 {
		card["graph_evidence"] = result.GraphEvidence
	}
	return card
}

func (s *deepOfficeCitationStore) bindingForChunkID(
	chunkID string,
	result *types.SearchResult,
) (*deepOfficeEvidenceBinding, bool) {
	if s != nil {
		if binding, ok := s.byChunkID[chunkID]; ok && deepOfficeBindingMatchesResult(binding, result) {
			return binding, true
		}
	}

	if result == nil || chunkID != result.ID {
		return nil, false
	}

	if binding, ok := deepOfficeBindingFromResultChunkMetadata(result); ok {
		return binding, true
	}

	if s == nil || result.ParentChunkID == "" {
		return nil, false
	}
	parent, ok := s.byChunkID[result.ParentChunkID]
	if !ok || !deepOfficeBindingMatchesResult(parent, result) {
		return nil, false
	}
	return deriveDeepOfficeBindingFromParent(
		parent,
		result.ID,
		result.Content,
		result.ChunkIndex,
		result.ChunkType,
		result.ParentChunkID,
		result.KnowledgeID,
		result.KnowledgeBaseID,
	), true
}

func deepOfficeBindingMatchesResult(binding *deepOfficeEvidenceBinding, result *types.SearchResult) bool {
	if binding == nil || result == nil {
		return binding != nil
	}
	if binding.KnowledgeID != "" && result.KnowledgeID != "" && binding.KnowledgeID != result.KnowledgeID {
		return false
	}
	if binding.KnowledgeBaseID != "" && result.KnowledgeBaseID != "" && binding.KnowledgeBaseID != result.KnowledgeBaseID {
		return false
	}
	return true
}

func deepOfficeChunkMatchesAnyResult(chunk *types.Chunk, results []*types.SearchResult) bool {
	if chunk == nil || len(results) == 0 {
		return false
	}
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.KnowledgeID != "" && chunk.KnowledgeID != "" && result.KnowledgeID != chunk.KnowledgeID {
			continue
		}
		if result.KnowledgeBaseID != "" && chunk.KnowledgeBaseID != "" && result.KnowledgeBaseID != chunk.KnowledgeBaseID {
			continue
		}
		return true
	}
	return false
}

func deepOfficeBindingFromResultChunkMetadata(result *types.SearchResult) (*deepOfficeEvidenceBinding, bool) {
	if result == nil || len(result.ChunkMetadata) == 0 {
		return nil, false
	}
	metadata, err := result.ChunkMetadata.Map()
	if err != nil {
		return nil, false
	}
	rawEvidence, ok := metadata[deepOfficeChunkMetadataKey]
	if !ok {
		return nil, false
	}
	raw, err := json.Marshal(rawEvidence)
	if err != nil {
		return nil, false
	}
	var binding deepOfficeEvidenceBinding
	if err := json.Unmarshal(raw, &binding); err != nil {
		return nil, false
	}
	if binding.WeknoraChunkID == "" {
		binding.WeknoraChunkID = result.ID
	}
	if binding.KnowledgeID == "" {
		binding.KnowledgeID = result.KnowledgeID
	}
	if binding.KnowledgeBaseID == "" {
		binding.KnowledgeBaseID = result.KnowledgeBaseID
	}
	if binding.ChunkIndex == 0 {
		binding.ChunkIndex = result.ChunkIndex
	}
	if binding.ChunkVersion == 0 {
		binding.ChunkVersion = 1
	}
	if binding.ChunkTextHash == "" {
		binding.ChunkTextHash = deepOfficeHashText(result.Content)
	}
	if binding.SourceTextHash == "" {
		binding.SourceTextHash = binding.ChunkTextHash
	}
	if binding.BindingID == "" {
		binding.BindingID = deepOfficeBindingID(
			binding.TenantID,
			binding.KnowledgeBaseID,
			binding.KnowledgeID,
			binding.WeknoraChunkID,
			fmt.Sprintf("%d", binding.ChunkIndex),
			binding.ChunkTextHash,
		)
	}
	return &binding, deepOfficeBindingMatchesResult(&binding, result)
}

func deepOfficeBindingFromChunkMetadata(chunk *types.Chunk) (*deepOfficeEvidenceBinding, bool) {
	if chunk == nil || len(chunk.Metadata) == 0 {
		return nil, false
	}
	return deepOfficeBindingFromResultChunkMetadata(&types.SearchResult{
		ID:              chunk.ID,
		Content:         chunk.Content,
		KnowledgeID:     chunk.KnowledgeID,
		KnowledgeBaseID: chunk.KnowledgeBaseID,
		ChunkIndex:      chunk.ChunkIndex,
		ChunkType:       string(chunk.ChunkType),
		ParentChunkID:   chunk.ParentChunkID,
		ChunkMetadata:   chunk.Metadata,
	})
}

func deriveDeepOfficeBindingFromParent(
	parent *deepOfficeEvidenceBinding,
	chunkID string,
	content string,
	chunkIndex int,
	chunkType string,
	parentChunkID string,
	knowledgeID string,
	knowledgeBaseID string,
) *deepOfficeEvidenceBinding {
	if parent == nil || chunkID == "" {
		return nil
	}
	if parentChunkID == "" {
		parentChunkID = parent.WeknoraChunkID
	}
	if chunkType == "" {
		chunkType = "derived"
	}

	binding := *parent
	binding.WeknoraChunkID = chunkID
	binding.ChunkIndex = chunkIndex
	binding.ChunkVersion = 1
	binding.ParentChunkID = parentChunkID
	binding.DerivedFromChunkIDs = []string{parentChunkID}
	binding.IsSynthetic = true
	binding.ChunkTextHash = deepOfficeHashText(content)
	if knowledgeID != "" {
		binding.KnowledgeID = knowledgeID
	}
	if knowledgeBaseID != "" {
		binding.KnowledgeBaseID = knowledgeBaseID
	}
	binding.BindingID = deepOfficeBindingID(
		binding.TenantID,
		binding.KnowledgeBaseID,
		binding.KnowledgeID,
		chunkID,
		parentChunkID,
		binding.ChunkTextHash,
	)

	sourceLocator := cloneInterfaceMap(parent.SourceLocator)
	sourceLocator["chunk_type"] = chunkType
	sourceLocator["derived_chunk_id"] = chunkID
	sourceLocator["parent_chunk_id"] = parentChunkID
	sourceLocator["derived_from_chunk_ids"] = []string{parentChunkID}
	binding.SourceLocator = sourceLocator

	parserGrounding := cloneInterfaceMap(parent.ParserGrounding)
	parserGrounding["derived_chunk_id"] = chunkID
	parserGrounding["derived_from_chunk_ids"] = []string{parentChunkID}
	binding.ParserGrounding = parserGrounding
	return &binding
}

func orderedReferenceChunkIDs(result *types.SearchResult) []string {
	var ids []string
	seen := make(map[string]struct{})
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	add(result.ID)
	for _, id := range result.SubChunkID {
		add(id)
	}
	return ids
}

func mergeDeepOfficeSourceLocator(
	bindings []*deepOfficeEvidenceBinding,
	chunkIDs []string,
	missingChunkIDs []string,
	result *types.SearchResult,
) map[string]interface{} {
	locator := cloneInterfaceMap(bindings[0].SourceLocator)
	locator["knowledge_id"] = result.KnowledgeID
	locator["knowledge_base_id"] = result.KnowledgeBaseID
	locator["weknora_chunk_ids"] = chunkIDs
	locator["evidence_binding_ids"] = uniqueStringsFromBindings(bindings, func(b *deepOfficeEvidenceBinding) string {
		return b.BindingID
	})
	if len(missingChunkIDs) > 0 {
		locator["missing_weknora_chunk_ids"] = missingChunkIDs
	}

	startAt, hasStart := intFromInterface(locator["start_at"])
	endAt, hasEnd := intFromInterface(locator["end_at"])
	for _, binding := range bindings[1:] {
		if start, ok := intFromInterface(binding.SourceLocator["start_at"]); ok {
			if !hasStart || start < startAt {
				startAt = start
				hasStart = true
			}
		}
		if end, ok := intFromInterface(binding.SourceLocator["end_at"]); ok {
			if !hasEnd || end > endAt {
				endAt = end
				hasEnd = true
			}
		}
	}
	if hasStart {
		locator["start_at"] = startAt
	}
	if hasEnd {
		locator["end_at"] = endAt
	}
	return locator
}

func mergeDeepOfficeParserGrounding(bindings []*deepOfficeEvidenceBinding) map[string]interface{} {
	grounding := map[string]interface{}{}
	if len(bindings) > 0 {
		for _, key := range []string{"parser_provider", "parser_run_id"} {
			if value, ok := bindings[0].ParserGrounding[key]; ok && value != nil {
				grounding[key] = value
			}
		}
	}

	pages := make(map[int]struct{})
	elementByID := make(map[string]map[string]interface{})
	pageImageByPage := make(map[int]map[string]interface{})
	var elements []map[string]interface{}
	var bboxes []map[string]interface{}

	for _, binding := range bindings {
		rawElements, ok := binding.ParserGrounding["elements"].([]interface{})
		if ok {
			for _, rawElement := range rawElements {
				element, ok := rawElement.(map[string]interface{})
				if !ok {
					continue
				}

				elementID, _ := element["element_id"].(string)
				if elementID == "" {
					continue
				}
				if _, seen := elementByID[elementID]; seen {
					continue
				}

				copiedElement := cloneInterfaceMap(element)
				elementByID[elementID] = copiedElement
				elements = append(elements, copiedElement)

				if page, ok := intFromInterface(element["page"]); ok {
					pages[page] = struct{}{}
				}
				if bbox, ok := element["bbox"].([]interface{}); ok && len(bbox) > 0 {
					bboxes = append(bboxes, map[string]interface{}{
						"element_id": elementID,
						"page":       copiedElement["page"],
						"bbox":       bbox,
					})
				}
			}
		}
		if rawImages, ok := binding.ParserGrounding["page_images"].([]interface{}); ok {
			for _, rawImage := range rawImages {
				image, ok := rawImage.(map[string]interface{})
				if !ok {
					continue
				}
				page, ok := intFromInterface(image["page"])
				if !ok || page <= 0 {
					continue
				}
				if _, seen := pageImageByPage[page]; seen {
					continue
				}
				pageImageByPage[page] = cloneInterfaceMap(image)
			}
		}
	}

	sort.Slice(elements, func(i, j int) bool {
		leftPage, _ := intFromInterface(elements[i]["page"])
		rightPage, _ := intFromInterface(elements[j]["page"])
		if leftPage != rightPage {
			return leftPage < rightPage
		}
		leftOrder, _ := intFromInterface(elements[i]["layout_order"])
		rightOrder, _ := intFromInterface(elements[j]["layout_order"])
		return leftOrder < rightOrder
	})

	pageList := make([]int, 0, len(pages))
	for page := range pages {
		pageList = append(pageList, page)
	}
	sort.Ints(pageList)

	grounding["pages"] = pageList
	grounding["elements"] = elements
	grounding["bboxes"] = bboxes
	if len(pageImageByPage) > 0 {
		imagePages := make([]int, 0, len(pageImageByPage))
		for page := range pageImageByPage {
			if len(pageList) > 0 {
				if _, ok := pages[page]; !ok {
					continue
				}
			}
			imagePages = append(imagePages, page)
		}
		sort.Ints(imagePages)
		pageImages := make([]map[string]interface{}, 0, len(imagePages))
		for _, page := range imagePages {
			pageImages = append(pageImages, pageImageByPage[page])
		}
		if len(pageImages) > 0 {
			grounding["page_images"] = pageImages
		}
	}
	return grounding
}

func buildDeepOfficeChunkEvidence(
	bindings []*deepOfficeEvidenceBinding,
	result *types.SearchResult,
) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(bindings))
	for _, binding := range bindings {
		grounding := cloneInterfaceMap(binding.ParserGrounding)
		pages, elementCount, bboxCount := summarizeBindingGrounding(binding)
		sourceLocator := cloneInterfaceMap(binding.SourceLocator)
		item := map[string]interface{}{
			"chunk_id":            binding.WeknoraChunkID,
			"chunk_index":         binding.ChunkIndex,
			"chunk_version":       binding.ChunkVersion,
			"evidence_binding_id": binding.BindingID,
			"knowledge_id":        binding.KnowledgeID,
			"knowledge_base_id":   binding.KnowledgeBaseID,
			"source_locator":      sourceLocator,
			"parser_grounding":    grounding,
			"pages":               pages,
			"element_count":       elementCount,
			"bbox_count":          bboxCount,
			"source_text_hash":    binding.SourceTextHash,
			"chunk_text_hash":     binding.ChunkTextHash,
		}
		if binding.ParentChunkID != "" {
			item["parent_chunk_id"] = binding.ParentChunkID
		}
		if len(binding.DerivedFromChunkIDs) > 0 {
			item["derived_from_chunk_ids"] = binding.DerivedFromChunkIDs
		}
		if binding.IsSynthetic {
			item["is_synthetic"] = true
		}
		if result != nil && binding.WeknoraChunkID == result.ID {
			item["content_preview"] = result.Content
			item["score"] = result.Score
			item["match_type"] = result.MatchType
			item["start_at"] = result.StartAt
			item["end_at"] = result.EndAt
		}
		items = append(items, item)
	}
	return items
}

func summarizeBindingGrounding(binding *deepOfficeEvidenceBinding) ([]int, int, int) {
	rawElements, ok := binding.ParserGrounding["elements"].([]interface{})
	if !ok {
		return nil, 0, 0
	}

	pages := make(map[int]struct{})
	bboxCount := 0
	for _, rawElement := range rawElements {
		element, ok := rawElement.(map[string]interface{})
		if !ok {
			continue
		}
		if page, ok := intFromInterface(element["page"]); ok {
			pages[page] = struct{}{}
		}
		if bbox, ok := element["bbox"].([]interface{}); ok && len(bbox) > 0 {
			bboxCount++
		}
	}

	pageList := make([]int, 0, len(pages))
	for page := range pages {
		pageList = append(pageList, page)
	}
	sort.Ints(pageList)
	return pageList, len(rawElements), bboxCount
}

func uniqueStringsFromBindings(
	bindings []*deepOfficeEvidenceBinding,
	get func(*deepOfficeEvidenceBinding) string,
) []string {
	var values []string
	seen := make(map[string]struct{})
	for _, binding := range bindings {
		value := get(binding)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func cloneInterfaceMap(source map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func deepOfficeBindingID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return fmt.Sprintf("%x", sum[:])
}

func deepOfficeHashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + fmt.Sprintf("%x", sum[:])
}

func intFromInterface(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		asInt, err := typed.Int64()
		if err == nil {
			return int(asInt), true
		}
	}
	return 0, false
}

func resetDeepOfficeCitationStoreForTest() {
	deepOfficeCitationOnce = sync.Once{}
	deepOfficeCitationStoreCache = nil
	deepOfficeCitationErr = nil
}
