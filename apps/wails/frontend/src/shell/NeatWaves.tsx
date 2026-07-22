import { useEffect, useRef } from "react";

const TARGET_FPS = 24;
const REDUCED_MOTION_FPS = 12;

const vertexShaderSource = `#version 300 es
precision highp float;

in vec2 a_position;
out vec2 v_uv;

void main() {
  v_uv = a_position * 0.5 + 0.5;
  gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

const fragmentShaderSource = `#version 300 es
precision highp float;

in vec2 v_uv;

uniform float u_aspect;
uniform float u_light_mode;
uniform float u_motion;
uniform float u_time;
uniform vec2 u_pointer;

out vec4 outColor;

float blob(vec2 point, vec2 center, vec2 radius) {
  vec2 delta = (point - center) / radius;
  return exp(-dot(delta, delta) * 1.45);
}

void main() {
  float time = u_time * u_motion;
  vec2 point = vec2((v_uv.x - 0.5) * u_aspect, v_uv.y - 0.5);

  vec2 warp = vec2(
    sin(point.y * 3.0 + time * 0.56) + sin(point.x * 1.7 - time * 0.38),
    cos(point.x * 2.5 - time * 0.48) + sin(point.y * 1.9 + time * 0.34)
  ) * 0.075;
  point += warp;

  vec2 warmCenter = vec2(
    -u_aspect * 0.46 + sin(time * 0.46) * 0.30,
    0.48 + cos(time * 0.38) * 0.18
  );
  vec2 pinkCenter = vec2(
    u_aspect * 0.48 + cos(time * 0.39) * 0.32,
    0.34 + sin(time * 0.51) * 0.24
  );
  vec2 blueCenter = vec2(
    -u_aspect * 0.44 + cos(time * 0.34) * 0.28,
    -0.12 + sin(time * 0.44) * 0.24
  );
  vec2 violetCenter = vec2(
    u_aspect * 0.36 + sin(time * 0.37) * 0.30,
    -0.34 + cos(time * 0.47) * 0.20
  );
  vec2 cyanCenter = vec2(
    sin(time * 0.31) * u_aspect * 0.24,
    -0.06 + cos(time * 0.36) * 0.20
  );

  float warmWeight = blob(point, warmCenter, vec2(u_aspect * 0.48, 0.42));
  float pinkWeight = blob(point, pinkCenter, vec2(u_aspect * 0.47, 0.48));
  float blueWeight = blob(point, blueCenter, vec2(u_aspect * 0.48, 0.54));
  float violetWeight = blob(point, violetCenter, vec2(u_aspect * 0.46, 0.52));
  float cyanWeight = blob(point, cyanCenter, vec2(u_aspect * 0.72, 0.72));

  float pointerWeight = blob(
    point,
    vec2((u_pointer.x - 0.5) * u_aspect, u_pointer.y - 0.5),
    vec2(u_aspect * 0.42, 0.48)
  ) * step(0.0, u_pointer.x);
  cyanWeight += pointerWeight * 0.18;

  vec3 lightBase = vec3(0.08, 0.78, 0.84);
  vec3 lightWarm = vec3(1.00, 0.61, 0.03);
  vec3 lightPink = vec3(1.00, 0.12, 0.49);
  vec3 lightBlue = vec3(0.08, 0.30, 1.00);
  vec3 lightViolet = vec3(0.57, 0.10, 0.98);
  vec3 lightCyan = vec3(0.04, 0.82, 0.78);

  vec3 darkBase = vec3(0.015, 0.085, 0.13);
  vec3 darkWarm = vec3(0.62, 0.28, 0.025);
  vec3 darkPink = vec3(0.64, 0.045, 0.33);
  vec3 darkBlue = vec3(0.025, 0.20, 0.62);
  vec3 darkViolet = vec3(0.34, 0.055, 0.62);
  vec3 darkCyan = vec3(0.015, 0.55, 0.53);

  vec3 base = mix(darkBase, lightBase, u_light_mode);
  vec3 warm = mix(darkWarm, lightWarm, u_light_mode);
  vec3 pink = mix(darkPink, lightPink, u_light_mode);
  vec3 blue = mix(darkBlue, lightBlue, u_light_mode);
  vec3 violet = mix(darkViolet, lightViolet, u_light_mode);
  vec3 cyan = mix(darkCyan, lightCyan, u_light_mode);

  float warmMask = smoothstep(0.06, 0.72, warmWeight);
  float pinkMask = smoothstep(0.07, 0.70, pinkWeight);
  float blueMask = smoothstep(0.06, 0.72, blueWeight);
  float violetMask = smoothstep(0.06, 0.70, violetWeight);
  float cyanMask = smoothstep(0.04, 0.74, cyanWeight);

  vec3 color = mix(base, cyan, 0.82 + cyanMask * 0.16);
  color = mix(color, blue, blueMask * 0.88);
  color = mix(color, violet, violetMask * 0.86);
  color = mix(color, warm, warmMask * 0.94);
  color = mix(color, pink, pinkMask * 0.92);

  float height =
    warmWeight * 0.54
    + pinkWeight * 0.62
    + blueWeight * 0.44
    + violetWeight * 0.52
    + cyanWeight * 0.35
    + sin(point.x * 2.0 + point.y * 2.6 + time * 0.18) * 0.08;
  vec3 normal = normalize(vec3(-dFdx(height) * 92.0, -dFdy(height) * 92.0, 1.0));
  vec3 lightDirection = normalize(vec3(-0.42, 0.58, 0.72));
  float diffuse = dot(normal, lightDirection) * 0.5 + 0.5;
  float softShadow = smoothstep(0.12, 0.88, diffuse);
  color *= mix(0.68, 1.16, softShadow);

  float glow = pow(max(normal.z, 0.0), 5.0);
  color += mix(vec3(0.01, 0.10, 0.12), vec3(0.12, 0.10, 0.16), u_light_mode) * glow * 0.16;
  color = mix(color, smoothstep(vec3(0.0), vec3(1.0), color), 0.16);

  outColor = vec4(color, 1.0);
}
`;

const vertices = new Float32Array([-1, -1, 1, -1, -1, 1, -1, 1, 1, -1, 1, 1]);

export function NeatWaves() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const gl = canvas.getContext("webgl2", {
      alpha: false,
      antialias: false,
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
      console.warn("GizClaw NEAT waves background is unavailable", reason);
      return;
    }

    const vertexBuffer = gl.createBuffer();
    if (!vertexBuffer) {
      canvas.dataset.webgl = "failed";
      gl.deleteProgram(program);
      return;
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, vertexBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, vertices, gl.STATIC_DRAW);

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
    const targetPointer = { x: -1, y: -1 };
    const pointer = { x: -1, y: -1 };
    let aspect = 1;
    let animationFrame = 0;
    let lastFrame = 0;

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
    };

    const render = (now: number, reduced = false) => {
      pointer.x += (targetPointer.x - pointer.x) * 0.035;
      pointer.y += (targetPointer.y - pointer.y) * 0.035;
      gl.uniform1f(uniforms.aspect, aspect);
      gl.uniform1f(uniforms.lightMode, lightMode.matches ? 1 : 0);
      gl.uniform1f(uniforms.motion, reduced ? 0.38 : 1);
      gl.uniform2f(uniforms.pointer, pointer.x, pointer.y);
      gl.uniform1f(uniforms.time, now / 1000);
      gl.drawArrays(gl.TRIANGLES, 0, 6);
    };

    const tick = (now: number) => {
      animationFrame = window.requestAnimationFrame(tick);
      if (document.hidden) return;
      const fps = reducedMotion.matches ? REDUCED_MOTION_FPS : TARGET_FPS;
      if (now - lastFrame < 1000 / fps) return;
      lastFrame = now;
      render(now, reducedMotion.matches);
    };

    const movePointer = (event: PointerEvent) => {
      const rect = canvas.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      targetPointer.x = (event.clientX - rect.left) / rect.width;
      targetPointer.y = 1 - (event.clientY - rect.top) / rect.height;
    };
    const clearPointer = () => {
      targetPointer.x = -1;
      targetPointer.y = -1;
    };
    resize();
    render(performance.now(), reducedMotion.matches);
    animationFrame = window.requestAnimationFrame(tick);
    window.addEventListener("resize", resize);
    window.addEventListener("pointermove", movePointer, { passive: true });
    window.addEventListener("pointerleave", clearPointer);

    return () => {
      window.cancelAnimationFrame(animationFrame);
      window.removeEventListener("resize", resize);
      window.removeEventListener("pointermove", movePointer);
      window.removeEventListener("pointerleave", clearPointer);
      gl.deleteBuffer(vertexBuffer);
      gl.deleteProgram(program);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="neat-waves-canvas"
      data-target-fps={TARGET_FPS}
      aria-hidden="true"
    />
  );
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
