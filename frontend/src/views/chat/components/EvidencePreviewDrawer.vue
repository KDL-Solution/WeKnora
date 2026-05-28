<template>
  <t-drawer
    v-model:visible="visibleModel"
    :footer="false"
    :close-btn="true"
    size="calc(100vw - 48px)"
    :z-index="2300"
    class="evidence-preview-drawer"
  >
    <template #header>
      <div class="evidence-drawer-header">
        <span class="evidence-drawer-title" :title="displayTitle">{{ displayTitle }}</span>
        <t-tag v-if="sourceTypeLabel" size="small" theme="success" variant="light">{{ sourceTypeLabel }}</t-tag>
      </div>
    </template>

    <div class="evidence-preview-layout">
      <section class="evidence-document-pane">
        <div class="document-pane-toolbar">
          <div class="document-pane-meta">
            <t-icon name="file" size="15px" />
            <span class="document-name" :title="fileName">{{ fileName }}</span>
          </div>
          <div class="document-pane-tags">
            <t-tag v-if="primaryPage" size="small" theme="primary" variant="light">
              페이지 {{ primaryPage }}
            </t-tag>
            <t-tag v-if="pageLabel" size="small" theme="default" variant="light">
              {{ pageLabel }}
            </t-tag>
          </div>
        </div>

        <DocumentPreview
          v-if="canRenderDocument"
          :knowledge-id="knowledgeId"
          :file-type="fileType"
          :file-name="fileName"
          :initial-page="primaryPage"
          :pdf-pages="pages"
          :pdf-bboxes="bboxes"
          :pdf-elements="elements"
          :active="visibleModel"
        />
        <div v-else class="document-preview-empty">
          <t-icon name="file-unknown" size="42px" />
          <div>이 출처는 아직 문서 preview로 열 수 없습니다.</div>
          <span>{{ sourceUri || displayTitle }}</span>
        </div>
      </section>

      <aside class="evidence-detail-pane">
        <section class="evidence-section">
          <div class="section-title">출처 문서</div>
          <div class="field-row">
            <span>문서</span>
            <strong :title="displayTitle">{{ displayTitle }}</strong>
          </div>
          <div class="field-row" v-if="sourceUri">
            <span>URI</span>
            <strong :title="sourceUri">{{ sourceUri }}</strong>
          </div>
          <div class="field-row" v-if="checksum">
            <span>해시</span>
            <strong :title="checksum">{{ checksum }}</strong>
          </div>
          <div class="field-row" v-if="evidenceStatusLabel">
            <span>상태</span>
            <strong>{{ evidenceStatusLabel }}</strong>
          </div>
        </section>

        <section class="evidence-section">
          <div class="section-title">문서 위치</div>
          <div class="locator-grid">
            <div>
              <span>페이지</span>
              <strong>{{ pageLabel || '-' }}</strong>
            </div>
            <div>
              <span>Parser elements</span>
              <strong>{{ elements.length }}</strong>
            </div>
            <div>
              <span>BBox</span>
              <strong>{{ bboxes.length }}</strong>
            </div>
            <div>
              <span>RAG 청크</span>
              <strong>{{ chunkIds.length }}</strong>
            </div>
          </div>
        </section>

        <section v-if="graphPaths.length || graphNodes.length" class="evidence-section">
          <div class="section-title">Graph 경로</div>
          <div v-if="graphPaths.length" class="graph-evidence-list">
            <div v-for="item in graphPaths" :key="item.key" class="graph-path-item">
              <t-icon name="share" size="13px" />
              <span :title="item.label">{{ item.label }}</span>
            </div>
          </div>
          <div v-if="graphNodes.length" class="graph-node-list">
            <span v-for="node in graphNodes" :key="node.name" :title="node.title">{{ node.name }}</span>
          </div>
          <div v-if="graphSupportChunkIds.length" class="graph-support">
            연결 청크 {{ graphSupportChunkIds.length }}개
          </div>
        </section>

        <section class="evidence-section">
          <div class="section-title">답변에 사용된 청크</div>
          <div v-if="chunkEvidenceItems.length" class="chunk-evidence-list">
            <div v-for="item in chunkEvidenceItems" :key="item.key" class="chunk-evidence-item">
              <div class="chunk-evidence-head">
                <span>{{ item.title }}</span>
                <div class="chunk-evidence-tags">
                  <span v-if="item.pageLabel">{{ item.pageLabel }}</span>
                  <span v-if="item.chunkLabel">{{ item.chunkLabel }}</span>
                  <code v-if="item.shortChunkId">{{ item.shortChunkId }}</code>
                </div>
              </div>
              <p>{{ item.snippet }}</p>
              <div class="chunk-evidence-meta">
                <span v-if="item.charRange">{{ item.charRange }}</span>
                <span v-if="item.elementLabel">{{ item.elementLabel }}</span>
                <span v-if="item.scoreLabel">{{ item.scoreLabel }}</span>
              </div>
            </div>
          </div>
          <div v-else class="empty-note">표시할 청크 텍스트가 없습니다.</div>
        </section>

        <section class="evidence-section">
          <div class="section-title">Parser grounding</div>
          <div v-if="elements.length" class="element-list">
            <div v-for="element in previewElements" :key="element.element_id" class="element-item">
              <div>
                <strong>{{ element.element_id }}</strong>
                <span>{{ element.raw_type || element.type || 'element' }}</span>
              </div>
              <code>{{ elementLocatorLabel(element) }}</code>
            </div>
            <div v-if="elements.length > previewElements.length" class="empty-note">
              외 {{ elements.length - previewElements.length }}개 element
            </div>
          </div>
          <div v-else class="empty-note">Parser element 정보가 없습니다.</div>
        </section>
      </aside>
    </div>
  </t-drawer>
</template>

<script setup>
import { computed } from 'vue';
import DocumentPreview from '@/components/document-preview.vue';

const props = defineProps({
  visible: {
    type: Boolean,
    default: false,
  },
  group: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(['update:visible']);

const visibleModel = computed({
  get: () => props.visible,
  set: value => emit('update:visible', value),
});

const citation = computed(() => props.group?.citation || {});
const sourceDocument = computed(() => citation.value.source_document || {});
const sourceLocator = computed(() => citation.value.source_locator || {});
const parserGrounding = computed(() => citation.value.parser_grounding || {});

const displayTitle = computed(() => (
  sourceDocument.value.title
  || props.group?.title
  || props.group?.knowledgeId
  || '근거 문서'
));

const knowledgeId = computed(() => (
  props.group?.knowledgeId
  || sourceLocator.value.knowledge_id
  || ''
));

const fileName = computed(() => (
  sourceDocument.value.file_name
  || props.group?.chunks?.[0]?.knowledge_filename
  || `${displayTitle.value}.pdf`
));

const fileType = computed(() => {
  const explicit = props.group?.chunks?.[0]?.metadata?.file_type || sourceDocument.value.file_type;
  if (explicit) return String(explicit).toLowerCase();
  const name = String(fileName.value || '');
  const match = name.match(/\.([^.]+)$/);
  return match ? match[1].toLowerCase() : 'pdf';
});

const canRenderDocument = computed(() => Boolean(knowledgeId.value && fileType.value));
const sourceUri = computed(() => sourceDocument.value.uri || props.group?.metadata?.source_uri || props.group?.knowledgeSource || '');
const checksum = computed(() => sourceDocument.value.checksum || props.group?.metadata?.checksum || '');
const sourceTypeLabel = computed(() => sourceDocument.value.source_type || '');

const evidenceStatusLabel = computed(() => {
  const status = citation.value.evidence_status;
  if (status === 'complete') return '완전 매핑';
  if (status === 'partial') return '부분 매핑';
  if (status) return String(status);
  return '';
});

const pages = computed(() => {
  const rawPages = parserGrounding.value.pages;
  if (!Array.isArray(rawPages)) return [];
  return rawPages
    .map(page => Number(page))
    .filter(page => Number.isFinite(page) && page > 0);
});

const primaryPage = computed(() => pages.value[0] || 0);

const pageLabel = computed(() => {
  if (!pages.value.length) return '';
  if (pages.value.length <= 3) return `페이지 ${pages.value.join(', ')}`;
  return `페이지 ${pages.value[0]}-${pages.value[pages.value.length - 1]}`;
});

const elements = computed(() => Array.isArray(parserGrounding.value.elements) ? parserGrounding.value.elements : []);
const bboxes = computed(() => Array.isArray(parserGrounding.value.bboxes) ? parserGrounding.value.bboxes : []);
const chunkIds = computed(() => Array.isArray(sourceLocator.value.weknora_chunk_ids) ? sourceLocator.value.weknora_chunk_ids : []);
const chunks = computed(() => Array.isArray(props.group?.chunks) ? props.group.chunks : []);
const previewElements = computed(() => elements.value.slice(0, 10));
const graphEvidence = computed(() => (
  citation.value.graph_evidence
  || chunks.value.find(chunk => chunk?.graph_evidence)?.graph_evidence
  || {}
));
const graphPaths = computed(() => {
  const paths = graphEvidence.value.paths;
  if (!Array.isArray(paths)) return [];
  return paths
    .map((path, index) => ({
      key: `${graphPathLabel(path)}-${index}`,
      label: graphPathLabel(path),
    }))
    .filter(item => item.label)
    .slice(0, 8);
});
const graphNodes = computed(() => {
  const nodes = graphEvidence.value.matched_nodes;
  if (!Array.isArray(nodes)) return [];
  return nodes
    .map(node => ({
      name: String(node?.name || ''),
      title: Array.isArray(node?.attributes) ? node.attributes.join(', ') : '',
    }))
    .filter(node => node.name)
    .slice(0, 16);
});
const graphSupportChunkIds = computed(() => {
  const ids = graphEvidence.value.support_chunk_ids;
  if (!Array.isArray(ids)) return [];
  return ids.map(item => String(item || '')).filter(Boolean);
});

const chunkEvidenceItems = computed(() => chunks.value.map((chunk, index) => {
  const itemCitation = chunk?.deep_office_citation || citation.value;
  const itemPages = pagesFromCitation(itemCitation);
  const itemElements = elementsFromCitation(itemCitation);
  const itemBboxes = bboxesFromCitation(itemCitation);
  const itemChunkIds = chunkIdsFromCitation(itemCitation, chunk);
  const start = chunk?.start_at ?? itemCitation?.source_locator?.start_at;
  const end = chunk?.end_at ?? itemCitation?.source_locator?.end_at;
  return {
    key: `${chunk?.id || index}-${index}`,
    title: `근거 ${index + 1}`,
    pageLabel: itemPages.length ? `p.${compactPageLabel(itemPages)}` : '',
    chunkLabel: chunk?.chunk_index !== undefined && chunk?.chunk_index !== null ? `chunk ${chunk.chunk_index}` : '',
    shortChunkId: itemChunkIds.length ? shortId(itemChunkIds[0]) : shortId(chunk?.id),
    snippet: truncateText(chunk?.content || chunk?.matched_content || '', 360) || '청크 텍스트 없음',
    charRange: start !== undefined && end !== undefined ? `chars ${start}-${end}` : '',
    elementLabel: itemElements.length || itemBboxes.length ? `elements ${itemElements.length} / bboxes ${itemBboxes.length}` : '',
    scoreLabel: typeof chunk?.score === 'number' ? `score ${chunk.score.toFixed(3)}` : '',
  };
}));

const shortId = value => {
  const text = String(value || '');
  if (text.length <= 12) return text;
  return `${text.slice(0, 8)}...${text.slice(-4)}`;
};

const truncateText = (value, maxLength) => {
  const text = String(value || '').replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength)}...`;
};

const pagesFromCitation = value => {
  const raw = value?.parser_grounding?.pages;
  if (!Array.isArray(raw)) return [];
  return [...new Set(
    raw
      .map(page => Number(page))
      .filter(page => Number.isFinite(page) && page > 0)
  )].sort((a, b) => a - b);
};

const elementsFromCitation = value => {
  const raw = value?.parser_grounding?.elements;
  return Array.isArray(raw) ? raw : [];
};

const bboxesFromCitation = value => {
  const raw = value?.parser_grounding?.bboxes;
  return Array.isArray(raw) ? raw : [];
};

const chunkIdsFromCitation = (value, chunk) => {
  const raw = value?.source_locator?.weknora_chunk_ids;
  if (Array.isArray(raw) && raw.length) return raw.map(item => String(item)).filter(Boolean);
  const fallback = [chunk?.id, ...(Array.isArray(chunk?.sub_chunk_id) ? chunk.sub_chunk_id : [])];
  return fallback.map(item => String(item || '')).filter(Boolean);
};

const compactPageLabel = value => {
  if (!value.length) return '';
  if (value.length <= 3) return value.join(', ');
  return `${value[0]}-${value[value.length - 1]}`;
};

const elementLocatorLabel = element => {
  const page = element?.page ? `p.${element.page}` : 'p.-';
  const order = element?.layout_order !== undefined ? `order ${element.layout_order}` : '';
  if (Array.isArray(element?.bbox)) {
    const bbox = element.bbox.map(item => Number(item).toFixed(3)).join(', ');
    return `${page} ${order} [${bbox}]`.trim();
  }
  return `${page} ${order}`.trim();
};

const graphPathLabel = path => {
  const steps = Array.isArray(path?.path) ? path.path : [];
  if (!steps.length) return '';
  return steps
    .map(step => step?.node || step?.relation || '')
    .filter(Boolean)
    .join(' -> ');
};
</script>

<style scoped lang="less">
.evidence-drawer-header {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.evidence-drawer-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}

.evidence-preview-drawer {
  :deep(.t-drawer__content-wrapper) {
    width: calc(100vw - 48px) !important;
    max-width: none !important;
  }

  :deep(.t-drawer__body) {
    padding: 0;
    overflow: hidden;
  }
}

.evidence-preview-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) clamp(320px, 31vw, 420px);
  height: calc(100vh - 58px);
  min-height: 0;
  overflow: hidden;
}

.evidence-document-pane {
  min-width: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-page);
}

.document-pane-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.document-pane-meta,
.document-pane-tags {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 8px;
}

.document-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.evidence-document-pane :deep(.document-preview) {
  flex: 1;
  min-height: 0;
  height: 100%;
  border: 0;
  border-radius: 0;
  background: var(--td-bg-color-container);
}

.evidence-document-pane :deep(.preview-pdf) {
  height: 100%;
  min-height: 0;
}

.document-preview-empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--td-text-color-placeholder);
  text-align: center;

  span {
    max-width: 70%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--td-text-color-secondary);
  }
}

.evidence-detail-pane {
  min-width: 0;
  overflow-y: auto;
  padding: 14px;
  background: var(--td-bg-color-container);
}

.evidence-section {
  padding-bottom: 14px;
  margin-bottom: 14px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.section-title {
  margin-bottom: 8px;
  color: var(--td-text-color-primary);
  font-weight: 700;
}

.field-row {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  gap: 8px;
  align-items: center;
  line-height: 22px;
  font-size: 12px;

  span {
    color: var(--td-text-color-placeholder);
  }

  strong {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--td-text-color-secondary);
    font-weight: 500;
  }
}

.locator-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;

  div {
    padding: 8px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    background: var(--td-bg-color-secondarycontainer);
  }

  span,
  strong {
    display: block;
  }

  span {
    color: var(--td-text-color-placeholder);
    font-size: 11px;
  }

  strong {
    margin-top: 3px;
    color: var(--td-text-color-primary);
    font-size: 13px;
  }
}

.chunk-evidence-list,
.element-list,
.graph-evidence-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chunk-evidence-item,
.element-item,
.graph-path-item {
  padding: 9px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
}

.chunk-evidence-head,
.element-item > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 5px;
  color: var(--td-text-color-primary);
  font-size: 12px;
  font-weight: 600;
}

.chunk-evidence-tags {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;

  span {
    padding: 0 6px;
    border-radius: 999px;
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    font-size: 11px;
    font-weight: 500;
    white-space: nowrap;
  }
}

.chunk-evidence-head code,
.element-item code {
  color: var(--td-text-color-placeholder);
  font-size: 11px;
  font-weight: 400;
}

.chunk-evidence-item p {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 18px;
}

.graph-path-item {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 18px;

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

.graph-node-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;

  span {
    max-width: 100%;
    padding: 2px 7px;
    border-radius: 999px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    font-size: 11px;
  }
}

.graph-support {
  margin-top: 8px;
  color: var(--td-text-color-placeholder);
  font-size: 11px;
}

.chunk-evidence-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 7px;
  color: var(--td-text-color-placeholder);
  font-size: 11px;
}

.element-item {
  code {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    color: var(--td-text-color-placeholder);
    font-weight: 400;
  }
}

.empty-note {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  line-height: 18px;
}

@media (max-width: 920px) {
  .evidence-preview-layout {
    grid-template-columns: 1fr;
    height: auto;
  }

  .evidence-document-pane {
    height: 62vh;
    border-right: 0;
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .evidence-detail-pane {
    max-height: 46vh;
  }
}
</style>
