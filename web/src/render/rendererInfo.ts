import { HUD } from "../config";

export interface RendererInfo {
  name: string;
  software: boolean;
}

/** The parts of a WebGL context used here (structural, so tests can fake it). */
export interface GL {
  readonly RENDERER: number;
  getExtension(name: string): unknown;
  getParameter(pname: number): unknown;
}

/**
 * Identifies the GPU renderer and whether the browser fell back to software
 * rendering (docs/VISUAL_SPEC.md "Software rendering warning").
 */
export function rendererInfo(gl: GL): RendererInfo {
  let name = "unknown";
  const ext = gl.getExtension("WEBGL_debug_renderer_info") as { UNMASKED_RENDERER_WEBGL: number } | null;
  const raw: unknown = ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : gl.getParameter(gl.RENDERER);
  if (typeof raw === "string" && raw.trim() !== "") name = raw;
  return { name, software: HUD.softwareRendererPattern.test(name) };
}
