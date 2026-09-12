import { describe, expect, it } from "vitest";
import { exportConfig, parseConfig } from "@/lib/config";
import { bytes, duration, rates } from "@/lib/format";

const token = `gizclaw_mk_${"x".repeat(32)}`;

describe("console configuration", () => {
  it("accepts an array of nodes and normalizes their origins", () => {
    const config = parseConfig(
      JSON.stringify({
        name: "生产集群",
        servers: [
          {
            id: "server-1",
            name: "Server 主节点",
            role: "server",
            url: "https://server1.example.com/",
            monitorToken: token,
          },
        ],
      }),
    );
    expect(config.name).toBe("生产集群");
    expect(config.servers).toEqual([
      {
        id: "server-1",
        name: "Server 主节点",
        url: "https://server1.example.com",
        role: "server",
        region: undefined,
        monitorToken: token,
      },
    ]);
  });

  it("accepts a map keyed by node address", () => {
    const config = parseConfig(
      JSON.stringify({
        servers: { "https://edge.example.com": { monitorToken: token } },
      }),
    );
    expect(config.servers[0]).toMatchObject({
      id: "https://edge.example.com",
      url: "https://edge.example.com",
      role: "unknown",
    });
  });

  it("keeps plain HTTP for local development only", () => {
    expect(
      parseConfig(
        JSON.stringify({
          servers: [{ url: "http://127.0.0.1:9820", monitorToken: token }],
        }),
      ).servers[0].url,
    ).toBe("http://127.0.0.1:9820");
    expect(() =>
      parseConfig(
        JSON.stringify({
          servers: [{ url: "http://node.example.com", monitorToken: token }],
        }),
      ),
    ).toThrow(/HTTPS/);
  });

  it("names the field to fix instead of failing silently", () => {
    expect(() => parseConfig("not json")).toThrow(/合法 JSON/);
    expect(() => parseConfig(JSON.stringify({ servers: [] }))).toThrow(
      /servers/,
    );
    expect(() =>
      parseConfig(
        JSON.stringify({
          servers: [{ url: "https://a.example.com", monitorToken: "nope" }],
        }),
      ),
    ).toThrow(/gizclaw_mk_/);
    expect(() =>
      parseConfig(
        JSON.stringify({
          servers: [
            { id: "a", url: "https://a.example.com", monitorToken: token },
            { id: "a", url: "https://b.example.com", monitorToken: token },
          ],
        }),
      ),
    ).toThrow(/重复/);
  });
});

describe("assistant configuration", () => {
  const apiKey = `gizclaw_sk_v1_${"k-_9".repeat(10)}abc`;
  const base = {
    servers: [{ url: "https://a.example.com", monitorToken: token }],
  };

  it("is optional and defaults the model alias", () => {
    expect(parseConfig(JSON.stringify(base)).assistant).toBeUndefined();
    const config = parseConfig(
      JSON.stringify({ ...base, assistant: { apiKey } }),
    );
    expect(config.assistant).toEqual({
      apiKey,
      model: "llm",
      endpoint: undefined,
    });
  });

  it("accepts a model alias and a separate /openai/v1 endpoint", () => {
    const config = parseConfig(
      JSON.stringify({
        ...base,
        assistant: {
          apiKey,
          model: "chat",
          endpoint: "https://edge.example.com/",
        },
      }),
    );
    expect(config.assistant).toEqual({
      apiKey,
      model: "chat",
      endpoint: "https://edge.example.com",
    });
  });

  it("rejects anything but a device API key", () => {
    for (const assistant of [
      { apiKey: token },
      { apiKey: "gizclaw_sk_v1_short" },
      { apiKey: `${apiKey}x` },
      { apiKey: apiKey.slice(0, -1) },
      { apiKey: `${apiKey.slice(0, -1)}=` },
      { apiKey, unknown: true },
      { apiKey, endpoint: "http://edge.example.com" },
      { apiKey, contextTokens: 7_999 },
      { apiKey, contextTokens: 12_000.5 },
    ]) {
      expect(() =>
        parseConfig(JSON.stringify({ ...base, assistant })),
      ).toThrow();
    }
  });

  it("round-trips through export", () => {
    const config = parseConfig(
      JSON.stringify({
        ...base,
        assistant: { apiKey, model: "chat", contextTokens: 128_000 },
      }),
    );
    expect(config.assistant?.contextTokens).toBe(128_000);
    expect(parseConfig(exportConfig(config, [])).assistant).toEqual(
      config.assistant,
    );
  });
});

describe("measurement formatting", () => {
  it("derives rates from cumulative counters and resets after a restart", () => {
    expect(
      rates({ time: 1000, rx: 100, tx: 200 }, { time: 3000, rx: 300, tx: 800 }),
    ).toMatchObject({ rx: 100, tx: 300 });
    expect(
      rates({ time: 1000, rx: 100, tx: 200 }, { time: 2000, rx: 1, tx: 2 }),
    ).toMatchObject({
      rx: 0,
      tx: 0,
    });
    expect(rates(undefined, { time: 1000, rx: 100, tx: 200 })).toMatchObject({
      rx: 0,
      tx: 0,
    });
  });

  it("keeps unavailable values explicitly empty", () => {
    expect(bytes(Number.NaN)).toBe("—");
    expect(duration(-1)).toBe("—");
    expect(bytes(2048)).toBe("2.0 KiB");
    expect(duration(3720)).toBe("1 小时 2 分");
  });
});

describe("configuration export", () => {
  it("round-trips nodes and the personal device list", () => {
    const config = parseConfig(
      JSON.stringify({
        name: "生产集群",
        servers: [
          {
            id: "edge-1",
            name: "Edge 华东",
            role: "edge",
            url: "https://edge1.example.com",
            monitorToken: token,
          },
        ],
      }),
    );
    const text = exportConfig(config, [
      { publicKey: "abc", label: "工位机", endpoint: "https://ap.gizclaw.com" },
      { publicKey: "def", label: "" },
    ]);
    const reloaded = parseConfig(text);
    expect(reloaded.servers).toEqual(config.servers);
    expect(reloaded.name).toBe("生产集群");
    expect(reloaded.peers).toEqual([
      { publicKey: "abc", label: "工位机", endpoint: "https://ap.gizclaw.com" },
      { publicKey: "def", label: "", endpoint: undefined },
    ]);
  });
});
