// Every visual constant from docs/VISUAL_SPEC.md. Renderer code imports from
// here and never hard-codes values (AGENTS.md conventions).

export const PALETTE = {
  bg: 0x05050a,
  gridMajor: 0x00f0ff,
  gridMinor: 0x052535,
  hud: 0x00f0ff,
  alertSky: 0x2a0008,
  alert: 0xff2040,
} as const;

export const DEVICE_COLORS: Record<string, number> = {
  workstation: 0x3d7cff,
  server: 0x22e07a,
  iot: 0xff3df2, // magenta, not red: DECISIONS D11
  network: 0xffb020,
  unknown: 0x00f0ff,
};

export const CATEGORY_COLORS: Record<string, number> = {
  https: 0x22e07a,
  web_cleartext: 0xff8c1a,
  dns: 0xffd23f,
  ssh: 0xffffff,
  legacy_cleartext: 0xff2040,
  discovery: 0xa970ff,
  control: 0x7f9cbf,
  other: 0x8a8fa3,
};

export const CAMERA = {
  fov: 60,
  near: 0.5,
  far: 2000,
  position: [170, 140, 170] as const,
  damping: 0.08,
  maxPolarAngleDeg: 85,
  minDistance: 40,
  maxDistance: 700,
} as const;

export const SCENE = {
  fogDensity: 0.0022,
  groundSize: 640,
  gridDivisions: 160,
  gridY: 0.02,
  gridMajorOpacity: 0.35,
  gridMinorOpacity: 0.2,
  maxPixelRatio: 2,
} as const;

export const LIGHTING = {
  hemisphere: { sky: 0x1a2a4a, ground: 0x05050a, intensity: 0.6 },
  moon: { color: 0x9fb8ff, intensity: 1.4, direction: [0.7, 1.0, -0.3] as const, distance: 400 },
  shadow: { mapSize: 2048, extent: 300, near: 10, far: 900, bias: -0.0005, normalBias: 0.5 },
  environment: { sigma: 0.04, intensity: 0.35 },
  toneMappingExposure: 1.0,
} as const;

export const MATERIALS = {
  ground: { color: 0x080a12, roughness: 0.55, metalness: 0.3 },
  plot: { color: 0x0b0e1a, roughness: 0.6, metalness: 0.2, height: 0.2, borderWidth: 0.3, borderIntensity: 0.4 },
  building: {
    baseColor: 0x0c1020,
    tint: 0.25,
    roughness: 0.35,
    metalness: 0.6,
    windowIntensity: 1.6,
    edgeWidth: 0.06,
    edgeIntensity: 1.2,
    rimIntensity: 0.35,
    rimPower: 3,
    roofIntensity: 0.25,
  },
} as const;

export const BLOOM = { strength: 0.9, radius: 0.4, threshold: 0.85 } as const;

export const LAYOUT = {
  blockSize: 64,
  avenue: 16,
  gridBlocks: 3, // 3 × 3 local districts
  slotsPerSide: 16,
  slotPitch: 4,
  footprint: 2,
  serverBlockOrder: [4, 1, 3, 5, 7, 0, 2, 6, 8] as const,
  outlands: { innerRadius: 180, ringStep: 8, rings: 11, arcSlot: 4 },
  sectors: ["N", "NE", "E", "SE", "S", "SW", "W", "NW"] as const,
} as const;

export const RATE = {
  halfLifeS: 2.0,
  heightScale: 6,
  heightDivisor: 1000,
  maxHeight: 40,
  displayTauS: 0.25,
} as const;

export const STATUS = {
  degradedRttMs: 150,
  degradedRttSpanMs: 350,
  degradedRetrans: 0.02,
  degradedRetransSpan: 0.08,
  spikeFactor: 4,
  spikeMinBps: 1_000_000,
  flashS: 0.6,
  lightningFactor: 10,
  lightningMinBps: 10_000_000,
  lightningS: 0.4,
  lightningColor: 0xe0f7ff,
  fadeInS: 0.8,
  fadeOutS: 1.0,
} as const;

export const HIGHWAYS = {
  activeS: 3,
  max: 500,
  y: 0.1,
  minWidth: 0.2,
  widthScale: 0.6,
  maxWidth: 3,
  opacity: 0.35,
  fadeS: 3,
} as const;

export const PARTICLES = {
  pool: 5000,
  poolLow: 2500,
  size: [0.3, 0.3, 0.6] as const,
  maxRate: 20,
  travelBaseS: 0.4,
  travelPerRttMs: 0.02,
  travelMaxS: 4.0,
  travelUnknownS: 1.5,
} as const;

export const EFFECTS = {
  searchlight: { durationS: 6, radius: 12, height: 60, opacity: 0.22, perTargetS: 0.3 },
  meteor: { durationS: 8, rampInS: 1, rampOutS: 2, particles: 400, particlesLow: 200, startY: 200, radius: 120, speedMin: 60, speedMax: 120, flickerHz: 12 },
  outage: { flickerS: 2, dissolveS: 3, flickerHz: 10 },
  maxSearchlights: 3,
  maxMeteors: 2,
} as const;

export const NET = {
  backoffBaseS: 0.5,
  backoffMaxS: 10,
  backoffJitter: 0.2,
} as const;

export const HUD = {
  eventLogSize: 50,
  softwareRendererPattern: /llvmpipe|swiftshader|software/i,
} as const;

export const MAX_BUILDINGS = 2000;
