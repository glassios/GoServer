import * as THREE from 'three';
import { assetUrl } from '@/src/shared/assetUrl';

export const EXPLOSION_SHEET_PATH = assetUrl('/explosive_sheet.png');
export const EXPLOSION_META_PATH = assetUrl('/explosive_sheet.meta.json');

/** Fallback when meta + auto-detect are unavailable. */
export const EXPLOSION_SHEET_DEFAULTS = {
  displaySize: 2.8 / 2.5,
  durationMs: 600,
  cols: 5,
  rows: 3,
} as const;

export interface ExplosionSheetLayout {
  cols: number;
  rows: number;
  frameCount: number;
  cellW: number;
  cellH: number;
  durationMs: number;
  displaySize: number;
}

interface ExplosionSheetMeta {
  cols?: number;
  rows?: number;
  frameCount?: number;
  durationMs?: number;
  /** Bust browser cache when the PNG is replaced (any new value). */
  version?: string | number;
}

let sheetTexture: THREE.Texture | null = null;
let sheetLayout: ExplosionSheetLayout | null = null;
let loadPromise: Promise<ExplosionSheetLayout> | null = null;

async function fetchExplosionMeta(): Promise<ExplosionSheetMeta | null> {
  try {
    const res = await fetch(EXPLOSION_META_PATH, { cache: 'no-store' });
    if (!res.ok) return null;
    return (await res.json()) as ExplosionSheetMeta;
  } catch {
    return null;
  }
}

/** Pick grid with square-ish cells and the most frames (typical sprite sheet). */
export function detectGridFromImageSize(
  width: number,
  height: number
): Pick<ExplosionSheetLayout, 'cols' | 'rows' | 'frameCount' | 'cellW' | 'cellH'> {
  if (width < 1 || height < 1) {
    return {
      cols: 1,
      rows: 1,
      frameCount: 1,
      cellW: width,
      cellH: height,
    };
  }

  let bestScore = -1;
  let best: Pick<ExplosionSheetLayout, 'cols' | 'rows' | 'frameCount' | 'cellW' | 'cellH'> | null =
    null;

  for (let cols = 1; cols <= width; cols++) {
    if (width % cols !== 0) continue;
    const cellW = width / cols;

    for (let rows = 1; rows <= height; rows++) {
      if (height % rows !== 0) continue;
      const cellH = height / rows;
      if (cellW !== cellH) continue;

      const frameCount = cols * rows;
      const score = frameCount * 1000 + 100;

      if (score > bestScore) {
        bestScore = score;
        best = { cols, rows, frameCount, cellW, cellH };
      }
    }
  }

  if (best) {
    return best;
  }

  // Non-square cells: any grid that tiles the image exactly
  let fallbackCols = 1;
  let fallbackRows = 1;
  let bestFrames = 1;

  for (let cols = 1; cols <= Math.min(width, 64); cols++) {
    if (width % cols !== 0) continue;
    for (let rows = 1; rows <= Math.min(height, 64); rows++) {
      if (height % rows !== 0) continue;
      const frames = cols * rows;
      if (frames > bestFrames) {
        bestFrames = frames;
        fallbackCols = cols;
        fallbackRows = rows;
      }
    }
  }

  return {
    cols: fallbackCols,
    rows: fallbackRows,
    frameCount: bestFrames,
    cellW: width / fallbackCols,
    cellH: height / fallbackRows,
  };
}

function resolveLayout(
  imageW: number,
  imageH: number,
  meta: ExplosionSheetMeta | null
): ExplosionSheetLayout {
  const detected = detectGridFromImageSize(imageW, imageH);

  let cols = detected.cols;
  let rows = detected.rows;

  if (meta?.cols && meta.cols > 0) cols = Math.floor(meta.cols);
  if (meta?.rows && meta.rows > 0) rows = Math.floor(meta.rows);

  const gridFrames = cols * rows;
  let frameCount = meta?.frameCount && meta.frameCount > 0 ? Math.floor(meta.frameCount) : gridFrames;
  frameCount = Math.min(Math.max(1, frameCount), gridFrames);

  const cellW = imageW / cols;
  const cellH = imageH / rows;
  const durationMs =
    meta?.durationMs && meta.durationMs > 0 ? meta.durationMs : EXPLOSION_SHEET_DEFAULTS.durationMs;

  return {
    cols,
    rows,
    frameCount,
    cellW,
    cellH,
    durationMs,
    displaySize: EXPLOSION_SHEET_DEFAULTS.displaySize,
  };
}

function getImageSize(tex: THREE.Texture): { w: number; h: number } {
  const img = tex.image as { width?: number; height?: number } | undefined;
  const w = img?.width ?? 1;
  const h = img?.height ?? 1;
  return { w, h };
}

/** Clears cache so the next load re-reads PNG + meta (after you replace files). */
export function invalidateExplosionSheetCache(): void {
  sheetTexture?.dispose();
  sheetTexture = null;
  sheetLayout = null;
  loadPromise = null;
}

export function getExplosionSheetLayout(): ExplosionSheetLayout {
  return (
    sheetLayout ?? {
      ...EXPLOSION_SHEET_DEFAULTS,
      frameCount: EXPLOSION_SHEET_DEFAULTS.cols * EXPLOSION_SHEET_DEFAULTS.rows,
      cellW: 1,
      cellH: 1,
    }
  );
}

/** World quad size [width, height] from cell aspect ratio. */
export function getExplosionDisplaySize(layout = getExplosionSheetLayout()): [number, number] {
  const base = layout.displaySize;
  const aspect = layout.cellW / Math.max(layout.cellH, 1);
  if (aspect >= 1) return [base * aspect, base];
  return [base, base / aspect];
}

export function loadExplosionSheetTexture(): Promise<ExplosionSheetLayout> {
  if (sheetLayout && sheetTexture) return Promise.resolve(sheetLayout);
  if (!loadPromise) {
    loadPromise = (async () => {
      const meta = await fetchExplosionMeta();
      const cacheKey =
        meta?.version !== undefined
          ? String(meta.version)
          : `auto-${Date.now()}`;
      const url = `${EXPLOSION_SHEET_PATH}?v=${encodeURIComponent(cacheKey)}`;

      const tex = await new Promise<THREE.Texture>((resolve, reject) => {
        new THREE.TextureLoader().load(
          url,
          resolve,
          undefined,
          reject
        );
      });

      tex.colorSpace = THREE.SRGBColorSpace;
      tex.magFilter = THREE.LinearFilter;
      tex.minFilter = THREE.LinearFilter;
      tex.wrapS = THREE.ClampToEdgeWrapping;
      tex.wrapT = THREE.ClampToEdgeWrapping;

      const { w, h } = getImageSize(tex);
      const layout = resolveLayout(w, h, meta);

      sheetTexture = tex;
      sheetLayout = layout;
      return layout;
    })();
  }
  return loadPromise;
}

export function getExplosionSheetTexture(): THREE.Texture | null {
  return sheetTexture;
}

/** UV window for frame index (0 = top-left, left-to-right, top-to-bottom). */
export function applyExplosionSheetFrame(texture: THREE.Texture, frameIndex: number): void {
  const { cols, rows, frameCount } = getExplosionSheetLayout();
  const idx = Math.max(0, Math.min(frameIndex, frameCount - 1));
  const col = idx % cols;
  const rowTop = Math.floor(idx / cols);
  const tileW = 1 / cols;
  const tileH = 1 / rows;

  texture.repeat.set(tileW, tileH);
  texture.offset.set(col * tileW, 1 - (rowTop + 1) * tileH);
}

export function createExplosionFrameTexture(source: THREE.Texture, frameIndex: number): THREE.Texture {
  const tex = source.clone();
  applyExplosionSheetFrame(tex, frameIndex);
  return tex;
}
