// SPDX-License-Identifier: MIT
// Additive glow material for highways and particles: per-instance colour and
// alpha, no lighting (these are light trails, not city geometry).
import * as THREE from "three";

const VERTEX = /* glsl */ `
attribute vec3 iColor;
attribute float iAlpha;
varying vec3 vColor;
varying float vAlpha;
void main() {
  vColor = iColor;
  vAlpha = iAlpha;
  gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0);
}`;

const FRAGMENT = /* glsl */ `
varying vec3 vColor;
varying float vAlpha;
void main() {
  gl_FragColor = vec4(vColor * vAlpha, vAlpha);
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
}`;

export function glowMaterial(): THREE.ShaderMaterial {
  return new THREE.ShaderMaterial({
    vertexShader: VERTEX,
    fragmentShader: FRAGMENT,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    toneMapped: true,
  });
}

/** Adds the iColor/iAlpha instance attributes to geo and returns them. */
export function glowAttributes(geo: THREE.BufferGeometry, capacity: number) {
  const color = new THREE.InstancedBufferAttribute(new Float32Array(capacity * 3), 3);
  const alpha = new THREE.InstancedBufferAttribute(new Float32Array(capacity), 1);
  color.setUsage(THREE.DynamicDrawUsage);
  alpha.setUsage(THREE.DynamicDrawUsage);
  geo.setAttribute("iColor", color);
  geo.setAttribute("iAlpha", alpha);
  return { color, alpha };
}
