import { useEffect, useRef } from "react";

const TARGET_FPS = 24;
const BACKGROUND_FPS = 6;

const vertexShaderSource = `#version 300 es
precision highp float;

in vec2 a_position;
in float a_facet;
in vec2 a_barycentric;

uniform float u_aspect;
uniform float u_motion;
uniform float u_time;
uniform vec2 u_pointer;

out vec3 v_world;
out vec2 v_uv;
out vec3 v_barycentric;
flat out float v_facet;

float clothHeight(vec2 point) {
  float waveA = sin(point.x * 2.35 + u_time * 0.58) * 0.145;
  float waveB = cos(point.y * 3.10 - u_time * 0.43) * 0.112;
  float waveC = sin(point.x * 1.25 + point.y * 1.85 - u_time * 0.29) * 0.082;
  vec2 pointerDelta = point - u_pointer;
  float pointerDistance = length(pointerDelta);
  float pointerWave = exp(-dot(pointerDelta, pointerDelta) * 2.4)
    * sin(pointerDistance * 8.0 - u_time * 1.15)
    * 0.055;
  return (waveA + waveB + waveC + pointerWave) * u_motion;
}

void main() {
  vec2 worldPoint = vec2(
    a_position.x * u_aspect * 1.18,
    a_position.y * 1.16
  );
  float height = clothHeight(worldPoint);
  float perspective = 1.0 + a_position.y * 0.105;
  vec2 projected = vec2(
    a_position.x / perspective,
    (a_position.y * 0.92 + height * 0.72) / perspective
  );

  v_world = vec3(worldPoint, height);
  v_uv = a_position * 0.5 + 0.5;
  v_barycentric = vec3(a_barycentric, 1.0 - a_barycentric.x - a_barycentric.y);
  v_facet = a_facet;
  gl_Position = vec4(projected, 0.32 - height * 0.12, 1.0);
}
`;

const fragmentShaderSource = `#version 300 es
precision highp float;

in vec3 v_world;
in vec2 v_uv;
in vec3 v_barycentric;
flat in float v_facet;

uniform float u_light_mode;

out vec4 outColor;

void main() {
  vec3 normal = normalize(cross(dFdx(v_world), dFdy(v_world)));
  if (normal.z < 0.0) normal = -normal;

  vec3 lightDirection = normalize(vec3(-0.42, 0.58, 0.78));
  float diffuse = max(dot(normal, lightDirection), 0.0);
  float edgeLight = pow(1.0 - clamp(normal.z, 0.0, 1.0), 2.0);
  vec3 darkBase = vec3(0.035, 0.043, 0.064);
  vec3 darkLift = vec3(0.105, 0.130, 0.220);
  vec3 lightBase = vec3(0.835, 0.855, 0.890);
  vec3 lightLift = vec3(0.965, 0.975, 1.000);
  vec3 base = mix(darkBase, lightBase, u_light_mode);
  vec3 lift = mix(darkLift, lightLift, u_light_mode);
  vec3 accent = mix(vec3(0.105, 0.365, 0.420), vec3(0.290, 0.520, 0.590), u_light_mode);

  float shade = clamp(0.14 + diffuse * 0.78 + (v_facet - 0.5) * 0.12, 0.0, 1.0);
  vec3 color = mix(base, lift, shade);
  color += accent * edgeLight * mix(0.22, 0.11, u_light_mode);
  color *= mix(0.88 + v_facet * 0.20, 0.95 + v_facet * 0.08, u_light_mode);

  vec3 edgeWidth = fwidth(v_barycentric) * 1.15;
  vec3 edgeDistance = smoothstep(vec3(0.0), edgeWidth, v_barycentric);
  float triangleInterior = min(edgeDistance.x, min(edgeDistance.y, edgeDistance.z));
  color *= mix(mix(0.86, 0.93, u_light_mode), 1.0, triangleInterior);

  float vignette = smoothstep(0.86, 0.20, distance(v_uv, vec2(0.50)));
  color *= mix(0.78, 1.0, vignette);
  color += accent * smoothstep(0.15, 0.78, v_uv.x) * 0.018;

  outColor = vec4(color, 1.0);
}
`;

export function LowPolyCloth() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const gl = canvas.getContext("webgl2", {
      alpha: false,
      antialias: true,
      depth: false,
      powerPreference: "low-power",
      preserveDrawingBuffer: true,
    });
    if (!gl) {
      canvas.dataset.webgl = "unavailable";
      return;
    }

    let program: WebGLProgram;
    try {
      program = createProgram(gl, vertexShaderSource, fragmentShaderSource);
    } catch (reason) {
      canvas.dataset.webgl = "failed";
      console.warn("GizClaw cloth background is unavailable", reason);
      return;
    }

    const mesh = createMesh(39, 25);
    const vertexBuffer = gl.createBuffer();
    if (!vertexBuffer) {
      canvas.dataset.webgl = "failed";
      gl.deleteProgram(program);
      return;
    }

    gl.bindBuffer(gl.ARRAY_BUFFER, vertexBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, mesh.vertices, gl.STATIC_DRAW);

    const positionLocation = gl.getAttribLocation(program, "a_position");
    const facetLocation = gl.getAttribLocation(program, "a_facet");
    const barycentricLocation = gl.getAttribLocation(program, "a_barycentric");
    if (positionLocation < 0 || facetLocation < 0 || barycentricLocation < 0) {
      canvas.dataset.webgl = "failed";
      gl.deleteBuffer(vertexBuffer);
      gl.deleteProgram(program);
      return;
    }
    const uniforms = {
      aspect: requiredUniform(gl, program, "u_aspect"),
      lightMode: requiredUniform(gl, program, "u_light_mode"),
      motion: requiredUniform(gl, program, "u_motion"),
      pointer: requiredUniform(gl, program, "u_pointer"),
      time: requiredUniform(gl, program, "u_time"),
    };
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    const lightMode = window.matchMedia("(prefers-color-scheme: light)");
    const targetPointer = { x: -10, y: -10 };
    const pointer = { x: -10, y: -10 };
    let aspect = 1;
    let animationFrame = 0;
    let lastFrame = 0;
    let staticFrameRendered = false;

    gl.useProgram(program);
    gl.enableVertexAttribArray(positionLocation);
    gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, 20, 0);
    gl.enableVertexAttribArray(facetLocation);
    gl.vertexAttribPointer(facetLocation, 1, gl.FLOAT, false, 20, 8);
    gl.enableVertexAttribArray(barycentricLocation);
    gl.vertexAttribPointer(barycentricLocation, 2, gl.FLOAT, false, 20, 12);
    gl.disable(gl.DEPTH_TEST);
    canvas.dataset.webgl = "active";

    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      const density = Math.min(window.devicePixelRatio || 1, 1.5);
      const width = Math.max(1, Math.round(rect.width * density));
      const height = Math.max(1, Math.round(rect.height * density));
      if (canvas.width !== width || canvas.height !== height) {
        canvas.width = width;
        canvas.height = height;
      }
      aspect = rect.width / rect.height;
      gl.viewport(0, 0, width, height);
      staticFrameRendered = false;
    };

    const render = (now: number, staticFrame = false) => {
      pointer.x += (targetPointer.x - pointer.x) * 0.075;
      pointer.y += (targetPointer.y - pointer.y) * 0.075;
      gl.clearColor(
        lightMode.matches ? 0.88 : 0.035,
        lightMode.matches ? 0.89 : 0.04,
        lightMode.matches ? 0.92 : 0.055,
        1,
      );
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.uniform1f(uniforms.aspect, aspect);
      gl.uniform1f(uniforms.lightMode, lightMode.matches ? 1 : 0);
      gl.uniform1f(uniforms.motion, staticFrame ? 0.58 : 1);
      gl.uniform2f(uniforms.pointer, pointer.x, pointer.y);
      gl.uniform1f(uniforms.time, staticFrame ? 1.8 : now / 1000);
      gl.drawArrays(gl.TRIANGLES, 0, mesh.vertexCount);
    };

    const tick = (now: number) => {
      animationFrame = window.requestAnimationFrame(tick);
      if (document.hidden) return;
      if (reducedMotion.matches) {
        if (!staticFrameRendered) {
          render(now, true);
          staticFrameRendered = true;
        }
        return;
      }
      staticFrameRendered = false;
      const fps = document.hasFocus() ? TARGET_FPS : BACKGROUND_FPS;
      if (now - lastFrame < 1000 / fps) return;
      lastFrame = now;
      render(now);
    };

    const movePointer = (event: PointerEvent) => {
      const rect = canvas.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      targetPointer.x =
        ((event.clientX - rect.left) / rect.width - 0.5) * aspect * 2.36;
      targetPointer.y = (0.5 - (event.clientY - rect.top) / rect.height) * 2.32;
    };
    const clearPointer = () => {
      targetPointer.x = -10;
      targetPointer.y = -10;
    };
    const refreshStaticFrame = () => {
      staticFrameRendered = false;
    };

    resize();
    render(performance.now(), reducedMotion.matches);
    animationFrame = window.requestAnimationFrame(tick);
    window.addEventListener("resize", resize);
    window.addEventListener("pointermove", movePointer, { passive: true });
    window.addEventListener("pointerleave", clearPointer);
    reducedMotion.addEventListener("change", refreshStaticFrame);
    lightMode.addEventListener("change", refreshStaticFrame);

    return () => {
      window.cancelAnimationFrame(animationFrame);
      window.removeEventListener("resize", resize);
      window.removeEventListener("pointermove", movePointer);
      window.removeEventListener("pointerleave", clearPointer);
      reducedMotion.removeEventListener("change", refreshStaticFrame);
      lightMode.removeEventListener("change", refreshStaticFrame);
      gl.deleteBuffer(vertexBuffer);
      gl.deleteProgram(program);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="cloth-canvas"
      data-target-fps={TARGET_FPS}
      aria-hidden="true"
    />
  );
}

function createMesh(columns: number, rows: number) {
  const vertexCount = (columns - 1) * (rows - 1) * 6;
  const vertices = new Float32Array(vertexCount * 5);
  let offset = 0;
  let triangle = 0;
  const point = (column: number, row: number) => [
    (column / (columns - 1) - 0.5) * 2.74,
    (row / (rows - 1) - 0.5) * 3.6,
  ];
  const writeTriangle = (points: number[][]) => {
    const facet = pseudoRandom(triangle);
    const barycentric = [
      [1, 0],
      [0, 1],
      [0, 0],
    ];
    triangle += 1;
    points.forEach(([x, y], index) => {
      vertices[offset] = x;
      vertices[offset + 1] = y;
      vertices[offset + 2] = facet;
      vertices[offset + 3] = barycentric[index][0];
      vertices[offset + 4] = barycentric[index][1];
      offset += 5;
    });
  };
  for (let row = 0; row < rows - 1; row += 1) {
    for (let column = 0; column < columns - 1; column += 1) {
      const topLeft = point(column, row);
      const topRight = point(column + 1, row);
      const bottomLeft = point(column, row + 1);
      const bottomRight = point(column + 1, row + 1);
      const forwardDiagonal = (row + column) % 2 === 0;
      if (forwardDiagonal) {
        writeTriangle([topLeft, bottomLeft, bottomRight]);
        writeTriangle([topLeft, bottomRight, topRight]);
      } else {
        writeTriangle([topLeft, bottomLeft, topRight]);
        writeTriangle([topRight, bottomLeft, bottomRight]);
      }
    }
  }
  return { vertexCount, vertices };
}

function pseudoRandom(value: number) {
  const random = Math.sin((value + 1) * 12.9898) * 43758.5453;
  return random - Math.floor(random);
}

function createProgram(
  gl: WebGL2RenderingContext,
  vertexSource: string,
  fragmentSource: string,
) {
  const vertexShader = compileShader(gl, gl.VERTEX_SHADER, vertexSource);
  const fragmentShader = compileShader(gl, gl.FRAGMENT_SHADER, fragmentSource);
  const program = gl.createProgram();
  if (!program) throw new Error("Unable to create WebGL program");
  gl.attachShader(program, vertexShader);
  gl.attachShader(program, fragmentShader);
  gl.linkProgram(program);
  gl.deleteShader(vertexShader);
  gl.deleteShader(fragmentShader);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const message =
      gl.getProgramInfoLog(program) || "Unable to link WebGL program";
    gl.deleteProgram(program);
    throw new Error(message);
  }
  return program;
}

function compileShader(
  gl: WebGL2RenderingContext,
  type: number,
  source: string,
) {
  const shader = gl.createShader(type);
  if (!shader) throw new Error("Unable to create WebGL shader");
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const message =
      gl.getShaderInfoLog(shader) || "Unable to compile WebGL shader";
    gl.deleteShader(shader);
    throw new Error(message);
  }
  return shader;
}

function requiredUniform(
  gl: WebGL2RenderingContext,
  program: WebGLProgram,
  name: string,
) {
  const location = gl.getUniformLocation(program, name);
  if (!location) throw new Error(`Missing WebGL uniform ${name}`);
  return location;
}
