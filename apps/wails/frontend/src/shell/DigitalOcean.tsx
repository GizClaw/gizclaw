import { useEffect, useRef } from "react";

const TARGET_FPS = 24;
const BACKGROUND_FPS = 6;

const vertexShaderSource = `#version 300 es
precision highp float;

in vec2 a_position;

uniform float u_aspect;
uniform float u_motion;
uniform float u_time;
uniform vec2 u_pointer;

out vec3 v_world;
out vec2 v_uv;
out float v_height;

float oceanHeight(vec2 point) {
  float waveA = sin(dot(point, normalize(vec2(0.86, 0.50))) * 0.72 + u_time * 0.34) * 0.46;
  float waveB = sin(dot(point, normalize(vec2(-0.38, 0.92))) * 1.08 - u_time * 0.25) * 0.24;
  float waveC = cos(dot(point, normalize(vec2(0.68, -0.74))) * 1.55 + u_time * 0.18) * 0.11;
  vec2 pointerDelta = point - u_pointer;
  float pointerRipple = exp(-dot(pointerDelta, pointerDelta) * 0.12)
    * sin(length(pointerDelta) * 2.8 - u_time * 0.7)
    * 0.08;
  return (waveA + waveB + waveC + pointerRipple) * u_motion;
}

void main() {
  float depth = a_position.y;
  float nearFactor = pow(depth, 1.45);
  vec2 worldPoint = vec2(
    (a_position.x - 0.5) * 17.0 * u_aspect,
    mix(18.0, 0.8, depth)
  );
  float height = oceanHeight(worldPoint);
  float width = mix(1.04, 1.24, nearFactor);
  float projectedX = (a_position.x * 2.0 - 1.0) * width;
  float projectedY = mix(0.18, -1.18, nearFactor)
    + height * mix(0.025, 0.20, nearFactor);

  v_world = vec3(worldPoint.x, height, worldPoint.y);
  v_uv = a_position;
  v_height = height;
  gl_Position = vec4(projectedX, projectedY, 0.0, 1.0);
}
`;

const fragmentShaderSource = `#version 300 es
precision highp float;

in vec3 v_world;
in vec2 v_uv;
in float v_height;

uniform float u_light_mode;

out vec4 outColor;

float gridLine(float coordinate) {
  float distanceToLine = min(fract(coordinate), 1.0 - fract(coordinate));
  float width = max(fwidth(coordinate) * 1.1, 0.004);
  return 1.0 - smoothstep(0.0, width, distanceToLine);
}

void main() {
  vec3 dx = dFdx(v_world);
  vec3 dy = dFdy(v_world);
  vec3 normal = normalize(cross(dx, dy));
  if (normal.y < 0.0) normal = -normal;

  vec3 lightDirection = normalize(vec3(-0.32, 0.88, 0.36));
  float diffuse = max(dot(normal, lightDirection), 0.0);
  float fresnel = pow(1.0 - clamp(normal.y, 0.0, 1.0), 2.15);
  float crest = smoothstep(0.18, 0.68, v_height);

  vec3 darkDeep = vec3(0.012, 0.040, 0.052);
  vec3 darkSurface = vec3(0.020, 0.105, 0.128);
  vec3 lightDeep = vec3(0.795, 0.865, 0.870);
  vec3 lightSurface = vec3(0.720, 0.895, 0.895);
  vec3 base = mix(darkDeep, lightDeep, u_light_mode);
  vec3 surface = mix(darkSurface, lightSurface, u_light_mode);
  vec3 cyan = mix(vec3(0.020, 0.760, 0.680), vec3(0.010, 0.540, 0.520), u_light_mode);

  float depthLift = mix(0.18, 0.82, smoothstep(0.0, 0.92, v_uv.y));
  vec3 color = mix(base, surface, depthLift * (0.58 + diffuse * 0.28));
  color += cyan * (crest * 0.17 + fresnel * 0.10) * mix(1.0, 0.62, u_light_mode);

  float grid = max(gridLine(v_world.x * 0.28), gridLine(v_world.z * 0.34));
  float gridFade = smoothstep(0.04, 0.28, v_uv.y) * (1.0 - smoothstep(0.84, 1.0, v_uv.y));
  color += cyan * grid * gridFade * mix(0.065, 0.040, u_light_mode);

  float horizonGlow = (1.0 - smoothstep(0.0, 0.24, v_uv.y));
  color += cyan * horizonGlow * 0.035;
  float vignette = 1.0 - smoothstep(0.18, 0.90, distance(v_uv, vec2(0.52, 0.46)));
  color *= mix(0.78, 1.0, vignette);

  outColor = vec4(color, 1.0);
}
`;

export function DigitalOcean() {
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
      console.warn("GizClaw digital ocean background is unavailable", reason);
      return;
    }

    const mesh = createMesh(64, 42);
    const vertexBuffer = gl.createBuffer();
    if (!vertexBuffer) {
      canvas.dataset.webgl = "failed";
      gl.deleteProgram(program);
      return;
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, vertexBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, mesh.vertices, gl.STATIC_DRAW);

    const positionLocation = gl.getAttribLocation(program, "a_position");
    if (positionLocation < 0) {
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
    const targetPointer = { x: -100, y: -100 };
    const pointer = { x: -100, y: -100 };
    let aspect = 1;
    let animationFrame = 0;
    let lastFrame = 0;
    let staticFrameRendered = false;

    gl.useProgram(program);
    gl.enableVertexAttribArray(positionLocation);
    gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, 8, 0);
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
      pointer.x += (targetPointer.x - pointer.x) * 0.045;
      pointer.y += (targetPointer.y - pointer.y) * 0.045;
      gl.clearColor(
        lightMode.matches ? 0.875 : 0.008,
        lightMode.matches ? 0.900 : 0.030,
        lightMode.matches ? 0.915 : 0.044,
        1,
      );
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.uniform1f(uniforms.aspect, aspect);
      gl.uniform1f(uniforms.lightMode, lightMode.matches ? 1 : 0);
      gl.uniform1f(uniforms.motion, staticFrame ? 0.74 : 1);
      gl.uniform2f(uniforms.pointer, pointer.x, pointer.y);
      gl.uniform1f(uniforms.time, staticFrame ? 2.1 : now / 1000);
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
      const x = (event.clientX - rect.left) / rect.width;
      const y = (event.clientY - rect.top) / rect.height;
      targetPointer.x = (x - 0.5) * 17 * aspect;
      targetPointer.y = 18 - y * 17.2;
    };
    const clearPointer = () => {
      targetPointer.x = -100;
      targetPointer.y = -100;
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
      className="ocean-canvas"
      data-target-fps={TARGET_FPS}
      aria-hidden="true"
    />
  );
}

function createMesh(columns: number, rows: number) {
  const vertexCount = (columns - 1) * (rows - 1) * 6;
  const vertices = new Float32Array(vertexCount * 2);
  let offset = 0;
  const point = (column: number, row: number) => [
    column / (columns - 1),
    row / (rows - 1),
  ];
  const writeTriangle = (points: number[][]) => {
    for (const [x, y] of points) {
      vertices[offset] = x;
      vertices[offset + 1] = y;
      offset += 2;
    }
  };
  for (let row = 0; row < rows - 1; row += 1) {
    for (let column = 0; column < columns - 1; column += 1) {
      const topLeft = point(column, row);
      const topRight = point(column + 1, row);
      const bottomLeft = point(column, row + 1);
      const bottomRight = point(column + 1, row + 1);
      writeTriangle([topLeft, bottomLeft, bottomRight]);
      writeTriangle([topLeft, bottomRight, topRight]);
    }
  }
  return { vertexCount, vertices };
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
