// Building materials (docs/VISUAL_SPEC.md "Lighting, materials and shading").
// One lit MeshStandardMaterial extended through onBeforeCompile: never a
// ShaderMaterial, which would lose three.js lighting and shadows (D17).
import * as THREE from "three";
import { CATEGORY_COLORS, MATERIALS } from "../config";
import { CATEGORIES } from "../protocol";

const B = MATERIALS.building;

/** GLSL helpers shared by the colour and depth passes. */
const NOISE = /* glsl */ `
float ccHash(vec3 p) {
  p = fract(p * 0.3183099 + 0.1);
  p *= 17.0;
  return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}`;

const VERTEX_DECL = /* glsl */ `
attribute vec3 aColor;
attribute float aServices;
attribute vec4 aState; // x degraded, y flash, z dissolve, w fade-in
varying vec3 vCCColor;
varying float vCCServices;
varying vec4 vCCState;
varying vec3 vCCLocal;
varying vec3 vCCObjNormal;
varying float vCCHeight;
`;

const VERTEX_BODY = /* glsl */ `
vCCColor = aColor;
vCCServices = aServices;
vCCState = aState;
vCCLocal = position;
vCCObjNormal = normal;
#ifdef USE_INSTANCING
vCCHeight = length(instanceMatrix[1].xyz);
#else
vCCHeight = 1.0;
#endif
`;

/** Category colours in facade-window bit order, as a GLSL array (linear). */
function categoryColorsGLSL(): string {
  const c = new THREE.Color();
  const rows = CATEGORIES.map((k) => {
    c.setHex(CATEGORY_COLORS[k] ?? 0xffffff);
    return `vec3(${c.r.toFixed(4)}, ${c.g.toFixed(4)}, ${c.b.toFixed(4)})`;
  });
  return `const vec3 CC_CATEGORY[8] = vec3[8](${rows.join(", ")});`;
}

/**
 * The building material: dark glass tinted by device class with emissive edge
 * accents, a fresnel rim and a roof cap. Windows (T3.5) and status effects
 * (T3.6) build on the same attributes.
 */
export function buildingMaterial(): THREE.MeshStandardMaterial {
  const mat = new THREE.MeshStandardMaterial({ roughness: B.roughness, metalness: B.metalness, side: THREE.FrontSide });
  const base = new THREE.Color(B.baseColor);
  mat.onBeforeCompile = (shader) => {
    shader.uniforms.uCCBase = { value: base };
    shader.uniforms.uCCTime = { value: 0 };
    mat.userData.shader = shader;
    shader.vertexShader = shader.vertexShader
      .replace("#include <common>", `#include <common>\n${VERTEX_DECL}`)
      .replace("#include <begin_vertex>", `#include <begin_vertex>\n${VERTEX_BODY}`);
    shader.fragmentShader = shader.fragmentShader
      .replace(
        "#include <common>",
        `#include <common>
        uniform vec3 uCCBase;
        uniform float uCCTime;
        varying vec3 vCCColor;
        varying float vCCServices;
        varying vec4 vCCState;
        varying vec3 vCCLocal;
        varying vec3 vCCObjNormal;
        varying float vCCHeight;
        ${categoryColorsGLSL()}
        ${NOISE}`,
      )
      .replace(
        "#include <clipping_planes_fragment>",
        `#include <clipping_planes_fragment>
        if (vCCState.z > 0.0 && ccHash(floor(vCCLocal * vec3(4.0, 4.0 * vCCHeight, 4.0))) < vCCState.z) discard;`,
      )
      .replace(
        "vec4 diffuseColor = vec4( diffuse, opacity );",
        `vec3 ccClass = mix(vCCColor, vec3(1.0, 0.0, 1.0), vCCState.x * 0.5); // degraded → magenta
        vec4 diffuseColor = vec4(mix(uCCBase, ccClass, ${B.tint.toFixed(3)}), opacity);`,
      )
      .replace(
        "#include <emissivemap_fragment>",
        `#include <emissivemap_fragment>
        {
          bool roof = vCCObjNormal.y > 0.5;
          float ew = ${B.edgeWidth.toFixed(3)};
          float ax = abs(vCCLocal.x), az = abs(vCCLocal.z);
          // Edge strips: the four vertical edges and the roof perimeter.
          float edge = (!roof && ax > 1.0 - ew && az > 1.0 - ew) || (roof && max(ax, az) > 1.0 - ew) ? 1.0 : 0.0;
          totalEmissiveRadiance += ccClass * ${B.edgeIntensity.toFixed(3)} * edge;
          if (roof) totalEmissiveRadiance += ccClass * ${B.roofIntensity.toFixed(3)};
          // Fresnel rim keeps the holographic feel on a solid body.
          float facing = max(dot(normal, normalize(vViewPosition)), 0.0);
          totalEmissiveRadiance += ccClass * ${B.rimIntensity.toFixed(3)} * pow(1.0 - facing, ${B.rimPower.toFixed(1)});
          // Facade windows (VISUAL_SPEC "Buildings"): 6 columns per side face,
          // assigned to the set service-category bits in order, repeating.
          if (!roof && abs(vCCObjNormal.y) < 0.5) {
            float u = abs(vCCObjNormal.x) > 0.5 ? (vCCLocal.z + 1.0) * 0.5 : (vCCLocal.x + 1.0) * 0.5;
            float colF = u * 6.0;
            int col = int(min(floor(colF), 5.0));
            // Degraded hosts: UV jitter of up to 0.15 units plus scanline noise.
            float jitter = vCCState.x * 0.15 * sin(uCCTime * 23.0 + vCCLocal.y * 40.0);
            float yM = vCCLocal.y * vCCHeight + jitter; // metres up the facade
            totalEmissiveRadiance += ccClass * vCCState.x * 0.3 * step(0.5, fract(yM * 8.0 + uCCTime * 3.0));
            float inWindow = step(abs(fract(colF) - 0.5), 0.375) * step(0.25, fract(yM)) * step(fract(yM), 0.75);
            int services = int(vCCServices + 0.5);
            int count = 0;
            for (int i = 0; i < 8; i++) count += (services >> i) & 1;
            if (count == 0) {
              // No services: dim cyan on alternate floors.
              float alt = 1.0 - mod(floor(yM), 2.0);
              totalEmissiveRadiance += vec3(0.0, 0.94, 1.0) * 0.15 * inWindow * alt;
            } else {
              int want = col - (col / count) * count; // col mod count
              int bit = 0;
              int seen = 0;
              for (int i = 0; i < 8; i++) {
                if (((services >> i) & 1) == 1) {
                  if (seen == want) bit = i;
                  seen++;
                }
              }
              float on = bit == 3 ? step(0.5, fract(uCCTime * 2.0)) : 1.0; // ssh blinks at 2 Hz
              totalEmissiveRadiance += CC_CATEGORY[bit] * ${B.windowIntensity.toFixed(3)} * inWindow * on;
            }
          }
          // Spike flash: a white band rising from the ground (aState.y 1 → 0).
          if (vCCState.y > 0.0) {
            float band = 1.0 - vCCState.y;
            totalEmissiveRadiance += vec3(1.0) * 1.5 * (1.0 - smoothstep(0.0, 0.08, abs(vCCLocal.y - band)));
          }
        }`,
      );
  };
  mat.customProgramCacheKey = () => "cybercity-building-v2";
  return mat;
}

/** Shadow-pass material applying the same dissolve discard (VISUAL_SPEC). */
export function buildingDepthMaterial(): THREE.MeshDepthMaterial {
  const mat = new THREE.MeshDepthMaterial({ depthPacking: THREE.RGBADepthPacking });
  mat.onBeforeCompile = (shader) => {
    shader.vertexShader = shader.vertexShader
      .replace("#include <common>", `#include <common>\n${VERTEX_DECL}`)
      .replace("#include <begin_vertex>", `#include <begin_vertex>\n${VERTEX_BODY}`);
    shader.fragmentShader = shader.fragmentShader
      .replace(
        "#include <common>",
        `#include <common>
        varying vec4 vCCState;
        varying vec3 vCCLocal;
        varying float vCCHeight;
        ${NOISE}`,
      )
      .replace(
        "#include <clipping_planes_fragment>",
        `#include <clipping_planes_fragment>
        if (vCCState.z > 0.0 && ccHash(floor(vCCLocal * vec3(4.0, 4.0 * vCCHeight, 4.0))) < vCCState.z) discard;`,
      );
  };
  mat.customProgramCacheKey = () => "cybercity-building-depth-v1";
  return mat;
}

