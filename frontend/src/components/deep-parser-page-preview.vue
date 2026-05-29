// @ts-nocheck
<script setup lang="ts">
// @ts-nocheck
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';

const props = defineProps<{
  fileName: string;
  initialPage?: number | string;
  pages?: number[];
  pageImages?: Array<Record<string, unknown>>;
  bboxes?: Array<Record<string, unknown>>;
  elements?: Array<Record<string, unknown>>;
}>();

const scrollContainer = ref<HTMLElement | null>(null);
const selectedPage = ref(0);
const imageUrls = ref<Record<number, string>>({});
const loadingPages = ref<Record<number, boolean>>({});
const errorPages = ref<Record<number, string>>({});
const ownedObjectUrls = new Set<string>();
const requestCache = new Map<string, string>();

const providerURLPattern = /^(local|minio|cos|tos|s3|oss|ks3|obs):\/\//;

const pageImages = computed(() => {
  const raw = Array.isArray(props.pageImages) ? props.pageImages : [];
  return raw
    .map((image, index) => {
      const page = Number(image?.page || index + 1);
      const source = imageSource(image);
      return {
        ...image,
        page: Number.isFinite(page) && page > 0 ? page : index + 1,
        source,
      };
    })
    .filter(image => image.source)
    .sort((left, right) => left.page - right.page);
});

const availablePages = computed(() => pageImages.value.map(image => image.page));

const pageCards = computed(() => {
  const requested = Array.isArray(props.pages) && props.pages.length
    ? props.pages.map(page => Number(page)).filter(page => Number.isFinite(page) && page > 0)
    : availablePages.value;
  const requestedSet = new Set(requested);
  const filtered = pageImages.value.filter(image => requestedSet.has(image.page));
  return filtered.length ? filtered : pageImages.value;
});

const selectedPageBoxes = computed(() => boxesForPage(selectedPage.value));

function imageSource(image: Record<string, unknown>) {
  return String(
    image?.url
    || image?.storage_url
    || image?.serving_url
    || image?.source_url
    || ''
  ).trim();
}

function getHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};
  try {
    const token = (localStorage.getItem('weknora_token') || '').trim();
    if (token) headers.Authorization = `Bearer ${token}`;
    const tenantId = (localStorage.getItem('weknora_selected_tenant_id') || '').trim();
    if (tenantId) headers['X-Tenant-ID'] = tenantId;
  } catch {
    // localStorage can be unavailable in tests or private contexts.
  }
  return headers;
}

function requestURL(source: string) {
  if (providerURLPattern.test(source)) {
    return `/files?${new URLSearchParams({ file_path: source }).toString()}`;
  }
  return source;
}

async function loadPageImage(page: number, source: string) {
  if (!source || imageUrls.value[page] || loadingPages.value[page]) return;
  const url = requestURL(source);
  if (requestCache.has(url)) {
    imageUrls.value = { ...imageUrls.value, [page]: requestCache.get(url)! };
    return;
  }
  if (!url.startsWith('/files?') && !providerURLPattern.test(source)) {
    imageUrls.value = { ...imageUrls.value, [page]: url };
    return;
  }

  loadingPages.value = { ...loadingPages.value, [page]: true };
  errorPages.value = { ...errorPages.value, [page]: '' };
  try {
    const response = await fetch(url, {
      method: 'GET',
      headers: getHeaders(),
      credentials: 'include',
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const blobUrl = URL.createObjectURL(await response.blob());
    ownedObjectUrls.add(blobUrl);
    requestCache.set(url, blobUrl);
    imageUrls.value = { ...imageUrls.value, [page]: blobUrl };
  } catch (error: any) {
    errorPages.value = { ...errorPages.value, [page]: error?.message || '이미지를 불러오지 못했습니다.' };
  } finally {
    loadingPages.value = { ...loadingPages.value, [page]: false };
  }
}

function ensureImagesLoaded() {
  pageCards.value.forEach(image => {
    loadPageImage(image.page, image.source);
  });
}

function selectPage(page: number) {
  selectedPage.value = page;
  scrollToPage(page);
}

function scrollToPage(page: number) {
  nextTick(() => {
    const root = scrollContainer.value;
    const target = root?.querySelector(`[data-page="${page}"]`);
    if (target instanceof HTMLElement) {
      target.scrollIntoView({ block: 'start', behavior: 'smooth' });
    }
  });
}

function boxesForPage(page: number) {
  if (!page) return [];
  const image = pageCards.value.find(item => item.page === page) || pageImages.value.find(item => item.page === page);
  const sourceBoxes = [
    ...(Array.isArray(props.bboxes) ? props.bboxes : []),
    ...(Array.isArray(props.elements) ? props.elements.filter(element => Array.isArray(element?.bbox)) : []),
  ];
  const seen = new Set<string>();
  return sourceBoxes
    .map((box, index) => normaliseBox(box, image, index))
    .filter(Boolean)
    .filter(box => {
      if (box.page !== page) return false;
      const key = `${box.elementId}:${box.x1}:${box.y1}:${box.x2}:${box.y2}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((left, right) => left.y1 - right.y1 || left.x1 - right.x1);
}

function normaliseBox(rawBox: Record<string, unknown>, image: Record<string, unknown> | undefined, index: number) {
  const bbox = Array.isArray(rawBox?.bbox) ? rawBox.bbox : [];
  if (bbox.length < 4) return null;
  const values = bbox.slice(0, 4).map(value => Number(value));
  if (values.some(value => !Number.isFinite(value))) return null;

  const width = Number(image?.width || 0);
  const height = Number(image?.height || 0);
  let [x1, y1, x2, y2] = values;
  const pixelBox = Math.max(x1, y1, x2, y2) > 1.5;
  if (pixelBox && width > 0 && height > 0) {
    x1 /= width;
    x2 /= width;
    y1 /= height;
    y2 /= height;
  }
  if (x2 < x1) [x1, x2] = [x2, x1];
  if (y2 < y1) [y1, y2] = [y2, y1];

  return {
    key: `${rawBox?.element_id || rawBox?.id || index}-${index}`,
    elementId: String(rawBox?.element_id || rawBox?.id || `bbox-${index + 1}`),
    page: Number(rawBox?.page || image?.page || 0),
    type: String(rawBox?.raw_type || rawBox?.type || rawBox?.element_type || ''),
    x1: clamp01(x1),
    y1: clamp01(y1),
    x2: clamp01(x2),
    y2: clamp01(y2),
  };
}

function clamp01(value: number) {
  return Math.min(1, Math.max(0, value));
}

function boxStyle(box: Record<string, number>) {
  return {
    left: `${box.x1 * 100}%`,
    top: `${box.y1 * 100}%`,
    width: `${Math.max(0.008, box.x2 - box.x1) * 100}%`,
    height: `${Math.max(0.008, box.y2 - box.y1) * 100}%`,
  };
}

function pageLabel(page: number) {
  return `페이지 ${page}`;
}

watch(
  () => [pageCards.value.map(item => `${item.page}:${item.source}`).join('|'), props.initialPage],
  () => {
    const initial = Number(props.initialPage || 0);
    const firstPage = pageCards.value.find(item => item.page === initial)?.page || pageCards.value[0]?.page || 0;
    selectedPage.value = firstPage;
    ensureImagesLoaded();
    if (firstPage) scrollToPage(firstPage);
  },
  { immediate: true }
);

onUnmounted(() => {
  ownedObjectUrls.forEach(url => URL.revokeObjectURL(url));
  ownedObjectUrls.clear();
});
</script>

<template>
  <div class="deep-parser-preview">
    <div class="deep-parser-toolbar">
      <div class="file-summary">
        <t-icon name="image" size="15px" />
        <span :title="fileName">{{ fileName }}</span>
      </div>
      <div class="page-tabs">
        <button
          v-for="image in pageCards"
          :key="image.page"
          type="button"
          :class="{ active: image.page === selectedPage }"
          @click="selectPage(image.page)"
        >
          {{ pageLabel(image.page) }}
        </button>
      </div>
    </div>

    <div ref="scrollContainer" class="page-scroll">
      <div v-for="image in pageCards" :key="image.page" class="page-card" :data-page="image.page">
        <div class="page-card-header">
          <strong>{{ pageLabel(image.page) }}</strong>
          <span>{{ boxesForPage(image.page).length }}개 근거 박스</span>
        </div>
        <div class="page-stage">
          <div v-if="loadingPages[image.page]" class="page-state">페이지 이미지를 불러오는 중입니다.</div>
          <div v-else-if="errorPages[image.page]" class="page-state error">{{ errorPages[image.page] }}</div>
          <div v-else-if="imageUrls[image.page]" class="page-image-wrap">
            <img :src="imageUrls[image.page]" :alt="`${fileName} ${pageLabel(image.page)}`" />
            <div class="bbox-layer" aria-hidden="true">
              <span
                v-for="(box, index) in boxesForPage(image.page)"
                :key="box.key"
                class="bbox"
                :style="boxStyle(box)"
                :title="`${box.elementId} ${box.type}`"
              >
                {{ index + 1 }}
              </span>
            </div>
          </div>
          <div v-else class="page-state">표시할 페이지 이미지가 없습니다.</div>
        </div>
      </div>
    </div>

    <div class="selected-evidence">
      <span>선택 페이지</span>
      <strong>{{ selectedPage ? pageLabel(selectedPage) : '-' }}</strong>
      <span>근거 박스</span>
      <strong>{{ selectedPageBoxes.length }}</strong>
    </div>
  </div>
</template>

<style scoped lang="less">
.deep-parser-preview {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--td-bg-color-page);
}

.deep-parser-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.file-summary,
.page-tabs {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 8px;
}

.file-summary span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.page-tabs {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.page-tabs button {
  height: 26px;
  padding: 0 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  font-size: 12px;
  cursor: pointer;
}

.page-tabs button.active {
  border-color: #2563eb;
  color: #1d4ed8;
  background: #eff6ff;
}

.page-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 18px;
}

.page-card {
  margin: 0 auto 18px;
  max-width: min(100%, 1100px);
}

.page-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.page-card-header strong {
  color: var(--td-text-color-primary);
  font-size: 13px;
}

.page-stage {
  display: flex;
  justify-content: center;
  min-height: 360px;
  padding: 18px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: #e5e7eb;
}

.page-image-wrap {
  position: relative;
  display: inline-block;
  max-width: 100%;
  line-height: 0;
  background: #ffffff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.12);
}

.page-image-wrap img {
  display: block;
  max-width: 100%;
  height: auto;
}

.bbox-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.bbox {
  position: absolute;
  display: flex;
  align-items: flex-start;
  justify-content: flex-start;
  min-width: 16px;
  min-height: 16px;
  padding: 1px 3px;
  border: 2px solid #2563eb;
  border-radius: 3px;
  color: #ffffff;
  background: rgba(37, 99, 235, 0.16);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.85);
  font-size: 10px;
  font-weight: 700;
  line-height: 12px;
}

.page-state {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 320px;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
}

.page-state.error {
  color: var(--td-error-color);
}

.selected-evidence {
  display: grid;
  grid-template-columns: max-content max-content max-content max-content;
  gap: 6px 10px;
  align-items: center;
  padding: 8px 12px;
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.selected-evidence strong {
  color: var(--td-text-color-primary);
}

@media (max-width: 760px) {
  .deep-parser-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .page-tabs {
    justify-content: flex-start;
  }

  .page-scroll {
    padding: 10px;
  }

  .page-stage {
    min-height: 280px;
    padding: 10px;
  }

  .selected-evidence {
    grid-template-columns: max-content 1fr;
  }
}
</style>
