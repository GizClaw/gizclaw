import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const health = (endpoint: string, state = "reachable") => ({
      endpoint,
      state,
    });
    const pods = [
      {
        id: "local-lab",
        name: "Local Lab",
        description: "Local development",
        mode: "local",
        valid: true,
        play_configured: true,
        local: {
          port: 9820,
          lan_addresses: ["192.168.1.20:9820"],
          admin_configured: true,
          process: { state: "running", logs: ["server ready"] },
          health: health("127.0.0.1:9820"),
        },
      },
      {
        id: "broken",
        name: "broken",
        mode: "invalid",
        valid: false,
        error: "pod.json is malformed",
        play_configured: false,
      },
      {
        id: "cn-dev",
        name: "China Development",
        description: "Remote mesh",
        mode: "remote",
        valid: true,
        play_configured: true,
        remote: {
          access_point: health("ap.dev.gizclaw.com:9820"),
          servers: [
            {
              id: "beijing-a",
              name: "Beijing A",
              endpoint: "115.191.6.117:9820",
              admin_configured: true,
              health: health("115.191.6.117:9820"),
            },
            {
              id: "beijing-b",
              name: "Beijing B",
              endpoint: "115.191.6.118:9820",
              admin_configured: false,
              health: health("115.191.6.118:9820", "unreachable"),
            },
            ...Array.from({ length: 118 }, (_, index) => ({
              id: `server-${index}`,
              name: `Server ${index}`,
              endpoint: `10.0.0.${index + 1}:9820`,
              admin_configured: index % 2 === 0,
              health: health(`10.0.0.${index + 1}:9820`),
            })),
          ],
        },
      },
    ];
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return { pods };
      },
      async CreatePod(input) {
        const pod = {
          id: input.id,
          name: input.name,
          description: input.description,
          mode: "local",
          valid: true,
          play_configured: false,
          local: {
            port: input.local_server?.port || 9820,
            lan_addresses: [],
            admin_configured: false,
            process: { state: "stopped" },
            health: health("127.0.0.1:9820", "checking"),
          },
        };
        pods.push(pod);
        return pod;
      },
      async DeletePod() {},
      async GetPod(id) {
        return pods.find((pod) => pod.id === id);
      },
      async ListPods() {
        return pods;
      },
      async OpenAdmin() {},
      async OpenPlay() {},
      async RevealPod() {},
      async RefreshPodHealth(id) {
        return pods.find((pod) => pod.id === id);
      },
      async RestartLocalServer(id) {
        return pods.find((pod) => pod.id === id);
      },
      async StartLocalServer(id) {
        return pods.find((pod) => pod.id === id);
      },
      async StopLocalServer(id) {
        return pods.find((pod) => pod.id === id);
      },
      async UpdatePod(input) {
        return pods.find((pod) => pod.id === input.id);
      },
    };
  });
});

test("Pod home opens cards and a scalable remote detail", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Pods" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Local Lab/ })).toBeVisible();
  await page.getByRole("button", { name: /China Development/ }).click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("heading", { name: "China Development" }),
  ).toBeVisible();
  await expect(page.getByText("Beijing A")).toBeVisible();
  await expect(page.getByText("120 servers")).toBeVisible();
  await page
    .getByRole("textbox", { name: "Search servers" })
    .fill("server-117");
  await expect(page.getByText("Server 117")).toBeVisible();
  await page.getByRole("textbox", { name: "Search servers" }).fill("Beijing B");
  await expect(page.getByText("Beijing A")).not.toBeVisible();
  await expect(page.getByText("Beijing B")).toBeVisible();
  await page
    .getByRole("combobox", { name: "Filter by Admin configuration" })
    .selectOption("configured");
  await expect(page.getByText("No Servers match")).toBeVisible();
});

test("Add Pod creates a local environment without exposing keys", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Add Pod/ }).click();
  await page.getByLabel("Pod ID").fill("new-lab");
  await page.getByLabel("Name").fill("New Lab");
  await page.getByRole("button", { name: "Create Pod" }).click();
  await expect(
    page.getByRole("dialog").getByRole("heading", { name: "New Lab" }),
  ).toBeVisible();
  await expect(page.locator("body")).not.toContainText("private_key");
});

test("malformed Pods remain visible and recoverable", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /broken/ }).click();
  await expect(
    page.getByRole("dialog").getByText("pod.json is malformed"),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Reveal in file manager" }),
  ).toBeVisible();
});

test("launcher follows system appearance and reduced motion", async ({
  page,
}) => {
  await page.emulateMedia({
    colorScheme: "dark",
    reducedMotion: "no-preference",
  });
  await page.goto("/");
  const dark = await page
    .locator(".desktop-shell")
    .evaluate((element) => getComputedStyle(element).backgroundImage);
  await page.emulateMedia({ colorScheme: "light" });
  const light = await page
    .locator(".desktop-shell")
    .evaluate((element) => getComputedStyle(element).backgroundImage);
  expect(light).not.toBe(dark);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const duration = await page
    .locator(".pod-card")
    .first()
    .evaluate((element) =>
      Number.parseFloat(getComputedStyle(element).animationDuration),
    );
  expect(duration).toBeLessThan(0.001);
});

test("launcher selects zh-CN from the operating-system locale", async ({
  page,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "zh-CN",
    }),
  );
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Pod 环境" })).toBeVisible();
  await expect(page.getByRole("button", { name: /添加 Pod/ })).toBeVisible();
});
