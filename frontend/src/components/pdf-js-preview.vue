// @ts-nocheck
<script setup lang="ts">
// @ts-nocheck
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import * as pdfjsLib from 'pdfjs-dist';
import PdfJsWorker from 'pdfjs-dist/build/pdf.worker.mjs?worker';
import { MessagePlugin } from 'tdesign-vue-next';

if (!pdfjsLib.GlobalWorkerOptions.workerPort) {
  pdfjsLib.GlobalWorkerOptions.workerPort = new PdfJsWorker();
}

const props = defineProps<{
  blobUrl: string;
  pdfData?: ArrayBuffer | null;
  fileName: string;
  initialPage?: number | string;
  pages?: number[];
  bboxes?: Array<Record<string, unknown>>;
  elements?: Array<Record<string, unknown>>;
}>();

const loading = ref(false);
const error = ref('');
const totalPages = ref(0);
const zoom = ref(1);
const renderedPages = ref<Array<{ pageNumber: number; width: number; height: number }>>([]);
const selectedElementId = ref('');
const scrollContainer = ref<HTMLElement | null>(null);
const rendering = ref(false);
const pdfDocument = shallowRef<any>(null);
const canvasRefs = new Map<number, HTMLCanvasElement>();
let renderToken = 0;
let resizeTimer: number | undefined;

function animationFrame() {
  return new Promise<void>(resolve => window.requestAnimationFrame(() => resolve()));
}

const requestedPages = computed(() => {
  const explicitPages = Array.isArray(props.pages) ? props.pages : [];
  const initialPage = Number(props.initialPage || 0);
  const pages = explicitPages
    .concat(Number.isFinite(initialPage) && initialPage > 0 ? [initialPage] : [])
    .map(page => Number(page))
    .filter(page => Number.isFinite(page) && page > 0);
  return Array.from(new Set(pages)).sort((left, right) => left - right);
});

const pageSummary = computed(() => {
  if (!renderedPages.value.length) return '';
  if (totalPages.value <= 0) return `${renderedPages.value.length}쪽`;
  const pages = renderedPages.value.map(page => page.pageNumber);
  if (pages.length === totalPages.value) return `전체 ${totalPages.value}쪽`;
  return `${pages.join(', ')}쪽 / 전체 ${totalPages.value}쪽`;
});

const elementById = computed(() => {
  const mapped = new Map<string, Record<string, unknown>>();
  (props.elements || []).forEach(element => {
    const id = String(element?.element_id || '');
    if (id) mapped.set(id, element);
  });
  return mapped;
});

const overlayBoxesByPage = computed(() => {
  const mapped = new Map<number, Array<Record<string, unknown>>>();
  const sourceBoxes = Array.isArray(props.bboxes) && props.bboxes.length
    ? props.bboxes
    : (props.elements || []).filter(element => Array.isArray(element?.bbox));

  sourceBoxes.forEach((box, index) => {
    const elementId = String(box?.element_id || '');
    const element = elementId ? elementById.value.get(elementId) : null;
    const rawBbox = Array.isArray(box?.bbox) ? box.bbox : element?.bbox;
    const bbox = normaliseBbox(rawBbox);
    if (!bbox) return;

    const page = Number(box?.page || element?.page || 0);
    if (!Number.isFinite(page) || page <= 0) return;

    const type = String(element?.raw_type || element?.type || 'element');
    const label = elementId ? `${elementId} · ${type}` : `bbox ${index + 1}`;
    const pageBoxes = mapped.get(page) || [];
    pageBoxes.push({
      elementId,
      label,
      bbox,
      type,
    });
    mapped.set(page, pageBoxes);
  });

  return mapped;
});

function normaliseBbox(raw: unknown) {
  if (!Array.isArray(raw) || raw.length < 4) return null;
  const numbers = raw.slice(0, 4).map(value => Number(value));
  if (numbers.some(value => !Number.isFinite(value))) return null;

  const maxValue = Math.max(...numbers.map(Math.abs));
  if (maxValue > 1.5) return null;

  const x1 = clamp(Math.min(numbers[0], numbers[2]));
  const y1 = clamp(Math.min(numbers[1], numbers[3]));
  const x2 = clamp(Math.max(numbers[0], numbers[2]));
  const y2 = clamp(Math.max(numbers[1], numbers[3]));
  if (x2 <= x1 || y2 <= y1) return null;

  return { x1, y1, x2, y2 };
}

function clamp(value: number) {
  return Math.min(1, Math.max(0, value));
}

function bboxStyle(box: Record<string, unknown>) {
  const bbox = box.bbox;
  return {
    left: `${bbox.x1 * 100}%`,
    top: `${bbox.y1 * 100}%`,
    width: `${(bbox.x2 - bbox.x1) * 100}%`,
    height: `${(bbox.y2 - bbox.y1) * 100}%`,
  };
}

function boxesForPage(pageNumber: number) {
  return overlayBoxesByPage.value.get(pageNumber) || [];
}

function setCanvasRef(pageNumber: number, el: HTMLCanvasElement | null) {
  if (el) {
    canvasRefs.set(pageNumber, el);
  } else {
    canvasRefs.delete(pageNumber);
  }
}

function measureAvailableWidth() {
  const container = scrollContainer.value;
  const candidates = [
    container?.clientWidth || 0,
    container?.getBoundingClientRect().width || 0,
    container?.parentElement?.clientWidth || 0,
    container?.parentElement?.getBoundingClientRect().width || 0,
  ].filter(width => Number.isFinite(width) && width > 0);

  const fallback = Math.min(820, Math.max(360, window.innerWidth - 560));
  const measured = candidates.length ? Math.max(...candidates) : fallback;
  return Math.max(320, Math.floor(measured - 36));
}

async function waitForVisibleLayout() {
  for (let attempt = 0; attempt < 8; attempt += 1) {
    await nextTick();
    await animationFrame();
    if (measureAvailableWidth() > 340) return;
  }
}

function changeZoom(delta: number) {
  const nextZoom = Math.min(2.2, Math.max(0.55, Number((zoom.value + delta).toFixed(2))));
  if (nextZoom === zoom.value) return;
  zoom.value = nextZoom;
  renderPages();
}

function resetZoom() {
  if (zoom.value === 1) return;
  zoom.value = 1;
  renderPages();
}

function selectBox(box: Record<string, unknown>, pageNumber: number) {
  selectedElementId.value = String(box.elementId || '');
  MessagePlugin.info(`${pageNumber}쪽 ${box.label}`);
}

function resolveTargetPages() {
  const explicit = requestedPages.value.filter(page => page <= totalPages.value);
  if (explicit.length) return explicit;
  if (!totalPages.value) return [];
  return Array.from({ length: totalPages.value }, (_, index) => index + 1);
}

async function loadPdf() {
  const token = ++renderToken;
  cleanupPdfDocument();
  canvasRefs.clear();
  renderedPages.value = [];
  totalPages.value = 0;
  error.value = '';

  if (!props.blobUrl) return;

  loading.value = true;
  try {
    const source = props.pdfData
      ? { data: new Uint8Array(props.pdfData.slice(0)) }
      : { url: props.blobUrl };
    const task = pdfjsLib.getDocument(source);
    const loaded = await task.promise;
    if (token !== renderToken) {
      loaded.destroy?.();
      return;
    }
    pdfDocument.value = loaded;
    totalPages.value = loaded.numPages;
    loading.value = false;
    await waitForVisibleLayout();
    await renderPages(token);
    await scrollToPrimaryPage();
  } catch (err: any) {
    console.error('PDF.js preview failed:', err);
    error.value = err?.message || 'PDF 문서를 렌더링하지 못했습니다.';
  } finally {
    if (token === renderToken) loading.value = false;
  }
}

async function renderPages(existingToken?: number) {
  const token = existingToken || ++renderToken;
  const pdf = pdfDocument.value;
  if (!pdf) return;

  const targetPages = resolveTargetPages();
  renderedPages.value = targetPages.map(pageNumber => ({ pageNumber, width: 0, height: 0 }));
  rendering.value = true;
  await nextTick();

  try {
    for (const pageNumber of targetPages) {
      if (token !== renderToken) return;

      const canvas = canvasRefs.get(pageNumber);
      if (!canvas) continue;

      const page = await pdf.getPage(pageNumber);
      if (token !== renderToken) return;

      const baseViewport = page.getViewport({ scale: 1 });
      const availableWidth = measureAvailableWidth();
      const fitScale = availableWidth / baseViewport.width;
      const viewport = page.getViewport({ scale: fitScale * zoom.value });
      const outputScale = window.devicePixelRatio || 1;
      const context = canvas.getContext('2d');
      if (!context) continue;

      renderedPages.value = renderedPages.value.map(pageView => (
        pageView.pageNumber === pageNumber
          ? { pageNumber, width: viewport.width, height: viewport.height }
          : pageView
      ));
      await nextTick();

      canvas.width = Math.floor(viewport.width * outputScale);
      canvas.height = Math.floor(viewport.height * outputScale);
      canvas.style.width = `${viewport.width}px`;
      canvas.style.height = `${viewport.height}px`;
      context.setTransform(1, 0, 0, 1, 0, 0);
      context.clearRect(0, 0, canvas.width, canvas.height);

      const transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : undefined;
      await page.render({ canvasContext: context, viewport, transform }).promise;
      renderedPages.value = renderedPages.value.map(pageView => (
        pageView.pageNumber === pageNumber
          ? { pageNumber, width: viewport.width, height: viewport.height }
          : pageView
      ));
    }
  } finally {
    if (token === renderToken) rendering.value = false;
  }
}

async function scrollToPrimaryPage() {
  await nextTick();
  const page = Number(props.initialPage || requestedPages.value[0] || 0);
  if (!page || !scrollContainer.value) return;
  const target = scrollContainer.value.querySelector(`[data-pdf-page="${page}"]`);
  target?.scrollIntoView({ block: 'start' });
}

function handleResize() {
  window.clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => {
    renderPages();
  }, 180);
}

function cleanupPdfDocument() {
  if (pdfDocument.value) {
    pdfDocument.value.destroy?.();
    pdfDocument.value = null;
  }
}

watch(
  () => [props.blobUrl, props.pdfData, requestedPages.value.join(',')],
  () => loadPdf(),
  { immediate: true }
);

window.addEventListener('resize', handleResize);

onBeforeUnmount(() => {
  renderToken += 1;
  window.removeEventListener('resize', handleResize);
  window.clearTimeout(resizeTimer);
  cleanupPdfDocument();
});
</script>

<template>
  <div class="pdfjs-preview">
    <div class="pdfjs-toolbar">
      <div class="pdfjs-toolbar-left">
        <t-icon name="file" size="15px" />
        <span class="pdfjs-file-name" :title="fileName">{{ fileName }}</span>
      </div>
      <div class="pdfjs-toolbar-right">
        <span v-if="pageSummary" class="pdfjs-page-summary">{{ pageSummary }}</span>
        <t-button theme="default" variant="text" shape="square" size="small" @click="changeZoom(-0.1)">
          <template #icon><t-icon name="remove" /></template>
        </t-button>
        <t-button theme="default" variant="text" size="small" @click="resetZoom">
          {{ Math.round(zoom * 100) }}%
        </t-button>
        <t-button theme="default" variant="text" shape="square" size="small" @click="changeZoom(0.1)">
          <template #icon><t-icon name="add" /></template>
        </t-button>
      </div>
    </div>

    <div ref="scrollContainer" class="pdfjs-scroll">
      <div v-if="loading && !renderedPages.length" class="pdfjs-state">
        <t-loading size="medium" />
        <span>PDF 문서를 렌더링하는 중입니다.</span>
      </div>

      <div v-else-if="error" class="pdfjs-state is-error">
        <t-icon name="error-circle" size="36px" />
        <span>{{ error }}</span>
      </div>

      <div v-else class="pdfjs-pages">
        <div v-if="rendering" class="pdfjs-rendering-badge">
          <t-loading size="small" />
          <span>렌더링 중</span>
        </div>
        <article
          v-for="page in renderedPages"
          :key="page.pageNumber"
          class="pdfjs-page"
          :data-pdf-page="page.pageNumber"
          :style="{ width: page.width ? `${page.width}px` : undefined }"
        >
          <div class="pdfjs-page-label">p.{{ page.pageNumber }}</div>
          <div
            class="pdfjs-page-surface"
            :style="{ width: page.width ? `${page.width}px` : undefined, height: page.height ? `${page.height}px` : undefined }"
          >
            <canvas :ref="el => setCanvasRef(page.pageNumber, el as HTMLCanvasElement | null)" />
            <div v-if="page.width && page.height" class="pdfjs-bbox-layer">
              <button
                v-for="(box, index) in boxesForPage(page.pageNumber)"
                :key="`${box.elementId || index}-${index}`"
                type="button"
                class="pdfjs-bbox"
                :class="{ 'is-selected': selectedElementId && selectedElementId === box.elementId }"
                :style="bboxStyle(box)"
                :title="box.label"
                @click="selectBox(box, page.pageNumber)"
              >
                <span>{{ box.elementId || index + 1 }}</span>
              </button>
            </div>
          </div>
        </article>
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
.pdfjs-preview {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 500px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  background: var(--td-bg-color-page);
}

.pdfjs-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 42px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.pdfjs-toolbar-left,
.pdfjs-toolbar-right {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 8px;
}

.pdfjs-file-name {
  min-width: 0;
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.pdfjs-page-summary {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  white-space: nowrap;
}

.pdfjs-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 16px;
  box-sizing: border-box;
}

.pdfjs-pages {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 18px;
  width: 100%;
  min-width: 0;
}

.pdfjs-rendering-badge {
  position: sticky;
  top: 8px;
  z-index: 4;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  align-self: center;
  padding: 5px 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.92);
  color: var(--td-text-color-secondary);
  font-size: 12px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.pdfjs-page {
  position: relative;
  flex-shrink: 0;
  box-sizing: border-box;
}

.pdfjs-page-label {
  margin-bottom: 6px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.pdfjs-page-surface {
  position: relative;
  flex-shrink: 0;
  min-height: 80px;
  overflow: hidden;
  box-shadow: 0 2px 14px rgba(15, 23, 42, 0.14);
  background: white;

  canvas {
    display: block;
    flex-shrink: 0;
    background: white;
  }
}

.pdfjs-bbox-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.pdfjs-bbox {
  position: absolute;
  pointer-events: auto;
  border: 2px solid rgba(7, 192, 95, 0.92);
  border-radius: 2px;
  background: rgba(7, 192, 95, 0.12);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.9);
  cursor: pointer;
  padding: 0;
  color: var(--td-success-color);
  overflow: hidden;
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover,
  &.is-selected {
    border-color: var(--td-warning-color);
    background: rgba(237, 123, 47, 0.2);
  }

  span {
    position: absolute;
    left: 2px;
    top: 1px;
    max-width: calc(100% - 4px);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: currentColor;
    font-size: 10px;
    line-height: 12px;
    text-shadow: 0 1px 0 white;
  }
}

.pdfjs-state {
  display: flex;
  min-height: 320px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--td-text-color-placeholder);
}

.pdfjs-state.is-error {
  color: var(--td-error-color);
}
</style>
