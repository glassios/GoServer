/* WebGL battle renderer for OLDSECTOR tactical combat.
   Exposes window.BattleGL.create(canvas) -> renderer.
   Renderer reads the simulation object B each frame (ships/projs/parts/asteroids
   + camera) and draws everything with custom shaders:
     - normal-mapped sprite lighting (ships + asteroids)
     - additive engine flames / sparks / fireballs (points)
     - additive tracers + sustained laser beams (rotated quads)
     - amber energy shield with REAL refraction of the scene FBO + impact ripples
   Falls back gracefully: create() returns null if WebGL is unavailable. */
(function () {
  'use strict';

  function compile(gl, type, src) {
    const sh = gl.createShader(type);
    gl.shaderSource(sh, src);
    gl.compileShader(sh);
    if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
      console.error('shader error:', gl.getShaderInfoLog(sh), src);
      return null;
    }
    return sh;
  }
  function program(gl, vs, fs) {
    const p = gl.createProgram();
    gl.attachShader(p, compile(gl, gl.VERTEX_SHADER, vs));
    gl.attachShader(p, compile(gl, gl.FRAGMENT_SHADER, fs));
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) { console.error('link error:', gl.getProgramInfoLog(p)); return null; }
    // cache uniforms + attribs
    p._u = {}; p._a = {};
    const nu = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS);
    for (let i = 0; i < nu; i++) { const info = gl.getActiveUniform(p, i); p._u[info.name.replace(/\[0\]$/, '')] = gl.getUniformLocation(p, info.name); }
    const na = gl.getProgramParameter(p, gl.ACTIVE_ATTRIBUTES);
    for (let i = 0; i < na; i++) { const info = gl.getActiveAttrib(p, i); p._a[info.name] = gl.getAttribLocation(p, info.name); }
    return p;
  }

  // ---- shared vertex shaders ----
  const VS_ROT = `
    precision highp float;
    attribute vec2 aPos;        // unit quad 0..1
    uniform vec2 uCenter;       // screen px (y-down)
    uniform vec2 uSize;         // css px
    uniform float uRot;         // radians (screen y-down)
    uniform vec2 uView;         // viewport css px
    varying vec2 vUv;
    varying vec2 vScr;          // fragment screen px (y-down) for point-light distance
    void main(){
      vUv = aPos;
      vec2 c = (aPos - 0.5) * uSize;
      float s = sin(uRot), co = cos(uRot);
      vec2 r = vec2(c.x*co - c.y*s, c.x*s + c.y*co);
      vec2 scr = uCenter + r;
      vScr = scr;
      gl_Position = vec4(scr.x/uView.x*2.0-1.0, 1.0-scr.y/uView.y*2.0, 0.0, 1.0);
    }`;
  const VS_QUAD = `
    precision highp float;
    attribute vec2 aPos;
    uniform vec2 uCenter; uniform vec2 uSize; uniform vec2 uView;
    varying vec2 vUv; varying vec2 vScreen;
    void main(){
      vUv = aPos;
      vec2 scr = uCenter + (aPos-0.5)*uSize;
      vec2 ndc = vec2(scr.x/uView.x*2.0-1.0, 1.0-scr.y/uView.y*2.0);
      vScreen = ndc*0.5+0.5;
      gl_Position = vec4(ndc,0.0,1.0);
    }`;

  // ---- fragment shaders ----
  const FS_SPRITE = `
    precision highp float;
    varying vec2 vUv;
    varying vec2 vScr;     // fragment screen px (y-down)
    uniform sampler2D uDiff, uNorm;
    uniform vec3 uLight;   // dir (screen space, z towards viewer)
    uniform float uRot;    // sprite rotation (to rotate normals)
    uniform vec3 uTint;
    uniform float uHit;    // white hit flash 0..1
    uniform float uGlow;        // death heat glow 0..N (tinted by uGlowCol)
    uniform vec3  uGlowCol;     // heat glow color (dark red on death)
    uniform float uAlpha;       // global alpha multiplier (hull fade-out during warp)
    uniform float uAmb;
    uniform int  uPLN;     // active impact point-lights
    uniform vec4 uPL[8];   // xy = screen px center, z = radius px, w = intensity
    uniform vec3 uPLC[8];  // point-light color
    // PBR — Three.js meshStandardMaterial; per-draw material (ships: metal 0.6 rough 0.4,
    // asteroids: metal 0.0 rough ~0.9 -> non-metallic stone)
    uniform float uMetal;
    uniform float uRough;
    const float PI = 3.14159265;
    float distGGX(float NoH, float a){
      float a2 = a*a; float d = NoH*NoH*(a2-1.0)+1.0;
      return a2 / (PI*d*d);
    }
    float geomSmith(float NoV, float NoL, float a){
      float k = (a*a)/2.0;
      float gv = NoV/(NoV*(1.0-k)+k);
      float gl = NoL/(NoL*(1.0-k)+k);
      return gv*gl;
    }
    vec3 fresnel(float c, vec3 F0){ return F0 + (1.0-F0)*pow(1.0-c,5.0); }
    void main(){
      vec4 d = texture2D(uDiff, vUv);
      if (d.a < 0.1) discard;                       // alphaTest = 0.1
      vec3 albedo = d.rgb * uTint;
      // normal map -> rotate into sprite orientation
      vec3 n = texture2D(uNorm, vUv).rgb * 2.0 - 1.0;
      n.y = -n.y;
      float s = sin(uRot), c = cos(uRot);
      vec2 nr = vec2(n.x*c - n.y*s, n.x*s + n.y*c);
      vec3 N = normalize(vec3(nr, max(n.z, 0.15)));
      vec3 V = vec3(0.0, 0.0, 1.0);                 // top-down view
      vec3 L = normalize(uLight);
      vec3 H = normalize(L + V);
      float NoL = max(dot(N, L), 0.0);
      float NoV = max(dot(N, V), 1e-3);
      float NoH = max(dot(N, H), 0.0);
      float VoH = max(dot(V, H), 0.0);
      // Cook-Torrance specular
      vec3 F0 = mix(vec3(0.04), albedo, uMetal);
      float a = uRough*uRough;
      float D = distGGX(NoH, a);
      float G = geomSmith(NoV, NoL, a);
      vec3  F = fresnel(VoH, F0);
      vec3 spec = (D * G) * F / max(4.0*NoV*NoL, 1e-3);
      vec3 kd = (vec3(1.0) - F) * (1.0 - uMetal);
      vec3 diffuse = kd * albedo / PI;
      vec3 lightCol = vec3(3.0);                     // directional light intensity
      vec3 Lo = (diffuse + spec) * lightCol * NoL;
      // ambient (no env map): irradiance + metallic fresnel rim for sheen
      float fres = pow(1.0 - NoV, 5.0);
      vec3 ambSpec = F0 * (0.3 + fres * (1.0 - uRough));
      vec3 ambient = albedo * uAmb * (1.0 - uMetal) + ambSpec * uAmb * 1.4;
      vec3 col = (Lo + ambient) * 0.7;               // overall exposure (-30%)
      // dynamic impact point-lights — illuminate the hull via the normal map (diffuse + spec)
      for (int i = 0; i < 8; i++) {
        if (i >= uPLN) break;
        vec2 dxy = uPL[i].xy - vScr;                 // toward light, screen px (y-down, matches N)
        float rad = max(uPL[i].z, 1.0);
        float att = clamp(1.0 - length(dxy) / rad, 0.0, 1.0);
        att = att * att;
        vec3 Lp = normalize(vec3(dxy / rad, 0.6));    // light sits a touch above the plane
        float NoLp = max(dot(N, Lp), 0.0);
        vec3 Hp = normalize(Lp + V);
        float specP = pow(max(dot(N, Hp), 0.0), 28.0) * (1.0 - uRough);
        col += uPLC[i] * (albedo * NoLp + specP) * att * uPL[i].w;
      }
      col += uHit * (albedo + 0.4);                  // white hit flash
      col += uGlow * (albedo + 0.4) * uGlowCol;      // death heat glow (dark red)
      gl_FragColor = vec4(col, d.a * uAlpha);
    }`;
  const FS_BEAM = `
    precision highp float;
    varying vec2 vUv;
    uniform vec3 uCore, uGlow;
    uniform float uIntensity, uFadeX;
    void main(){
      float distY = abs(vUv.y - 0.5) * 2.0;
      float aY = 1.0 - pow(distY, 2.0);
      float aX = mix(1.0, 1.0 - pow(vUv.x, 3.0), uFadeX);
      float a = aX * aY * uIntensity;
      vec3 col = mix(uCore, uGlow, clamp(distY * 1.5, 0.0, 1.0));
      gl_FragColor = vec4(col * 1.6, a);
    }`;
  const FS_BLIT = `
    precision highp float;
    varying vec2 vUv; varying vec2 vScreen;
    uniform sampler2D uTex;
    uniform float uAspect;
    uniform float uRipZoom;
    uniform int uRipCount;
    uniform vec4 uRip[10];   // cx, cy (uv), leadR (height-fraction), amp
    void main(){
      vec2 uv = vUv;
      float crest = 0.0;
      float z = max(uRipZoom, 0.0001);
      for (int i = 0; i < 10; i++) {
        if (i >= uRipCount) break;
        vec2 c = uRip[i].xy;
        float leadR = uRip[i].z;
        float amp = uRip[i].w;
        vec2 d = (vUv - c) * vec2(uAspect, 1.0);
        float dist = length(d);
        float fr = dist - leadR;                       // signed distance to the expanding front
        // thin water-like wave packet riding the front; all spatial terms scale with zoom so
        // the effect stays locked to the hull (no parallax / slide when the camera pans/zooms)
        float env = exp(-fr * fr * 3200.0 / (z * z));
        float wave = sin(fr * 110.0 / z) * env;
        float w = wave * amp * 0.0052 * z;             // radial UV displacement (space refraction)
        vec2 dir = dist > 1e-4 ? d / dist : vec2(0.0);
        uv += vec2(dir.x / uAspect, dir.y) * w;
        crest += env * amp;
      }
      vec3 col = texture2D(uTex, uv).rgb;
      col += vec3(0.45, 0.6, 1.0) * crest * 0.07;      // faint caustic glint on the wavefront
      gl_FragColor = vec4(col, 1.0);
    }`;
  const FS_BG = `
    precision highp float;
    varying vec2 vUv; varying vec2 vScreen;
    uniform vec2 uCam; uniform float uZoom; uniform float uAspect;
    float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1,311.7)))*43758.5453); }
    vec2 hash2(vec2 p){ return fract(sin(vec2(dot(p,vec2(127.1,311.7)),dot(p,vec2(269.5,183.3))))*43758.5453); }
    // one scattered star layer
    float starLayer(vec2 uv, float density, float thresh){
      vec2 g = uv*density; vec2 cell = floor(g); vec2 f = fract(g);
      float h = hash(cell);
      if (h < thresh) return 0.0;
      // keep star center >=0.18 from the cell edge so its 0.14 glow never crosses
      // the boundary into a neighbour cell (which would hard-clip it)
      vec2 pos = 0.18 + 0.64 * hash2(cell+1.7);
      float d = length(f - pos);
      float br = (h - thresh)/(1.0 - thresh);          // 0..1 brightness
      float a = smoothstep(0.14, 0.0, d);              // soft radial alpha
      return a * a * (0.35 + br*0.65);
    }
    void main(){
      // base screen->world (before parallax). Each layer is sampled at base + uCam*p, where a
      // smaller parallax factor p = more distant = drifts slower as the camera pans (depth).
      vec2 base = (vUv - 0.5)/uZoom;
      vec3 col = vec3(0.012,0.018,0.030);
      // very faint sector grid (mid parallax)
      vec2 guv = base + uCam*0.5; guv.x *= uAspect;
      vec2 gf = abs(fract(guv/0.5) - 0.5);
      float gl = min(gf.x, gf.y);
      col += smoothstep(0.5, 0.47, gl) * vec3(0.018,0.026,0.034);
      // 4 parallax star layers, far -> near (distant = denser/smaller/slower)
      float s = 0.0;
      vec2 l1 = base + uCam*0.10; l1.x *= uAspect; s += starLayer(l1 + 1.7,  62.0, 0.905) * 0.55; // farthest
      vec2 l2 = base + uCam*0.26; l2.x *= uAspect; s += starLayer(l2 + 7.3,  40.0, 0.885) * 0.80;
      vec2 l3 = base + uCam*0.46; l3.x *= uAspect; s += starLayer(l3 + 3.1,  24.0, 0.855) * 1.05;
      vec2 l4 = base + uCam*0.72; l4.x *= uAspect; s += starLayer(l4 + 12.9, 13.0, 0.820) * 1.45; // nearest
      // ~12% amber stars, rest cool white (matches main starfield)
      vec2 tuv = base + uCam*0.46; tuv.x *= uAspect;
      float tone = hash(floor(tuv*20.0) + 5.0);
      vec3 starCol = tone > 0.88 ? vec3(1.0,0.69,0.38) : vec3(0.80,0.87,1.0);
      col += starCol * s;
      gl_FragColor = vec4(col, 1.0);
    }`;
  const FS_SHIELD = `
    precision highp float;
    varying vec2 vUv; varying vec2 vScreen;
    uniform float uPower, uTime, uShipRot, uPulseFreq, uBandWidth;
    uniform vec3 uColor1, uColor2;
    uniform vec4 uHits[8];   // x,y (local -0.5..0.5), age 0..1, scale
    uniform int uHitCount;
    void main(){
      vec2 pos = vUv - 0.5;            // -0.5..0.5
      float dist = length(pos);
      float radius = 0.31;
      float dr = abs(dist - radius);
      float bw = max(0.2, uBandWidth);
      float gw = 0.118 * bw, cw = 0.0104 * bw;
      float glow = dr < gw ? pow(1.0 - dr/gw, 2.0) * 0.5 : 0.0;
      float core = dr < cw ? pow(1.0 - dr/cw, 2.0) : 0.0;
      float inten = glow + core;
      // arc limited to facing direction
      float ang = atan(pos.y, pos.x) - uShipRot;
      ang = atan(sin(ang), cos(ang));
      float maxA = uPower * 3.14159265;
      float arc = abs(ang) > maxA ? 0.0 : smoothstep(0.0, 0.22, maxA - abs(ang));
      float pulse = 0.9 + 0.1 * sin(uTime*uPulseFreq - abs(ang)*2.0);
      inten *= pulse;
      // impact ripples (additive bright rings) — confined to the shield band + arc so the
      // expanding ring never spills to the quad edges (which clipped into square artifacts)
      float bandMask = 1.0 - smoothstep(0.0, 0.133 * bw, dr);
      float shock = 0.0;
      for (int i=0;i<8;i++){
        if (i >= uHitCount) break;
        vec2 dp = pos - uHits[i].xy; float hd = length(dp);
        float sr = uHits[i].z * 0.31; float sd = abs(hd - sr); float th = 0.044;
        if (sd < th) shock += (1.0 - sd/th) * pow(1.0 - uHits[i].z, 2.0);
      }
      shock *= bandMask * arc;
      vec3 scol = mix(uColor1, uColor2, smoothstep(0.0, 1.0, core*2.0));
      vec3 col = scol * inten * arc * 2.4 + shock * vec3(1.0,0.85,0.55) * 1.3;
      float a = clamp(inten * arc + shock * 0.5, 0.0, 1.0);
      // additive: only adds light, never paints/darkens the background
      gl_FragColor = vec4(col, a);
    }`;
  const VS_POINT = `
    precision highp float;
    attribute vec2 aPos;     // screen px
    attribute float aSize;   // css px
    attribute vec4 aColor;
    uniform vec2 uView; uniform float uDpr;
    varying vec4 vColor;
    void main(){
      vColor = aColor;
      gl_Position = vec4(aPos.x/uView.x*2.0-1.0, 1.0-aPos.y/uView.y*2.0, 0.0, 1.0);
      gl_PointSize = aSize * uDpr;
    }`;
  const FS_POINT = `
    precision highp float;
    varying vec4 vColor;
    void main(){
      vec2 d = gl_PointCoord - 0.5;
      float r = length(d) * 2.0;
      if (r > 1.0) discard;
      float a = 1.0 - r; a = a*a;
      vec3 col = vColor.rgb + (1.0 - r) * 0.5; // hot white core
      gl_FragColor = vec4(col, vColor.a * a);
    }`;
  // heat marks: warm additive glow clipped to the hull silhouette (sampled from the
  // ship diffuse alpha) so smoldering damage never paints outside the corpus
  const FS_HEAT = `
    precision highp float;
    varying vec2 vUv;
    uniform sampler2D uDiff;
    uniform int uCount;
    uniform vec4 uMarks[16];   // xy = uv center, z = radius(uv), w = intensity
    void main(){
      float alpha = texture2D(uDiff, vUv).a;
      if (alpha < 0.1) discard;                 // strictly inside the hull
      float h = 0.0;
      for (int i = 0; i < 16; i++) {
        if (i >= uCount) break;
        float d = length(vUv - uMarks[i].xy) / max(uMarks[i].z, 0.001);
        if (d < 1.0) h += pow(1.0 - d, 2.0) * uMarks[i].w;
      }
      if (h <= 0.0) discard;
      h = clamp(h, 0.0, 1.3);
      vec3 col = mix(vec3(0.55, 0.04, 0.015), vec3(1.0, 0.42, 0.1), clamp(h - 0.45, 0.0, 1.0)) * h;
      gl_FragColor = vec4(col * alpha, 1.0);
    }`;
  // impact spark: radial gradient white core -> yellow -> orange -> dark red edge (additive)
  const FS_SPARK = `
    precision highp float;
    varying vec4 vColor;        // a = life fade
    void main(){
      vec2 d = gl_PointCoord - 0.5;
      float r = length(d) * 2.0;
      if (r > 1.0) discard;
      vec3 c = mix(vec3(1.0, 0.55, 0.5), vec3(1.0, 0.02, 0.06), r) * 1.9;
      float a = 1.0 - r * r;   // fuller, denser core
      gl_FragColor = vec4(c, vColor.a * a);
    }`;

  function texFromImage(gl, img) {
    const t = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, t);
    gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
    gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, img);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    return t;
  }

  function create(canvas) {
    const gl = canvas.getContext('webgl', { alpha: false, antialias: true, premultipliedAlpha: false, preserveDrawingBuffer: false })
            || canvas.getContext('experimental-webgl');
    if (!gl) return null;

    const progSprite = program(gl, VS_ROT, FS_SPRITE);
    const progBeam = program(gl, VS_ROT, FS_BEAM);
    const progBlit = program(gl, VS_QUAD, FS_BLIT);
    const progBG = program(gl, VS_QUAD, FS_BG);
    const progShield = program(gl, VS_QUAD, FS_SHIELD);
    const progPoint = program(gl, VS_POINT, FS_POINT);
    const progHeat = program(gl, VS_ROT, FS_HEAT);
    const progSpark = program(gl, VS_POINT, FS_SPARK);
    // DEBUG: hollow ring outline (red = caustic center, blue = laser↔shield)
    const FS_DBGRING = `
      precision highp float;
      varying vec4 vColor;
      void main(){
        vec2 d = gl_PointCoord - 0.5; float r = length(d) * 2.0;
        float ring = (1.0 - smoothstep(0.78, 0.94, r)) * smoothstep(0.6, 0.76, r);
        if (ring <= 0.01) discard;
        gl_FragColor = vec4(vColor.rgb, ring);
      }`;
    const progDbgRing = program(gl, VS_POINT, FS_DBGRING);
    if (!progSprite || !progShield || !progPoint) return null;

    // unit quad
    const quad = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, quad);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0,0, 1,0, 0,1, 0,1, 1,0, 1,1]), gl.STATIC_DRAW);

    // dynamic point buffer
    const MAXP = 8000;
    const pointBuf = gl.createBuffer();
    const pointArr = new Float32Array(MAXP * 7); // x,y,size,r,g,b,a

    // scene FBO
    let fbo = null, fboTex = null, fboW = 0, fboH = 0;
    function ensureFBO(w, h) {
      if (fbo && fboW === w && fboH === h) return;
      fboW = w; fboH = h;
      if (fboTex) gl.deleteTexture(fboTex);
      if (fbo) gl.deleteFramebuffer(fbo);
      fboTex = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, fboTex);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      fbo = gl.createFramebuffer();
      gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
      gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, fboTex, 0);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    }

    // textures
    const tex = { ready: false };
    const srcs = [['shipD','textures/sprites/ship.png'],['shipN','textures/sprites/ship_n.png'],['destD','textures/sprites/destroyer.png'],['destN','textures/sprites/destroyer_n.png'],['frigD','textures/sprites/frigate.png'],['frigN','textures/sprites/frigate_n.png'],['astD','textures/sprites/asteroid.png'],['astN','textures/sprites/asteroid_n.png'],['t2cD','textures/sprites/t2c.png'],['t2cN','textures/sprites/t2c_n.png'],['t2lD','textures/sprites/t2l.png'],['t2lN','textures/sprites/t2l_n.png'],['t2mD','textures/sprites/t2m.png'],['t2mN','textures/sprites/t2m_n.png'],['tlasD','textures/sprites/tlas.png'],['tlasN','textures/sprites/tlas_n.png'],['trkD','textures/sprites/trk.png'],['trkN','textures/sprites/trk_n.png']];
    let loaded = 0;
    srcs.forEach(([key, src]) => {
      const im = new Image();
      im.onload = () => { tex[key] = texFromImage(gl, im); tex[key + 'W'] = im.naturalWidth; tex[key + 'H'] = im.naturalHeight; if (++loaded === srcs.length) tex.ready = true; };
      im.onerror = () => { if (++loaded === srcs.length) tex.ready = true; };
      im.src = src;
    });

    function bindQuad(prog) {
      gl.bindBuffer(gl.ARRAY_BUFFER, quad);
      gl.enableVertexAttribArray(prog._a.aPos);
      gl.vertexAttribPointer(prog._a.aPos, 2, gl.FLOAT, false, 0, 0);
    }

    let Wpx, Hpx, Wcss, Hcss, dpr;
    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      Wcss = canvas.clientWidth || canvas.offsetWidth;
      Hcss = canvas.clientHeight || canvas.offsetHeight;
      Wpx = canvas.width = Math.round(Wcss * dpr);
      Hpx = canvas.height = Math.round(Hcss * dpr);
      ensureFBO(Wpx, Hpx);
    }

    // map world -> screen css px. ISOTROPIC: a single pixels-per-world-unit scale (zoom*Hcss)
    // on BOTH axes, so a world circle renders as a true screen circle (was anisotropic: x scaled
    // by Wcss, y by Hcss, which stretched the world horizontally and forced per-axis fudges in
    // the sim). Center offsets keep the camera-centred framing. Vertical mapping is unchanged
    // vs. the old code; horizontal now matches it (no more aspect stretch).
    function worldScale(B) { return B.zoom * Hcss; }
    function sx(B, x) { return (x - B.camX) * worldScale(B) + Wcss * 0.5; }
    function sy(B, y) { return (y - B.camY) * worldScale(B) + Hcss * 0.5; }

    // active impact point-lights for the sprite shader (populated each frame in render)
    let plN = 0;
    const plArr = new Float32Array(32);  // 8 * vec4 (xy, radius, intensity)
    const plCol = new Float32Array(24);  // 8 * vec3

    function drawSprite(prog, diff, norm, cx, cy, sizeW, sizeH, rot, tint, hit, amb, metal, rough, glow, glowCol, alpha) {
      gl.useProgram(prog);
      bindQuad(prog);
      gl.uniform2f(prog._u.uView, Wcss, Hcss);
      gl.uniform2f(prog._u.uCenter, cx, cy);
      gl.uniform2f(prog._u.uSize, sizeW, sizeH);
      gl.uniform1f(prog._u.uRot, rot);
      gl.uniform3f(prog._u.uLight, -0.4, -0.5, 1.0);   // more overhead (was -0.5,-0.62,0.62): grazing light made facing-flipped ships differ drastically; higher Z keeps NoL stable across in-plane rotation
      gl.uniform3f(prog._u.uTint, tint[0], tint[1], tint[2]);
      gl.uniform1f(prog._u.uHit, hit || 0);
      if (prog._u.uGlow) gl.uniform1f(prog._u.uGlow, glow || 0);
      if (prog._u.uGlowCol) { const gc = glowCol || [1, 1, 1]; gl.uniform3f(prog._u.uGlowCol, gc[0], gc[1], gc[2]); }
      if (prog._u.uAlpha) gl.uniform1f(prog._u.uAlpha, alpha == null ? 1 : alpha);
      gl.uniform1f(prog._u.uAmb, amb);
      gl.uniform1f(prog._u.uMetal, metal == null ? 0.6 : metal);   // material: ship metal, asteroid stone
      gl.uniform1f(prog._u.uRough, rough == null ? 0.4 : rough);
      gl.uniform1i(prog._u.uPLN, plN);
      if (plN > 0 && prog._u.uPL) { gl.uniform4fv(prog._u.uPL, plArr); gl.uniform3fv(prog._u.uPLC, plCol); }
      gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, diff); gl.uniform1i(prog._u.uDiff, 0);
      gl.activeTexture(gl.TEXTURE1); gl.bindTexture(gl.TEXTURE_2D, norm); gl.uniform1i(prog._u.uNorm, 1);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    }

    function drawBeamQuad(x1, y1, x2, y2, thick, core, glow, intensity, fadeX) {
      const dx = x2 - x1, dy = y2 - y1;
      const len = Math.hypot(dx, dy);
      if (len < 0.5) return;
      const rot = Math.atan2(dy, dx);
      gl.useProgram(progBeam);
      bindQuad(progBeam);
      gl.uniform2f(progBeam._u.uView, Wcss, Hcss);
      gl.uniform2f(progBeam._u.uCenter, (x1 + x2) / 2, (y1 + y2) / 2);
      gl.uniform2f(progBeam._u.uSize, len, thick);
      gl.uniform1f(progBeam._u.uRot, rot);
      gl.uniform3f(progBeam._u.uCore, core[0], core[1], core[2]);
      gl.uniform3f(progBeam._u.uGlow, glow[0], glow[1], glow[2]);
      gl.uniform1f(progBeam._u.uIntensity, intensity);
      gl.uniform1f(progBeam._u.uFadeX, fadeX);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    }

    // smoldering heat glow per ship, masked to the hull silhouette (additive)
    const heatMarks = new Float32Array(64); // 16 * vec4
    function drawHeat(B) {
      if (!tex.ready || !tex.shipD) return;
      gl.useProgram(progHeat);
      bindQuad(progHeat);
      gl.blendFunc(gl.ONE, gl.ONE);
      gl.uniform2f(progHeat._u.uView, Wcss, Hcss);
      for (const s of B.ships) {
        if (s.dead || !s.heat || !s.heat.length) continue;
        const k = (s.hullTex && tex[s.hullTex + 'D']) ? s.hullTex : 'ship';
        const iw = tex[k + 'DW'], ih = tex[k + 'DH'];
        const dw = s.radius * 3.4 * B.zoom, dh = dw * ih / iw;
        let c = 0;
        for (const hm of s.heat) {
          if (c >= 16 || hm.u == null) continue;
          const t = hm.t * hm.t;
          if (t <= 0.001) continue;
          heatMarks[c * 4] = hm.u; heatMarks[c * 4 + 1] = hm.v;
          heatMarks[c * 4 + 2] = hm.r * 0.235;   // uv radius (≈3x smaller than the old point glow)
          heatMarks[c * 4 + 3] = t;
          c++;
        }
        if (!c) continue;
        gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, tex[k + 'D']); gl.uniform1i(progHeat._u.uDiff, 0);
        gl.uniform2f(progHeat._u.uCenter, sx(B, s.x), sy(B, s.y));
        gl.uniform2f(progHeat._u.uSize, dw, dh);
        gl.uniform1f(progHeat._u.uRot, s.angle + Math.PI);
        gl.uniform1i(progHeat._u.uCount, c);
        gl.uniform4fv(progHeat._u.uMarks, heatMarks.subarray(0, c * 4));
        gl.drawArrays(gl.TRIANGLES, 0, 6);
      }
    }

    // shield-impact ripples, gathered in screen UV space and refracted in the blit pass
    const ripArr = new Float32Array(40); // 10 * vec4
    let ripN = 0;

    // impact sparks as their own pass so they get the bright radial gradient (FS_SPARK)
    function drawSparks(B) {
      let n = 0; const arr = pointArr;
      for (const p of B.parts) {
        if (!p.spark || n >= MAXP) continue;
        const o = n * 7;
        arr[o] = sx(B, p.x); arr[o + 1] = sy(B, p.y); arr[o + 2] = p.r * 2.4 * B.zoom;
        arr[o + 3] = 1; arr[o + 4] = 1; arr[o + 5] = 1; arr[o + 6] = Math.max(0, p.life / 30);
        n++;
      }
      if (!n) return;
      gl.useProgram(progSpark);
      gl.bindBuffer(gl.ARRAY_BUFFER, pointBuf);
      gl.bufferData(gl.ARRAY_BUFFER, arr.subarray(0, n * 7), gl.DYNAMIC_DRAW);
      const stride = 7 * 4;
      gl.enableVertexAttribArray(progSpark._a.aPos); gl.vertexAttribPointer(progSpark._a.aPos, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(progSpark._a.aSize); gl.vertexAttribPointer(progSpark._a.aSize, 1, gl.FLOAT, false, stride, 8);
      gl.enableVertexAttribArray(progSpark._a.aColor); gl.vertexAttribPointer(progSpark._a.aColor, 4, gl.FLOAT, false, stride, 12);
      gl.uniform2f(progSpark._u.uView, Wcss, Hcss);
      gl.uniform1f(progSpark._u.uDpr, dpr);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      gl.drawArrays(gl.POINTS, 0, n);
    }

    function render(B) {
      if (!Wpx) resize();
      // expose the exact Wcss/Hcss this frame uses so the DC-side overlay (_renderDmgNums/_toScreen)
      // can use identical scale values — prevents Y drift caused by clientHeight vs cached Hcss mismatch
      B.gl_Wcss = Wcss; B.gl_Hcss = Hcss;
      gl.viewport(0, 0, Wpx, Hpx);
      // ---------- pass 1: scene -> FBO ----------
      gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
      gl.clearColor(0.018, 0.026, 0.040, 1.0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.disable(gl.BLEND);
      // background
      gl.useProgram(progBG);
      bindQuad(progBG);
      gl.uniform2f(progBG._u.uView, Wcss, Hcss);
      gl.uniform2f(progBG._u.uCenter, Wcss / 2, Hcss / 2);
      gl.uniform2f(progBG._u.uSize, Wcss, Hcss);
      gl.uniform2f(progBG._u.uCam, B.camX, B.camY);
      gl.uniform1f(progBG._u.uZoom, B.zoom);
      gl.uniform1f(progBG._u.uAspect, Wcss / Hcss);
      gl.drawArrays(gl.TRIANGLES, 0, 6);

      if (!tex.ready) { blitToScreen(); return; }
      // collect impact flash point-lights (illuminate hulls + asteroids via normal map)
      plN = 0;
      for (const p of B.parts) {
        if (!p.flash || plN >= 8) continue;
        const a = Math.max(0, p.life / p.life0);
        const inten = a * a * 2.2 * (p.li == null ? 1 : p.li);
        if (inten <= 0.01) continue;
        const o = plN * 4;
        plArr[o] = sx(B, p.x); plArr[o + 1] = sy(B, p.y);
        plArr[o + 2] = (p.r0 || 13) * 3.5 * B.zoom;   // illumination radius (px)
        plArr[o + 3] = inten;
        const c = p.col || [1, 0.95, 0.78];
        const co = plN * 3;
        plCol[co] = c[0]; plCol[co + 1] = c[1]; plCol[co + 2] = c[2];
        plN++;
      }

      // asteroids (normal mapped)
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      if (tex.astD && B.asteroids) {
        const aw = tex.astDW, ah = tex.astDH;
        for (const a of B.asteroids) {
          // asteroid parallax layer — sits slightly behind the action (factor < 1 ⇒ drifts a
          // touch slower than the ships as the camera pans), giving the field depth.
          // parallax: asteroids track the camera slightly slower for depth. Isotropic mapping
          // (single zoom*Hcss scale) — same as sx()/sy() but with the parallaxed camera offset.
          const p = a.par != null ? a.par : 0.82;
          const axs = (a.x - B.camX * p) * worldScale(B) + Wcss * 0.5;
          const ays = (a.y - B.camY * p) * worldScale(B) + Hcss * 0.5;
          const dw = 132 * a.scale * B.zoom, dh = dw * ah / aw;
          drawSprite(progSprite, tex.astD, tex.astN, axs, ays, dw, dh, a.rot, [0.95, 0.93, 0.86], 0, 0.4, 0.0, 0.92); // stone: non-metallic, very rough
        }
      }
      // engine flames — drawn BEFORE the hull so the nozzles sit behind the ship
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      drawFlames(B);
      // ships (normal mapped + hit flash)
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      if (tex.shipD) {
        for (const s of B.ships) {
          if (s.dead && !(s.vanish > 0)) continue;   // keep drawing while it fades out during the warp
          // per-hull texture: use this ship's hull sprite if loaded, else the default ship
          const k = (s.hullTex && tex[s.hullTex + 'D']) ? s.hullTex : 'ship';
          const D = tex[k + 'D'], N = tex[k + 'N'], iw = tex[k + 'DW'], ih = tex[k + 'DH'];
          const dw = s.radius * 3.4 * B.zoom, dh = dw * ih / iw;
          // per-ship material override (editor preview sets these; battle ships leave them
          // undefined → identical to before: neutral tint, metal 0.6, rough 0.4)
          const tint = s.tint || [1.0, 1.0, 1.0];
          const mMetal = (s.metal == null ? 0.6 : s.metal);
          const mRough = (s.rough == null ? 0.4 : s.rough);
          const va = (s.vanish != null ? s.vanish : 1);          // 1 -> 0 alpha fade during detonation warp
          const hitFlash = (s.hitT > 0 ? s.hitT * 0.7 : 0);      // white hit flash only
          const heatGlow = s.dieGlow || 0;                       // dark-red death heat
          drawSprite(progSprite, D, N, sx(B, s.x), sy(B, s.y), dw, dh, s.angle + Math.PI, tint, hitFlash, 0.42, mMetal, mRough, heatGlow, [0.85, 0.05, 0.02], va);
        }
      }
      // TURRETS — drawn in the scene with the SAME PBR sprite shader as hulls, so they share the
      // directional light, normal-mapped shading, impact point-lights, the white hit flash, the
      // red→white death burn, and the post-pass space-warp / caustic. (Moved here from the old flat
      // 2D overlay, which ignored all hull lighting.) Each turret carries a precomputed `tr.gl`
      // block from the sim (texKey, tint, metal, rough, scale); aim is resolved here from s.target.
      if (tex.shipD) {
        for (const s of B.ships) {
          if (s.dead && !(s.vanish > 0)) continue;       // fade out with the hull during the warp
          const list = s._turrets; if (!list || !list.length) continue;
          const aspect = s.hullAspect || 0.44;
          const bfwx = Math.cos(s.angle), bfwy = Math.sin(s.angle);
          const brgx = -Math.sin(s.angle), brgy = Math.cos(s.angle);
          const wW = s.radius * 3.4 / Hcss, wH = wW * aspect;   // world quad size (matches sim fire code)
          const dwHull = s.radius * 3.4 * B.zoom;               // hull screen width (turret scale is × this)
          const tgt = (s.target && !s.target.dead) ? s.target : null;
          const hitFlash = (s.hitT > 0 ? s.hitT * 0.7 : 0);     // shared white hit flash
          const heatGlow = s.dieGlow || 0;                      // shared red→white death burn
          const va = (s.vanish != null ? s.vanish : 1);         // shared detonation fade
          for (const tr of list) {
            const g = tr.gl; if (!g) continue;
            const D = tex[g.texKey + 'D'], N = tex[g.texKey + 'N']; if (!D) continue;
            const iw = tex[g.texKey + 'DW'], ih = tex[g.texKey + 'DH'];
            const slotWX = s.x + bfwx * (tr.x * wW) + brgx * (tr.y * wH);
            const slotWY = s.y + bfwy * (tr.x * wW) + brgy * (tr.y * wH);
            const drawW = dwHull * (g.scale || 0.45), drawH = drawW * (ih / iw);
            // aim (world angle): toward the ship's current target, else swing about hull forward.
            // Same left-facing-art convention as the hull (drawn rot = aim + π).
            let aim;
            if (g.fixed) { aim = s.angle; }   // stationary turret: locked to hull forward, no tracking
            else if (tgt) { aim = Math.atan2(tgt.y - slotWY, tgt.x - slotWX); }
            else {
              const st = tr.st || (tr.st = { dir: 1, t: 30 });
              st.t -= 1; if (st.t <= 0) { st.dir *= -1; st.t = 60 + Math.random() * 90; }
              aim = s.angle + st.dir * 0.12;
            }
            drawSprite(progSprite, D, N, sx(B, slotWX), sy(B, slotWY), drawW, drawH, aim + Math.PI, g.tint, hitFlash, 0.42, g.metal, g.rough, heatGlow, [0.85, 0.05, 0.02], va);
          }
        }
      }
      // additive layer: beams, tracers
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      const beamHits = [];
      for (const s of B.ships) {
        if (s.dead || !(s.beam > 0.02) || !s.beamTgt || s.beamTgt.dead) continue;
        // beam starts at the fitted laser turret's muzzle (s.beamMuzWX/WY from the sim) when present,
        // else the ship centre. Trim the far end to the exact endpoint the sim provides.
        const ox = (s.beamMuzWX != null) ? s.beamMuzWX : s.x;
        const oy = (s.beamMuzWY != null) ? s.beamMuzWY : s.y;
        let x1 = sx(B, ox), y1 = sy(B, oy);
        let x2 = sx(B, s.beamTX), y2 = sy(B, s.beamTY);
        // trim BOTH ends to hull surfaces so the beam never paints across either hull
        const bdx = x2 - x1, bdy = y2 - y1, blen = Math.hypot(bdx, bdy) || 1;
        const ux = bdx / blen, uy = bdy / blen;
        // small advance off the turret muzzle (nose offset only when falling back to ship centre)
        const muzzle = (s.beamMuzWX != null) ? 0 : s.radius * 1.35 * B.zoom;
        const surf = 0;                                   // sim provides exact endpoint (hull point / shield surface)
        x1 += ux * muzzle; y1 += uy * muzzle;
        x2 -= ux * surf; y2 -= uy * surf;
        const jit = s.beam > 0.6 ? (0.8 + Math.random() * 0.4) : 1;
        // beam colours/widths come from the fitted beam preset (s.beamCore/Glow/GlowOuter + widths);
        // default to the original red laser when a ship has no per-beam override.
        const bCoreOuter = s.beamCoreOuter || [1, 0.86, 0.84];
        const bGlowOuter = s.beamGlowOuter || [1, 0.1, 0.14];
        const bCore = s.beamCore || [1, 0.95, 0.92];
        const bGlow = s.beamGlow || [1, 0.35, 0.3];
        const wGlow = (s.beamGlowW || 3), wCore = (s.beamCoreW || 1.13);
        drawBeamQuad(x1, y1, x2, y2, wGlow * B.zoom * s.beam, bCoreOuter, bGlowOuter, 0.5 * s.beam * jit, 0);
        drawBeamQuad(x1, y1, x2, y2, wCore * B.zoom * s.beam, bCore, bGlow, s.beam * jit, 0);
        // round, flickering impact glow (drawn later as a circular point sprite)
        if (s.beam > 0.4) {
          beamHits.push({ x: x2, y: y2, r: (7 + Math.random() * 4) * B.zoom * s.beam, i: s.beam * (0.85 + Math.random() * 0.3), col: s.beamImpact || null });
        }
      }
      for (const p of B.projs) {
        if (p.missile) {
          // guided ordnance: warm elongated body + white-hot nose; the exhaust torch is the
          // particle trail emitted each frame (drawFlames pass), so this is just the body.
          const m1x = sx(B, p.x), m1y = sy(B, p.y);
          const mt = (p.tail != null) ? p.tail : 3;
          const m2x = sx(B, p.x - p.vx * mt), m2y = sy(B, p.y - p.vy * mt);
          const mhx = sx(B, p.x - p.vx * 0.7), mhy = sy(B, p.y - p.vy * 0.7);
          const mwg = (p.wg != null ? p.wg : 6.5) * B.zoom, mwc = (p.wc != null ? p.wc : 2.8) * B.zoom;
          const mgc = p.gc || [1, 0.5, 0.2], mcc = p.cc || [1, 0.95, 0.82];
          drawBeamQuad(m1x, m1y, m2x, m2y, mwg, mgc, mgc, 0.75, 0.3);    // warm body glow
          drawBeamQuad(m1x, m1y, mhx, mhy, mwc, [1, 1, 1], mcc, 1.5, 0.1); // white-hot nose
          continue;
        }
        // green player blaster matches the small gun-bullet look (no 5× player upscale)
        const smallBullet = (p.type !== 'he' && p.side === 'player');
        const tailMul = (p.tail != null) ? p.tail : ((p.side === 'player' && !smallBullet) ? 5 : 0.333);
        const x1 = sx(B, p.x), y1 = sy(B, p.y);
        const x2 = sx(B, p.x - p.vx * tailMul), y2 = sy(B, p.y - p.vy * tailMul);   // streak tail
        const coreMul = (p.side === 'player' && !smallBullet) ? 1.5 : 0.5;
        const hx = sx(B, p.x - p.vx * coreMul), hy = sy(B, p.y - p.vy * coreMul); // short core
        let core, glow, wGlow, wCore;
        const eScale = (p.side === 'enemy' || smallBullet) ? 0.2 : 1;   // pirate + player gun bullets 5x smaller
        if (p.type === 'he') {
          const heScale = eScale * 0.2;   // yellow HE rounds 5x smaller
          wGlow = 13 * B.zoom * heScale; wCore = 5 * B.zoom * heScale;
          core = [1, 0.95, 0.7];
          glow = p.side === 'player' ? [1, 0.55, 0.12] : [1, 0.4, 0.12];
        } else { // gun / kinetic
          wGlow = 7 * B.zoom * eScale; wCore = 2.8 * B.zoom * eScale;
          if (p.side === 'player') { core = [0.85, 1, 0.9]; glow = [0.15, 1, 0.4]; }
          else { core = [1, 0.86, 0.78]; glow = [1, 0.3, 0.22]; }
        }
        // editor preset overrides (battle projectiles never set these → unchanged in combat)
        if (p.gc) glow = p.gc;
        if (p.cc) core = p.cc;
        if (p.wg != null) wGlow = p.wg * B.zoom;
        if (p.wc != null) wCore = p.wc * B.zoom;
        drawBeamQuad(x1, y1, x2, y2, wGlow, glow, glow, 0.6, 0.35);      // soft outer glow
        drawBeamQuad(x1, y1, hx, hy, wCore, [1, 1, 1], core, 1.4, 0.1);  // white-hot core
      }
      // additive points: flames / sparks / blast / heat + beam impacts
      drawPoints(B, beamHits);
      drawSparks(B);
      drawHeat(B);
      // shields drawn into the scene FBO (additive) so they stay locked to their hulls
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      gl.useProgram(progShield);
      bindQuad(progShield);
      gl.uniform2f(progShield._u.uView, Wcss, Hcss);
      gl.uniform1f(progShield._u.uTime, B.t);
      const hitArr = new Float32Array(32);
      ripN = 0;
      if (B) {
        B.__dbg = { cam: { x: B.camX, y: B.camY }, zoom: B.zoom, hits: [] };
        for (const s of B.ships) {
          if (s.dead) continue;
          if (s.side === 'player' && !B.__dbg.player) B.__dbg.player = { x: sx(B, s.x), y: sy(B, s.y) };
          if (s.side === 'enemy' && !B.__dbg.enemy) B.__dbg.enemy = { x: sx(B, s.x), y: sy(B, s.y) };
          // laser/shield intersection = the beam endpoint while a beam is live
          if (s.beam > 0.6 && s.beamTgt && !s.beamTgt.dead && s.beamTX != null) {
            B.__dbg.laser = { x: sx(B, s.beamTX), y: sy(B, s.beamTY) };
            B.__dbg.laserTargetSide = s.beamTgt.side;
          }
        }
      }
      for (const s of B.ships) {
        if (s.dead) continue;
        const dep = s.shieldDeploy || 0;
        if (dep <= 0.001) continue;
        const size = s.radius * B.zoom * 6.75;   // larger quad so the shield ring + ripple fit inside the inscribed circle (no square clipping)
        const scx = sx(B, s.x), scy = sy(B, s.y);
        gl.uniform2f(progShield._u.uCenter, scx, scy);
        gl.uniform2f(progShield._u.uSize, size, size);
        gl.uniform1f(progShield._u.uPower, (s.shieldArc != null ? s.shieldArc : 0.66) * dep);
        gl.uniform1f(progShield._u.uShipRot, s.shieldA);
        // per-ship shield look (defaults to amber when unset — battle ships)
        const c1 = s.shieldCol1 || [1.0, 0.33, 0.0], c2 = s.shieldCol2 || [1.0, 0.6, 0.0];
        gl.uniform3f(progShield._u.uColor1, c1[0], c1[1], c1[2]);
        gl.uniform3f(progShield._u.uColor2, c2[0], c2[1], c2[2]);
        gl.uniform1f(progShield._u.uPulseFreq, s.shieldFlicker != null ? s.shieldFlicker : 5.0);
        gl.uniform1f(progShield._u.uBandWidth, s.shieldBand != null ? s.shieldBand : 1.0);
        let hc = 0;
        if (s.sHits) {
          for (const sh of s.sHits) {
            // screen point of the ACTUAL impact: world endpoint (ground truth, same coords the beam
            // line uses) mapped via sx/sy. Falls back to the screen-angle on the bubble if no world pt.
            let rcx, rcy;
            if (sh.wx != null) { rcx = sx(B, sh.wx); rcy = sy(B, sh.wy); }
            else { rcx = scx + Math.cos(sh.ang) * 0.31 * size; rcy = scy + Math.sin(sh.ang) * 0.31 * size; }
            if (hc < 8) {
              // bright hit ring on the bubble, in the shield quad's local uv (offset / size)
              hitArr[hc*4] = (rcx - scx) / size; hitArr[hc*4+1] = (rcy - scy) / size;
              hitArr[hc*4+2] = 1 - sh.t; hitArr[hc*4+3] = 1.0; hc++;
            }
            if (ripN < 10) {
              const age = 1 - sh.t;
              ripArr[ripN*4]   = rcx / Wcss;
              // FBO texture has OpenGL y-convention (y=0 at bottom) while CSS/screen uses y=0
              // at top. Flip Y so the caustic center lands on the correct fragment in FS_BLIT.
              ripArr[ripN*4+1] = 1.0 - rcy / Hcss;
              ripArr[ripN*4+2] = age * 0.31 * size / Hcss;   // leading-edge radius (height-fraction)
              ripArr[ripN*4+3] = sh.t;                        // amp fades as the wave expands
              ripN++;
              if (B.__dbg && B.__dbg.hits.length < 10) {
                // debug: store SCREEN-space coords (y-down, for rings drawn directly on canvas)
                B.__dbg.hits.push({ side: s.side, shieldX: rcx, shieldY: rcy, causticX: rcx, causticY: rcy });
              }
            }
          }
        }
        gl.uniform1i(progShield._u.uHitCount, hc);
        gl.uniform4fv(progShield._u.uHits, hitArr);
        gl.drawArrays(gl.TRIANGLES, 0, 6);
      }

      // explosion space-warp shockwaves: strong expanding refraction rings (no shield needed)
      if (B.warps) {
        for (const w of B.warps) {
          if (ripN >= 10) break;
          const age = 1 - w.t;                       // 0 -> 1 as it expands
          const wcx = sx(B, w.x), wcy = sy(B, w.y);
          ripArr[ripN*4]   = wcx / Wcss;
          ripArr[ripN*4+1] = 1.0 - wcy / Hcss;       // FBO y-flip (same convention as shield ripples)
          ripArr[ripN*4+2] = age * 0.1551 * ((B && B.zoom) || 1);  // world-anchored radius: scales with zoom (3× larger)
          ripArr[ripN*4+3] = w.t * 14.0;             // stronger amplitude (×2), fades as it expands
          ripN++;
        }
      }

      // ---------- pass 2: blit scene to screen ----------
      blitToScreen(B);
      drawDebugRings(B);
    }

    // DEBUG: red ring at each caustic center, blue ring at laser↔shield intersection
    const dbgArr = new Float32Array(12 * 7);
    function drawDebugRings(B) {
      return;   // debug red/blue test rings disabled
      if (!B || !B.__dbg) return;
      let n = 0;
      const put = (x, y, rd, gn, bl) => {
        const o = n * 7; dbgArr[o] = x; dbgArr[o+1] = y; dbgArr[o+2] = 40;
        dbgArr[o+3] = rd; dbgArr[o+4] = gn; dbgArr[o+5] = bl; dbgArr[o+6] = 1; n++;
      };
      // red ring only on the laser TARGET's caustic (avoids confusion with the shooter's own shield)
      for (const h of B.__dbg.hits) { if (n < 11 && h.side === B.__dbg.laserTargetSide) put(h.causticX, h.causticY, 1, 0.15, 0.15); }
      if (B.__dbg.laser && n < 12) put(B.__dbg.laser.x, B.__dbg.laser.y, 0.3, 0.6, 1);
      if (!n) return;
      gl.useProgram(progDbgRing);
      gl.bindBuffer(gl.ARRAY_BUFFER, pointBuf);
      gl.bufferData(gl.ARRAY_BUFFER, dbgArr.subarray(0, n * 7), gl.DYNAMIC_DRAW);
      const stride = 7 * 4;
      gl.enableVertexAttribArray(progDbgRing._a.aPos); gl.vertexAttribPointer(progDbgRing._a.aPos, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(progDbgRing._a.aSize); gl.vertexAttribPointer(progDbgRing._a.aSize, 1, gl.FLOAT, false, stride, 8);
      gl.enableVertexAttribArray(progDbgRing._a.aColor); gl.vertexAttribPointer(progDbgRing._a.aColor, 4, gl.FLOAT, false, stride, 12);
      gl.uniform2f(progDbgRing._u.uView, Wcss, Hcss);
      gl.uniform1f(progDbgRing._u.uDpr, dpr);
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      gl.drawArrays(gl.POINTS, 0, n);
    }

    function blitToScreen(B) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.viewport(0, 0, Wpx, Hpx);
      gl.disable(gl.BLEND);
      gl.useProgram(progBlit);
      bindQuad(progBlit);
      gl.uniform2f(progBlit._u.uView, Wcss, Hcss);
      gl.uniform2f(progBlit._u.uCenter, Wcss / 2, Hcss / 2);
      gl.uniform2f(progBlit._u.uSize, Wcss, Hcss);
      gl.activeTexture(gl.TEXTURE0); gl.bindTexture(gl.TEXTURE_2D, fboTex); gl.uniform1i(progBlit._u.uTex, 0);
      gl.uniform1f(progBlit._u.uAspect, Wcss / Hcss);
      gl.uniform1f(progBlit._u.uRipZoom, (B && B.zoom) || 1);
      gl.uniform1i(progBlit._u.uRipCount, ripN);
      gl.uniform4fv(progBlit._u.uRip, ripArr);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    }

    function drawFlames(B) {
      let n = 0;
      const arr = pointArr;
      const push = (x, y, size, r, g, b, a) => {
        if (n >= MAXP) return;
        const o = n * 7; arr[o] = x; arr[o + 1] = y; arr[o + 2] = size; arr[o + 3] = r; arr[o + 4] = g; arr[o + 5] = b; arr[o + 6] = a; n++;
      };
      for (const p of B.parts) {
        if (!p.flame) continue;
        const X = sx(B, p.x), Y = sy(B, p.y);
        const a = Math.max(0, p.life / p.life0);   // 1 fresh -> 0 dead
        let r, g, b, al, size;
        if (p.core) {
          // inner cone — blazing white-blue spine, hot and bright (high-pressure torch)
          // p.cc (editor preset) overrides the base hue; particle still whitens near the nozzle.
          if (p.cc) { const w = a * 0.6; r = p.cc[0] + (1 - p.cc[0]) * w; g = p.cc[1] + (1 - p.cc[1]) * w; b = p.cc[2] + (1 - p.cc[2]) * w; }
          else { r = 0.55 + a * 0.42; g = 0.78 + a * 0.20; b = 1.0; }
          al = a * 1.05 * (p.br == null ? 1 : p.br);
          size = p.r * (0.35 + a * 0.65) * 3.0 * B.zoom;  // thin near nozzle, tapers to a point
        } else {
          // outer envelope — deep blue roar, softer
          if (p.ec) { const m = 0.5 + a * 0.5; r = p.ec[0] * m; g = p.ec[1] * m; b = p.ec[2] * m; }
          else { r = a * 0.34; g = 0.36 + a * 0.40; b = 1.0; }
          al = a * 0.62 * (p.br == null ? 1 : p.br);
          size = p.r * a * 3.0 * B.zoom;
        }
        push(X, Y, size, r, g, b, al);
      }
      if (!n) return;
      gl.useProgram(progPoint);
      gl.bindBuffer(gl.ARRAY_BUFFER, pointBuf);
      gl.bufferData(gl.ARRAY_BUFFER, arr.subarray(0, n * 7), gl.DYNAMIC_DRAW);
      const stride = 7 * 4;
      gl.enableVertexAttribArray(progPoint._a.aPos); gl.vertexAttribPointer(progPoint._a.aPos, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(progPoint._a.aSize); gl.vertexAttribPointer(progPoint._a.aSize, 1, gl.FLOAT, false, stride, 8);
      gl.enableVertexAttribArray(progPoint._a.aColor); gl.vertexAttribPointer(progPoint._a.aColor, 4, gl.FLOAT, false, stride, 12);
      gl.uniform2f(progPoint._u.uView, Wcss, Hcss);
      gl.uniform1f(progPoint._u.uDpr, dpr);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      gl.drawArrays(gl.POINTS, 0, n);
    }

    function drawPoints(B, beamHits) {
      let n = 0;
      const arr = pointArr;
      const push = (x, y, size, r, g, b, a) => {
        if (n >= MAXP) return;
        const o = n * 7; arr[o] = x; arr[o + 1] = y; arr[o + 2] = size; arr[o + 3] = r; arr[o + 4] = g; arr[o + 5] = b; arr[o + 6] = a; n++;
      };
      if (beamHits) { for (const b of beamHits) { const c = b.col || [1.0, 0.42, 0.32]; push(b.x, b.y, b.r * 2.0, c[0], c[1], c[2], Math.min(1, b.i)); push(b.x, b.y, b.r * 0.9, 1.0, 0.95, 0.9, Math.min(1, b.i)); } }
      for (const p of B.parts) {
        const X = sx(B, p.x), Y = sy(B, p.y);
        if (p.flame) {
          continue;   // flames now drawn in drawFlames() — BEFORE the hull
        } else if (p.flash) {
          // brief bright bloom of light at an impact point — expands slightly, fades fast
          const a = Math.max(0, p.life / p.life0);   // 1 -> 0
          const c = p.col || [1, 0.95, 0.78];
          const size = p.r0 * (1.5 - a * 0.6) * 1.4 * B.zoom;  // slight expansion (sprite 30% smaller)
          push(X, Y, size, c[0], c[1], c[2], a * a * 0.95);  // quadratic fade
        } else if (p.blast) {
          const prog = 1 - p.life / p.life0;
          push(X, Y, (p.r0 + prog * p.grow) * 2 * B.zoom, 1.0, 0.55 * (1 - prog), 0.15, (1 - prog) * 0.9);
        } else if (p.ring) {
          const prog = 1 - p.life / p.life0;
          push(X, Y, (p.r0 + prog * p.grow) * 2 * B.zoom, 1.0, 0.7, 0.4, Math.max(0, p.life / p.life0) * 0.5);
        } else if (!p.spark) {
          push(X, Y, p.r * 2.4 * B.zoom, 1.0, 0.82, 0.5, Math.max(0, p.life / 30));
        }
      }
      // heat marks now rendered in drawHeat() — masked to the hull silhouette
      if (!n) return;
      gl.useProgram(progPoint);
      gl.bindBuffer(gl.ARRAY_BUFFER, pointBuf);
      gl.bufferData(gl.ARRAY_BUFFER, arr.subarray(0, n * 7), gl.DYNAMIC_DRAW);
      const stride = 7 * 4;
      gl.enableVertexAttribArray(progPoint._a.aPos);
      gl.vertexAttribPointer(progPoint._a.aPos, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(progPoint._a.aSize);
      gl.vertexAttribPointer(progPoint._a.aSize, 1, gl.FLOAT, false, stride, 8);
      gl.enableVertexAttribArray(progPoint._a.aColor);
      gl.vertexAttribPointer(progPoint._a.aColor, 4, gl.FLOAT, false, stride, 12);
      gl.uniform2f(progPoint._u.uView, Wcss, Hcss);
      gl.uniform1f(progPoint._u.uDpr, dpr);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      gl.drawArrays(gl.POINTS, 0, n);
    }

    const ret = {
      gl: gl,
      resize: resize,
      render: render,
      // exact world->screen of the LAST rendered frame (uses the same cached Wcss/Hcss the
      // renderer drew with). The DC overlay uses this so turret sockets / barrels are anchored
      // to the hull with zero formula drift under any dpr / pan / zoom.
      toScreen: function (B, x, y) { return [sx(B, x), sy(B, y)]; },
      worldScale: function (B) { return worldScale(B); },
      _tex: tex,
      get ready() { return tex.ready; },
      dispose: function () { /* context GC'd with canvas */ }
    };
    window.__BGL = ret;
    return ret;
  }

  window.BattleGL = { create: create };
})();
