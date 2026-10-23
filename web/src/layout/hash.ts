// SPDX-License-Identifier: MIT
/** 32-bit FNV-1a over the UTF-8 bytes of s (docs/VISUAL_SPEC.md "Layout"). */
export function fnv1a32(s: string): number {
  let h = 0x811c9dc5;
  for (const b of new TextEncoder().encode(s)) {
    h ^= b;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}
