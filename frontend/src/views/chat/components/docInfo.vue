<template>
    <div class="refer" v-if="session.knowledge_references && session.knowledge_references.length">
        <div class="refer_header" @click="referBoxSwitch">
            <div class="refer_title">
                <img src="@/assets/img/ziliao.svg" :alt="$t('chat.referenceIconAlt')" />
                <span>{{ headerText }}</span>
            </div>
            <div class="refer_show_icon">
                <t-icon :name="showReferBox ? 'chevron-up' : 'chevron-down'" />
            </div>
        </div>
        <div class="refer_box" v-show="showReferBox">
            <!-- Web search references (ungrouped) -->
            <div v-for="(item, index) in webSearchRefs" :key="'web-' + index">
                <a
                    :href="getWebSearchUrl(item)"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="doc doc-web"
                    @click.stop
                >
                    {{ webSearchRefs.length < 2 ? getWebSearchDisplayText(item) : `${index + 1}. ${getWebSearchDisplayText(item)}` }}
                </a>
            </div>

            <!-- Knowledge references grouped by document -->
            <div v-for="(group, gIdx) in groupedKnowledgeRefs" :key="'grp-' + gIdx" class="doc-group">
                <div class="doc-group-header" @click="toggleGroup(group.key)">
                    <div class="doc-group-left">
                        <t-icon :name="expandedGroups[group.key] ? 'chevron-down' : 'chevron-right'" size="14px" class="doc-group-arrow" />
                        <t-icon name="file" size="14px" class="doc-group-icon" />
                        <span class="doc-group-title" :title="group.title">{{ group.title }}</span>
                        <span class="doc-group-count">{{ $t('chat.referenceChunkCount', { count: group.chunks.length }) }}</span>
                    </div>
                    <div class="doc-group-actions" v-if="group.knowledgeBaseId" @click.stop>
                        <t-tooltip :content="$t('chat.navigateToDocument')">
                            <span class="doc-group-navigate" @click="navigateToDocument(group)">
                                <t-icon name="jump" size="14px" />
                            </span>
                        </t-tooltip>
                    </div>
                </div>
                <div v-if="hasEvidence(group)" class="doc-evidence-card" @click.stop>
                    <div class="doc-evidence-title">
                        <t-icon name="check-circle" size="13px" />
                        <span>근거</span>
                        <span class="doc-evidence-status" :class="{ partial: evidenceStatus(group) === 'partial' }">
                            {{ evidenceStatusLabel(group) }}
                        </span>
                        <t-button
                            v-if="canOpenEvidencePreview(group)"
                            theme="primary"
                            variant="text"
                            size="small"
                            class="doc-evidence-open"
                            @click.stop="openEvidencePreview(group)"
                        >
                            <template #icon><t-icon name="file-view" /></template>
                            근거 보기
                        </t-button>
                    </div>
                    <div class="doc-evidence-grid">
                        <div class="doc-evidence-item">
                            <span class="doc-evidence-label">출처</span>
                            <span class="doc-evidence-value" :title="sourceUri(group)">{{ sourceUri(group) }}</span>
                        </div>
                        <div class="doc-evidence-item" v-if="pageLabel(group)">
                            <span class="doc-evidence-label">페이지</span>
                            <span class="doc-evidence-value">{{ pageLabel(group) }}</span>
                        </div>
                        <div class="doc-evidence-item" v-if="parserSummary(group)">
                            <span class="doc-evidence-label">Parser</span>
                            <span class="doc-evidence-value">{{ parserSummary(group) }}</span>
                        </div>
                        <div class="doc-evidence-item" v-if="chunkSummary(group)">
                            <span class="doc-evidence-label">청크</span>
                            <span class="doc-evidence-value">{{ chunkSummary(group) }}</span>
                        </div>
                        <div class="doc-evidence-item" v-if="graphSummary(group)">
                            <span class="doc-evidence-label">Graph</span>
                            <span class="doc-evidence-value">{{ graphSummary(group) }}</span>
                        </div>
                        <div class="doc-evidence-item" v-if="checksum(group)">
                            <span class="doc-evidence-label">해시</span>
                            <span class="doc-evidence-value" :title="checksum(group)">{{ checksum(group) }}</span>
                        </div>
                    </div>
                    <div v-if="chunkEvidenceItems(group).length" class="doc-evidence-used">
                        <div class="doc-evidence-used-title">답변에 사용된 근거 청크</div>
                        <button
                            v-for="item in chunkEvidenceItems(group)"
                            :key="item.key"
                            type="button"
                            class="doc-evidence-used-item"
                            @click.stop="openChunkEvidencePreview(group, item.reference)"
                        >
                            <div class="used-item-head">
                                <span class="used-item-rank">근거 {{ item.rank }}</span>
                                <span v-if="item.pageLabel" class="used-item-page">{{ item.pageLabel }}</span>
                                <span v-if="item.chunkLabel" class="used-item-chunk">{{ item.chunkLabel }}</span>
                                <code v-if="item.shortChunkId">{{ item.shortChunkId }}</code>
                            </div>
                            <p>{{ item.snippet }}</p>
                            <div class="used-item-meta">
                                <span v-if="item.elementLabel">{{ item.elementLabel }}</span>
                                <span v-if="item.scoreLabel">{{ item.scoreLabel }}</span>
                            </div>
                        </button>
                    </div>
                    <div v-if="graphPathItems(group).length" class="doc-graph-evidence">
                        <div class="doc-graph-title">Graph 경로</div>
                        <div v-for="path in graphPathItems(group)" :key="path.key" class="doc-graph-path">
                            <t-icon name="share" size="12px" />
                            <span :title="path.label">{{ path.label }}</span>
                        </div>
                    </div>
                </div>
                <div class="doc-group-chunks" v-show="expandedGroups[group.key]">
                    <div v-for="(chunk, cIdx) in group.chunks" :key="'chunk-' + cIdx" class="doc-chunk-item">
                        <t-popup overlayClassName="refer-to-layer" placement="bottom-left" width="400" :showArrow="false" trigger="click">
                            <template #content>
                                <ContentPopup :content="safeProcessContent(chunk.content)" :is-html="true" />
                            </template>
                            <span class="doc-chunk-text">
                                <span class="doc-chunk-index">{{ chunkReferenceLabel(chunk, cIdx) }}</span>
                                {{ truncateContent(chunk.content, 80) }}
                            </span>
                        </t-popup>
                    </div>
                </div>
            </div>
        </div>
        <EvidencePreviewDrawer v-model:visible="evidencePreviewVisible" :group="evidencePreviewGroup" />
    </div>
</template>
<script setup>
import { defineProps, computed, ref, reactive } from "vue";
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { sanitizeHTML } from '@/utils/security';
import ContentPopup from './tool-results/ContentPopup.vue';
import EvidencePreviewDrawer from './EvidencePreviewDrawer.vue';

const router = useRouter();
const { t } = useI18n();

const props = defineProps({
    content: {
        type: String,
        required: false
    },
    session: {
        type: Object,
        required: false
    }
});

const showReferBox = ref(false);
const expandedGroups = reactive({});
const evidencePreviewVisible = ref(false);
const evidencePreviewGroup = ref(null);

const referBoxSwitch = () => {
    showReferBox.value = !showReferBox.value;
};

const toggleGroup = (key) => {
    expandedGroups[key] = !expandedGroups[key];
};

const webSearchRefs = computed(() => {
    if (!props.session?.knowledge_references) return [];
    return props.session.knowledge_references.filter(item => item.chunk_type === 'web_search');
});

const knowledgeRefs = computed(() => {
    if (!props.session?.knowledge_references) return [];
    return props.session.knowledge_references.filter(item => item.chunk_type !== 'web_search');
});

const groupedKnowledgeRefs = computed(() => {
    const refs = knowledgeRefs.value;
    if (!refs.length) return [];

    const groupMap = new Map();
    for (const item of refs) {
        const key = item.knowledge_id || item.knowledge_title || item.id;
        if (!groupMap.has(key)) {
            groupMap.set(key, {
                key,
                title: item.knowledge_title || item.knowledge_filename || key,
                knowledgeId: item.knowledge_id,
                knowledgeBaseId: item.knowledge_base_id,
                citation: item.deep_office_citation || null,
                metadata: item.metadata || {},
                knowledgeSource: item.knowledge_source,
                chunks: [],
            });
        }
        const group = groupMap.get(key);
        if (!group.citation && item.deep_office_citation) {
            group.citation = item.deep_office_citation;
        }
        if (!group.metadata && item.metadata) {
            group.metadata = item.metadata;
        }
        if (!group.knowledgeSource && item.knowledge_source) {
            group.knowledgeSource = item.knowledge_source;
        }
        group.chunks.push(item);
    }
    return Array.from(groupMap.values()).map(group => ({
        ...group,
        citation: aggregateGroupCitation(group) || group.citation,
    }));
});

const headerText = computed(() => {
    const total = props.session?.knowledge_references?.length ?? 0;
    const docCount = groupedKnowledgeRefs.value.length;
    const webCount = webSearchRefs.value.length;
    if (docCount > 0 && webCount > 0) {
        return t('chat.referencesDocAndWebCount', { docCount, webCount });
    }
    if (docCount > 0) {
        return t('chat.referencesDocCount', { count: docCount });
    }
    return t('chat.referencesTitle', { count: total });
});

const safeProcessContent = (content) => {
    if (!content) return '';
    const sanitized = sanitizeHTML(content);
    return sanitized.replace(/\n/g, '<br/>');
};

const truncateContent = (content, maxLen) => {
    if (!content) return '';
    const text = content.replace(/\n/g, ' ').trim();
    if (text.length <= maxLen) return text;
    return text.slice(0, maxLen) + '...';
};

const navigateToDocument = (group) => {
    if (!group.knowledgeBaseId) return;
    const query = {};
    if (group.knowledgeId) {
        query.knowledge_id = group.knowledgeId;
    }
    router.push({
        path: `/platform/knowledge-bases/${group.knowledgeBaseId}`,
        query
    });
};

const getWebSearchUrl = (item) => {
    if (item.metadata?.url) {
        return item.metadata.url;
    }
    if (item.id && (item.id.startsWith('http://') || item.id.startsWith('https://'))) {
        return item.id;
    }
    return '#';
};

const getWebSearchDisplayText = (item) => {
    if (item.knowledge_title) {
        return item.knowledge_title;
    }
    if (item.metadata?.title) {
        return item.metadata.title;
    }
    const url = getWebSearchUrl(item);
    if (url && url !== '#') {
        try {
            const urlObj = new URL(url);
            return urlObj.hostname;
        } catch {
            return url;
        }
    }
    return 'Web Search Result';
};

const sourceDocument = (group) => group?.citation?.source_document || {};
const sourceLocator = (group) => group?.citation?.source_locator || {};
const parserGrounding = (group) => group?.citation?.parser_grounding || {};
const graphEvidence = (group) => group?.citation?.graph_evidence || {};

const aggregateGroupCitation = (group) => {
    const citations = (group?.chunks || [])
        .map(chunk => chunk?.deep_office_citation)
        .filter(Boolean);
    if (!citations.length) return null;

    const sourceDocument = citations[0].source_document || {};
    const sourceLocator = {};
    const parserElementsById = new Map();
    const pages = new Set();
    const bboxes = [];
    const chunkIds = [];
    const evidenceBindingIds = [];
    const chunkEvidence = [];
    const graphEvidences = [];
    let startAt = null;
    let endAt = null;
    let hasPartial = false;

    for (const citation of citations) {
        if (citation.evidence_status && citation.evidence_status !== 'complete') {
            hasPartial = true;
        }
        const locator = citation.source_locator || {};
        for (const [key, value] of Object.entries(locator)) {
            if (value !== undefined && value !== null && value !== '' && !(Array.isArray(value) && value.length === 0)) {
                if (sourceLocator[key] === undefined) {
                    sourceLocator[key] = value;
                }
            }
        }
        if (locator.start_at !== undefined && locator.start_at !== null) {
            const value = Number(locator.start_at);
            if (Number.isFinite(value)) startAt = startAt === null ? value : Math.min(startAt, value);
        }
        if (locator.end_at !== undefined && locator.end_at !== null) {
            const value = Number(locator.end_at);
            if (Number.isFinite(value)) endAt = endAt === null ? value : Math.max(endAt, value);
        }
        if (Array.isArray(locator.weknora_chunk_ids)) {
            chunkIds.push(...locator.weknora_chunk_ids.map(item => String(item)).filter(Boolean));
        }
        if (Array.isArray(locator.evidence_binding_ids)) {
            evidenceBindingIds.push(...locator.evidence_binding_ids.map(item => String(item)).filter(Boolean));
        }
        const grounding = citation.parser_grounding || {};
        if (Array.isArray(grounding.pages)) {
            grounding.pages.forEach(page => {
                const value = Number(page);
                if (Number.isFinite(value) && value > 0) pages.add(value);
            });
        }
        if (Array.isArray(grounding.elements)) {
            grounding.elements.forEach(element => {
                const key = element?.element_id || `${element?.page || ''}:${element?.layout_order || ''}:${parserElementsById.size}`;
                parserElementsById.set(key, element);
            });
        }
        if (Array.isArray(grounding.bboxes)) {
            bboxes.push(...grounding.bboxes);
        }
        if (Array.isArray(citation.chunk_evidence)) {
            chunkEvidence.push(...citation.chunk_evidence);
        }
        if (citation.graph_evidence) {
            graphEvidences.push(citation.graph_evidence);
        }
    }
    for (const chunk of group?.chunks || []) {
        if (chunk?.graph_evidence) {
            graphEvidences.push(chunk.graph_evidence);
        }
    }

    if (startAt !== null) sourceLocator.start_at = startAt;
    if (endAt !== null) sourceLocator.end_at = endAt;
    sourceLocator.weknora_chunk_ids = [...new Set(chunkIds)];
    sourceLocator.evidence_binding_ids = [...new Set(evidenceBindingIds)];

    return {
        evidence_status: hasPartial ? 'partial' : 'complete',
        source_document: sourceDocument,
        source_locator: sourceLocator,
        parser_grounding: {
            ...(citations[0].parser_grounding || {}),
            pages: [...pages].sort((a, b) => a - b),
            elements: Array.from(parserElementsById.values()),
            bboxes,
        },
        chunk_evidence: chunkEvidence,
        graph_evidence: aggregateGraphEvidence(graphEvidences),
    };
};

const aggregateGraphEvidence = (items) => {
    const evidences = (items || []).filter(Boolean);
    if (!evidences.length) return null;

    const queryEntities = new Set();
    const supportChunkIds = new Set();
    const nodeMap = new Map();
    const relationMap = new Map();
    const pathMap = new Map();

    for (const evidence of evidences) {
        if (Array.isArray(evidence.query_entities)) {
            evidence.query_entities.map(item => String(item || '')).filter(Boolean).forEach(item => queryEntities.add(item));
        }
        if (Array.isArray(evidence.support_chunk_ids)) {
            evidence.support_chunk_ids.map(item => String(item || '')).filter(Boolean).forEach(item => supportChunkIds.add(item));
        }
        if (Array.isArray(evidence.matched_nodes)) {
            for (const node of evidence.matched_nodes) {
                const name = String(node?.name || '');
                if (!name || nodeMap.has(name)) continue;
                nodeMap.set(name, node);
            }
        }
        if (Array.isArray(evidence.relationships)) {
            for (const relation of evidence.relationships) {
                const key = [relation?.source, relation?.relation, relation?.target].map(item => String(item || '')).join('|');
                if (!key.replace(/\|/g, '') || relationMap.has(key)) continue;
                relationMap.set(key, relation);
            }
        }
        if (Array.isArray(evidence.paths)) {
            for (const path of evidence.paths) {
                const label = graphPathLabel(path);
                if (!label || pathMap.has(label)) continue;
                pathMap.set(label, path);
            }
        }
    }

    return {
        match_type: 'graph',
        query_entities: [...queryEntities],
        matched_nodes: [...nodeMap.values()],
        relationships: [...relationMap.values()],
        paths: [...pathMap.values()],
        support_chunk_ids: [...supportChunkIds],
    };
};

const hasEvidence = (group) => Boolean(group?.citation || sourceUri(group) || checksum(group));

const canOpenEvidencePreview = (group) => Boolean(group?.knowledgeId && hasEvidence(group));

const openEvidencePreview = (group) => {
    evidencePreviewGroup.value = group;
    evidencePreviewVisible.value = true;
};

const openChunkEvidencePreview = (group, chunk) => {
    evidencePreviewGroup.value = {
        ...group,
        citation: chunk?.deep_office_citation || group?.citation || null,
        chunks: chunk ? [chunk] : group?.chunks || [],
    };
    evidencePreviewVisible.value = true;
};

const chunkReferenceLabel = (chunk, index) => {
    const pages = citationPages(chunk?.deep_office_citation);
    const pagePart = pages.length ? `p.${compactPageLabel(pages)}` : '';
    const chunkPart = chunk?.chunk_index !== undefined && chunk?.chunk_index !== null
        ? `chunk ${chunk.chunk_index}`
        : `chunk ${index + 1}`;
    return [pagePart, chunkPart].filter(Boolean).join(' / ');
};

const chunkEvidenceItems = (group) => {
    const chunks = Array.isArray(group?.chunks) ? group.chunks : [];
    return chunks
        .map((chunk, index) => {
            const citation = chunk?.deep_office_citation || null;
            const pages = citationPages(citation);
            const chunkIds = citationChunkIds(citation, chunk);
            const elements = citationElements(citation);
            const bboxes = citationBboxes(citation);
            return {
                key: `${chunk?.id || index}-${index}`,
                rank: index + 1,
                reference: chunk,
                pageLabel: pages.length ? `p.${compactPageLabel(pages)}` : '',
                chunkLabel: chunk?.chunk_index !== undefined && chunk?.chunk_index !== null ? `chunk ${chunk.chunk_index}` : '',
                shortChunkId: chunkIds.length ? shortId(chunkIds[0]) : shortId(chunk?.id),
                snippet: truncateContent(chunk?.content || chunk?.matched_content || '', 130) || '청크 텍스트 없음',
                elementLabel: elements.length || bboxes.length ? `elements ${elements.length} / bboxes ${bboxes.length}` : '',
                scoreLabel: typeof chunk?.score === 'number' ? `score ${chunk.score.toFixed(3)}` : '',
            };
        })
        .filter(item => item.snippet || item.pageLabel || item.chunkLabel || item.shortChunkId);
};

const citationPages = (citation) => {
    const pages = citation?.parser_grounding?.pages;
    if (!Array.isArray(pages)) return [];
    return [...new Set(
        pages
            .map(page => Number(page))
            .filter(page => Number.isFinite(page) && page > 0)
    )].sort((a, b) => a - b);
};

const citationChunkIds = (citation, chunk) => {
    const ids = citation?.source_locator?.weknora_chunk_ids;
    if (Array.isArray(ids) && ids.length) return ids.map(item => String(item)).filter(Boolean);
    const fallback = [chunk?.id, ...(Array.isArray(chunk?.sub_chunk_id) ? chunk.sub_chunk_id : [])];
    return fallback.map(item => String(item || '')).filter(Boolean);
};

const citationElements = (citation) => {
    const elements = citation?.parser_grounding?.elements;
    return Array.isArray(elements) ? elements : [];
};

const citationBboxes = (citation) => {
    const bboxes = citation?.parser_grounding?.bboxes;
    return Array.isArray(bboxes) ? bboxes : [];
};

const compactPageLabel = (pages) => {
    if (!pages.length) return '';
    if (pages.length <= 3) return pages.join(', ');
    return `${pages[0]}-${pages[pages.length - 1]}`;
};

const shortId = (value) => {
    const text = String(value || '');
    if (text.length <= 12) return text;
    return `${text.slice(0, 8)}...${text.slice(-4)}`;
};

const evidenceStatus = (group) => group?.citation?.evidence_status || 'metadata';

const evidenceStatusLabel = (group) => {
    const status = evidenceStatus(group);
    if (status === 'complete') return '완전 매핑';
    if (status === 'partial') return '부분 매핑';
    return '메타데이터';
};

const sourceUri = (group) => (
    sourceDocument(group).uri
    || group?.metadata?.source_uri
    || group?.knowledgeSource
    || ''
);

const checksum = (group) => sourceDocument(group).checksum || group?.metadata?.checksum || '';

const pageLabel = (group) => {
    const pages = parserGrounding(group).pages;
    if (!Array.isArray(pages) || pages.length === 0) return '';
    if (pages.length <= 3) return pages.join(', ');
    return `${pages[0]}-${pages[pages.length - 1]}`;
};

const parserSummary = (group) => {
    const grounding = parserGrounding(group);
    const elements = Array.isArray(grounding.elements) ? grounding.elements.length : 0;
    const bboxes = Array.isArray(grounding.bboxes) ? grounding.bboxes.length : 0;
    if (!elements && !bboxes) return '';
    if (elements && bboxes) return `elements ${elements} / bboxes ${bboxes}`;
    if (elements) return `elements ${elements}`;
    return `bboxes ${bboxes}`;
};

const chunkSummary = (group) => {
    const ids = sourceLocator(group).weknora_chunk_ids;
    if (!Array.isArray(ids) || ids.length === 0) return '';
    return `${ids.length}개`;
};

const graphSummary = (group) => {
    const evidence = graphEvidence(group);
    const nodes = Array.isArray(evidence.matched_nodes) ? evidence.matched_nodes.length : 0;
    const paths = Array.isArray(evidence.paths) ? evidence.paths.length : 0;
    if (!nodes && !paths) return '';
    return `nodes ${nodes} / paths ${paths}`;
};

const graphPathItems = (group) => {
    const paths = graphEvidence(group).paths;
    if (!Array.isArray(paths)) return [];
    return paths
        .map((path, index) => ({
            key: `${graphPathLabel(path)}-${index}`,
            label: graphPathLabel(path),
        }))
        .filter(item => item.label)
        .slice(0, 3);
};

const graphPathLabel = (path) => {
    const steps = Array.isArray(path?.path) ? path.path : [];
    if (!steps.length) return '';
    return steps
        .map(step => step?.node || step?.relation || '')
        .filter(Boolean)
        .join(' -> ');
};
</script>
<style lang="less" scoped>
.refer {
    display: flex;
    flex-direction: column;
    font-size: 12px;
    width: 100%;
    border-radius: 8px;
    background-color: var(--td-bg-color-container);
    border: .5px solid var(--td-component-stroke);
    box-shadow: 0 2px 4px rgba(37, 99, 235, 0.08);
    box-sizing: border-box;
    overflow: hidden;
    box-sizing: border-box;
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
    margin-bottom: 8px;

    .refer_header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 6px 14px;
        color: var(--td-text-color-primary);
        font-weight: 500;

        .refer_title {
            display: flex;
            align-items: center;

            img {
                width: 16px;
                height: 16px;
                color: var(--td-brand-color);
                fill: currentColor;
                margin-right: 8px;
            }

            span {
                white-space: nowrap;
                font-size: 12px;
            }
        }

        .refer_show_icon {
            font-size: 14px;
            padding: 0 2px 1px 2px;
            color: var(--td-brand-color);
        }
    }

    .refer_header:hover {
        background-color: rgba(37, 99, 235, 0.04);
        cursor: pointer;
    }

    .refer_box {
        padding: 4px 14px 8px 14px;
        flex-direction: column;
        border-top: 1px solid var(--td-bg-color-secondarycontainer);
    }
}

.doc {
    text-decoration: none;
    color: var(--td-brand-color);
    cursor: pointer;
    display: inline-block;
    white-space: nowrap;
    max-width: calc(100% - 24px);
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 20px;
    padding: 2px 0;
    transition: all 0.2s ease;
    border-bottom: 1px dashed var(--td-brand-color);

    &:hover {
        background-color: rgba(37, 99, 235, 0.08);
        border-radius: 3px;
        padding-right: 4px;
    }

    &.doc-web {
        white-space: normal;
        word-break: break-all;
    }
}

.doc-group {
    margin-top: 4px;

    .doc-group-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 4px 4px;
        border-radius: 4px;
        cursor: pointer;
        transition: background-color 0.15s ease;

        &:hover {
            background-color: rgba(37, 99, 235, 0.04);
        }

        .doc-group-left {
            display: flex;
            align-items: center;
            min-width: 0;
            flex: 1;
        }

        .doc-group-arrow {
            color: var(--td-text-color-placeholder);
            flex-shrink: 0;
            margin-right: 2px;
        }

        .doc-group-icon {
            color: var(--td-brand-color);
            flex-shrink: 0;
            margin-right: 6px;
        }

        .doc-group-title {
            color: var(--td-text-color-primary);
            font-weight: 500;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            max-width: 200px;
        }

        .doc-group-count {
            color: var(--td-text-color-placeholder);
            font-size: 11px;
            margin-left: 6px;
            white-space: nowrap;
            flex-shrink: 0;
        }

        .doc-group-actions {
            flex-shrink: 0;
            margin-left: 8px;
        }

        .doc-group-navigate {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            width: 22px;
            height: 22px;
            border-radius: 4px;
            color: var(--td-brand-color);
            cursor: pointer;
            transition: all 0.15s ease;

            &:hover {
                background-color: var(--td-brand-color-light);
            }
        }
    }

    .doc-group-chunks {
        padding-left: 22px;
    }

    .doc-evidence-card {
        margin: 2px 0 6px 22px;
        padding: 7px 8px;
        border: 1px solid var(--td-component-stroke);
        border-radius: 6px;
        background-color: var(--td-bg-color-secondarycontainer);
    }

    .doc-evidence-title {
        display: flex;
        align-items: center;
        gap: 5px;
        color: var(--td-text-color-primary);
        font-weight: 600;
        line-height: 18px;

        .t-icon {
            color: var(--td-brand-color);
            flex-shrink: 0;
        }
    }

    .doc-evidence-status {
        margin-left: auto;
        padding: 0 6px;
        border-radius: 999px;
        color: var(--td-success-color);
        background-color: var(--td-success-color-light);
        font-size: 11px;
        font-weight: 500;
        white-space: nowrap;

        &.partial {
            color: var(--td-warning-color);
            background-color: var(--td-warning-color-light);
        }
    }

    .doc-evidence-open {
        flex-shrink: 0;
        height: 22px;
        margin-left: 2px;
        padding: 0 4px;
        font-size: 11px;
    }

    .doc-evidence-grid {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 4px 8px;
        margin-top: 5px;
    }

    .doc-evidence-item {
        display: flex;
        min-width: 0;
        line-height: 17px;
    }

    .doc-evidence-label {
        flex-shrink: 0;
        width: 46px;
        color: var(--td-text-color-placeholder);
        font-size: 11px;
    }

    .doc-evidence-value {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--td-text-color-secondary);
        font-size: 11px;
    }

    .doc-evidence-used {
        margin-top: 8px;
        padding-top: 7px;
        border-top: 1px solid var(--td-component-stroke);
    }

    .doc-evidence-used-title {
        margin-bottom: 5px;
        color: var(--td-text-color-primary);
        font-size: 11px;
        font-weight: 700;
    }

    .doc-evidence-used-item {
        display: block;
        width: 100%;
        padding: 7px;
        margin-top: 5px;
        border: 1px solid var(--td-component-stroke);
        border-radius: 6px;
        background: var(--td-bg-color-container);
        color: inherit;
        text-align: left;
        cursor: pointer;
        transition: border-color 0.15s ease, background-color 0.15s ease;

        &:hover {
            border-color: var(--td-brand-color);
            background-color: rgba(37, 99, 235, 0.04);
        }

        p {
            margin: 5px 0 0 0;
            color: var(--td-text-color-secondary);
            font-size: 11px;
            line-height: 16px;
            display: -webkit-box;
            -webkit-line-clamp: 2;
            -webkit-box-orient: vertical;
            overflow: hidden;
        }
    }

    .used-item-head,
    .used-item-meta {
        display: flex;
        align-items: center;
        gap: 5px;
        min-width: 0;
    }

    .used-item-rank {
        color: var(--td-text-color-primary);
        font-weight: 700;
        white-space: nowrap;
    }

    .used-item-page,
    .used-item-chunk {
        padding: 0 5px;
        border-radius: 999px;
        background: var(--td-brand-color-light);
        color: var(--td-brand-color);
        font-size: 10px;
        line-height: 16px;
        white-space: nowrap;
    }

    .used-item-head code {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        color: var(--td-text-color-placeholder);
        font-size: 10px;
        font-weight: 400;
        white-space: nowrap;
    }

    .used-item-meta {
        margin-top: 5px;
        color: var(--td-text-color-placeholder);
        font-size: 10px;
    }

    .doc-graph-evidence {
        margin-top: 8px;
        padding-top: 7px;
        border-top: 1px solid var(--td-component-stroke);
    }

    .doc-graph-title {
        margin-bottom: 5px;
        color: var(--td-text-color-primary);
        font-size: 11px;
        font-weight: 700;
    }

    .doc-graph-path {
        display: flex;
        align-items: center;
        gap: 5px;
        min-width: 0;
        margin-top: 4px;
        color: var(--td-text-color-secondary);
        font-size: 11px;
        line-height: 17px;

        .t-icon {
            flex-shrink: 0;
            color: var(--td-brand-color);
        }

        span {
            min-width: 0;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
    }
}

.doc-chunk-item {
    .doc-chunk-text {
        display: block;
        color: var(--td-text-color-secondary);
        font-size: 12px;
        line-height: 18px;
        padding: 3px 6px;
        border-radius: 4px;
        cursor: pointer;
        transition: background-color 0.15s ease;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;

        &:hover {
            background-color: rgba(37, 99, 235, 0.04);
            color: var(--td-brand-color);
        }

        .doc-chunk-index {
            color: var(--td-text-color-placeholder);
            font-size: 11px;
            margin-right: 4px;
        }
    }
}
</style>

<style>
.refer-to-layer {
    width: 400px;
    max-width: 500px;

    .t-popup__content {
        max-height: 400px;
        max-width: 500px;
        overflow-y: auto;
        overflow-x: hidden;
        word-wrap: break-word;
        word-break: break-word;
    }
}
</style>
