import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    (window as any).__GIZCLAW_WINDOW_ACTIONS__ = [];
    window.runtime = {
      WindowHide() {
        (window as any).__GIZCLAW_WINDOW_ACTIONS__.push("hide");
      },
      WindowMinimise() {
        (window as any).__GIZCLAW_WINDOW_ACTIONS__.push("minimise");
      },
      WindowToggleMaximise() {
        (window as any).__GIZCLAW_WINDOW_ACTIONS__.push("maximise");
      },
      BrowserOpenURL(url) {
        (window as any).__GIZCLAW_WINDOW_ACTIONS__.push(`open:${url}`);
      },
    };
    const health = (endpoint: string, state = "reachable") => ({
      endpoint,
      state,
      public_key: `server-public-key-${endpoint}`,
    });
    const pods = [
      {
        id: "local-lab",
        name: "Local Lab",
        description: "Local development",
        mode: "local",
        valid: true,
        play_configured: true,
        play_public_key: "local-play-public-key",
        local: {
          port: 9820,
          lan_addresses: [
            "100.100.100.100:9820",
            "192.168.1.6:9820",
            "192.168.139.3:9820",
            "192.168.147.0:9820",
            "192.168.148.0:9820",
            "192.168.155.0:9820",
            "192.168.156.0:9820",
            "192.168.158.0:9820",
            "192.168.163.0:9820",
            "192.168.194.0:9820",
            "[fd07:b51a:cc66:0:a617:db5e:ab7:e9f1]:9820",
            "[fd07:b51a:cc66:a:ffff:ffff:ffff:fffe]:9820",
            "[fd1f:411f:eafd:458f:1898:35f7:287f:c259]:9820",
          ],
          admin_configured: true,
          admin_public_key: "local-admin-public-key",
          server_public_key: "local-server-public-key",
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
        play_public_key: "remote-play-public-key",
        remote: {
          access_point: health("ap.dev.gizclaw.com:9820"),
          servers: [
            {
              id: "beijing-a",
              name: "Beijing A",
              endpoint: "115.191.6.117:9820",
              admin_configured: true,
              admin_public_key: "beijing-a-admin-public-key",
              health: health("115.191.6.117:9820"),
            },
            {
              id: "beijing-b",
              name: "Beijing B",
              endpoint: "115.191.6.118:9820",
              admin_configured: true,
              admin_public_key: "beijing-b-admin-public-key",
              health: health("115.191.6.118:9820", "unreachable"),
            },
            ...Array.from({ length: 118 }, (_, index) => ({
              id: `server-${index}`,
              name: `Server ${index}`,
              endpoint: `10.0.0.${index + 1}:9820`,
              admin_configured: true,
              admin_public_key: `server-${index}-admin-public-key`,
              health: health(`10.0.0.${index + 1}:9820`),
            })),
          ],
        },
      },
    ];
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return {
          locale: navigator.language.toLowerCase().startsWith("zh")
            ? "zh-CN"
            : "en",
          pods,
        };
      },
      async CreatePod(input) {
        const pod = input.local_server
          ? {
              id: input.id || "pod-generated",
              name: input.name,
              description: input.description,
              mode: "local",
              valid: true,
              play_configured: true,
              play_public_key: "generated-local-play-public-key",
              local: {
                port: input.local_server.port || 9820,
                lan_addresses: ["192.168.1.6:9820"],
                admin_configured: true,
                admin_public_key: "generated-local-admin-public-key",
                server_public_key: "generated-local-server-public-key",
                process: { state: "stopped" },
                health: health("127.0.0.1:9820", "checking"),
              },
            }
          : {
              id: input.id || "pod-generated-remote",
              name: input.name,
              description: input.description,
              mode: "remote",
              valid: true,
              play_configured: true,
              play_public_key: "generated-remote-play-public-key",
              remote: {
                access_point: health(input.remote_access_point, "checking"),
                servers: [],
              },
            };
        pods.push(pod);
        return pod;
      },
      async DeletePod(id) {
        const index = pods.findIndex((pod) => pod.id === id);
        if (index >= 0) pods.splice(index, 1);
      },
      async GetPod(id) {
        return pods.find((pod) => pod.id === id);
      },
      async ListPods() {
        return pods;
      },
      async OpenAdmin() {
        return "http://127.0.0.1:4101/#launch=admin-token";
      },
      async OpenPlay() {
        return "http://127.0.0.1:4102/#launch=play-token";
      },
      async RevealPod() {},
      async RefreshPodHealth(id) {
        return pods.find((pod) => pod.id === id);
      },
      async RestartLocalServer(id) {
        return pods.find((pod) => pod.id === id);
      },
      async StartLocalServer(id) {
        const pod = pods.find((candidate) => candidate.id === id);
        (pod as any).local.process.state = "running";
        return structuredClone(pod);
      },
      async StopLocalServer(id) {
        const pod = pods.find((candidate) => candidate.id === id);
        (pod as any).local.process.state = "stopped";
        return structuredClone(pod);
      },
      async UpdatePod(input) {
        const index = pods.findIndex((pod) => pod.id === input.id);
        const current = pods[index];
        const next = input.remote_access_point
          ? {
              ...current,
              name: input.name,
              description: input.description,
              remote: {
                access_point: health(input.remote_access_point),
                servers: (input.remote_servers || []).map(
                  (server, serverIndex) => {
                    const existing = current.remote?.servers.find(
                      (candidate) => candidate.id === server.id,
                    );
                    const adminConfigured = Boolean(
                      server.admin_private_key || existing?.admin_configured,
                    );
                    return {
                      id: server.id || `server-generated-${serverIndex}`,
                      name: server.name || server.endpoint,
                      endpoint: server.endpoint,
                      admin_configured: adminConfigured,
                      admin_public_key: adminConfigured
                        ? `configured-admin-public-key-${serverIndex}`
                        : undefined,
                      health: health(server.endpoint),
                    };
                  },
                ),
              },
            }
          : {
              ...current,
              name: input.name,
              description: input.description,
            };
        pods[index] = next;
        return next;
      },
    };
  });
});

test("Pod home opens a share face and scalable remote management", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Pods" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Hide window" })).toBeVisible();
  await page.getByRole("button", { name: "Hide window" }).click();
  await expect
    .poll(() => page.evaluate(() => (window as any).__GIZCLAW_WINDOW_ACTIONS__))
    .toEqual(["hide"]);
  await expect(page.getByRole("button", { name: "Refresh" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Local Lab/ })).toBeVisible();
  await page.getByRole("button", { name: /China Development/ }).click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("heading", { level: 2, name: "China Development" }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "Pod actions" }),
  ).toHaveCount(0);
  const remoteQR = page.getByRole("dialog").locator(".qr-code");
  await expect(
    remoteQR.getByRole("img", { name: "Server QR code" }),
  ).toBeVisible();
  const remotePayload = new URL(
    (await remoteQR.getAttribute("data-qr-payload")) ?? "",
  );
  expect(remotePayload.protocol).toBe("gizclaw:");
  expect(remotePayload.host).toBe("ap");
  expect(remotePayload.pathname).toBe("/ap.dev.gizclaw.com:9820");
  expect(remotePayload.searchParams.get("name")).toBe("China Development");
  expect(remotePayload.searchParams.get("mode")).toBe("remote");
  expect(remotePayload.searchParams.get("public_key")).toBe(
    "server-public-key-ap.dev.gizclaw.com:9820",
  );
  await page.getByRole("button", { name: "Manage Servers" }).click();
  const remoteDialog = page.getByRole("dialog");
  await expect(
    remoteDialog
      .locator(".pod-dialog-header")
      .getByRole("button", { name: "Add Server" }),
  ).toHaveCount(0);
  await expect(remoteDialog.locator(".server-add-card")).toHaveText(
    "Add Server",
  );
  await expect(
    remoteDialog.locator(".server-add-card + .virtual-server-list"),
  ).toBeVisible();
  await expect(page.getByText("Beijing A")).toBeVisible();
  await expect(page.getByText("120 servers")).toBeVisible();
  await expect(page.getByText("cn-dev")).toBeVisible();
  await expect
    .poll(() =>
      page.getByRole("dialog").evaluate((element) => element.clientWidth),
    )
    .toBeLessThanOrEqual(420);
  await expect
    .poll(() =>
      page
        .locator(".server-admin-action")
        .first()
        .evaluate((element) => element.clientWidth),
    )
    .toBeLessThanOrEqual(90);
  const remoteCardStyle = await page
    .locator(".server-row")
    .first()
    .evaluate((element) => {
      const style = getComputedStyle(element);
      return {
        radius: Number.parseFloat(style.borderTopLeftRadius),
        background: style.backgroundColor,
      };
    });
  expect(remoteCardStyle.radius).toBeGreaterThanOrEqual(12);
  expect(remoteCardStyle.background).not.toBe("rgba(0, 0, 0, 0)");
  await page.locator(".virtual-server-list").evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.dispatchEvent(new Event("scroll"));
  });
  await expect(page.getByText("Server 117")).toBeVisible();
  await page
    .getByRole("textbox", { name: "Search servers" })
    .fill("server-117");
  await expect(page.getByText("Server 117")).toBeVisible();
  await page.getByRole("textbox", { name: "Search servers" }).fill("Beijing B");
  await expect(page.getByText("Beijing A")).not.toBeVisible();
  await expect(page.getByText("Beijing B")).toBeVisible();
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "Admin" }),
  ).toBeVisible();
});

test("Add Pod creates a local environment without exposing keys", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Add Pod/ }).click();
  await expect(page.getByLabel("Pod ID")).toHaveCount(0);
  await page
    .locator(".create-dialog")
    .getByRole("button", { name: /^Local/ })
    .click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("heading", { level: 2, name: "Local Server" }),
  ).toBeVisible();
  await expect(
    page.getByRole("dialog").getByRole("img", { name: "Server QR code" }),
  ).toBeVisible();
  await expect(page.locator("body")).not.toContainText("private_key");
});

test("local share stays simple and switches to focused controls", async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await page.getByRole("button", { name: /Local Lab/ }).click();
  const dialog = page.getByRole("dialog");
  const qr = dialog.locator(".qr-code");
  const payload = new URL((await qr.getAttribute("data-qr-payload")) ?? "");
  expect(payload.protocol).toBe("gizclaw:");
  expect(payload.host).toBe("ap");
  expect(payload.pathname).toBe("/192.168.1.6:9820");
  expect(payload.searchParams.get("name")).toBe("Local Lab");
  expect(payload.searchParams.get("mode")).toBe("local");
  expect(payload.searchParams.get("public_key")).toBe(
    "local-server-public-key",
  );
  await expect(dialog).not.toContainText("100.100.100.100:9820");
  await expect(dialog).not.toContainText("fd1f:411f");
  await expect(dialog).not.toContainText("local-server-public-key");
  await expect(dialog.getByText("192.168.1.6:9820")).toBeVisible();
  await expect(dialog.locator(".qr-card")).toHaveCount(0);
  await expect(qr).toHaveCSS("box-shadow", "none");
  await expect(dialog.getByRole("button", { name: /Play/ })).toBeVisible();
  await dialog.getByRole("button", { name: /Play/ }).click();
  await expect
    .poll(() => page.evaluate(() => (window as any).__GIZCLAW_WINDOW_ACTIONS__))
    .toContain("open:http://127.0.0.1:4102/#launch=play-token");
  await expect
    .poll(() => dialog.evaluate((element) => element.clientWidth))
    .toBeLessThanOrEqual(420);
  await expect
    .poll(() =>
      dialog
        .locator(".pod-detail-stage")
        .evaluate((element) => element.clientHeight),
    )
    .toBeLessThanOrEqual(340);
  await dialog.getByRole("button", { name: "Server controls" }).click();
  const statusCard = dialog.locator(".local-status-card");
  const adminButton = dialog.getByRole("button", { name: /Admin/ });
  await expect(statusCard).toHaveClass(/manage-list-item/);
  await expect(adminButton).toHaveClass(/manage-list-item/);
  await expect
    .poll(() => statusCard.evaluate((element) => element.tagName))
    .toBe("SECTION");
  await expect
    .poll(() => adminButton.evaluate((element) => element.tagName))
    .toBe("BUTTON");
  const sharedStyle = (selector: string) =>
    dialog.locator(selector).evaluate((element) => {
      const style = getComputedStyle(element);
      const icon = element.querySelector(".manage-list-item-icon");
      const iconStyle = icon ? getComputedStyle(icon) : null;
      return {
        backgroundColor: style.backgroundColor,
        borderRadius: style.borderRadius,
        color: style.color,
        iconHeight: iconStyle?.height,
        iconWidth: iconStyle?.width,
        minHeight: style.minHeight,
        padding: style.padding,
      };
    });
  expect(await sharedStyle(".local-admin-action")).toEqual(
    await sharedStyle(".local-status-card"),
  );
  await page.emulateMedia({ colorScheme: "dark" });
  expect(await sharedStyle(".local-admin-action")).toEqual(
    await sharedStyle(".local-status-card"),
  );
  await expect(statusCard.getByRole("button", { name: "Stop" })).toBeVisible();
  await expect(dialog.locator(".local-power-actions")).toHaveCount(0);
  await statusCard.getByRole("button", { name: "Stop" }).click();
  await expect(
    dialog.locator(".local-status-card").getByRole("button", { name: "Start" }),
  ).toBeVisible();
  await expect(adminButton).toBeVisible();
  const deleteButton = dialog.getByRole("button", { name: "Delete Pod" });
  await expect(deleteButton).toBeVisible();
  await expect(deleteButton).not.toHaveClass(/manage-list-item/);
  await expect
    .poll(async () => {
      const admin = await adminButton.boundingBox();
      const remove = await deleteButton.boundingBox();
      return Boolean(admin && remove && remove.y > admin.y + admin.height);
    })
    .toBe(true);
  await adminButton.click();
  await expect
    .poll(() => page.evaluate(() => (window as any).__GIZCLAW_WINDOW_ACTIONS__))
    .toContain("open:http://127.0.0.1:4101/#launch=admin-token");
  await expect(dialog.getByRole("button", { name: /Play/ })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: /Restart/ })).toHaveCount(0);
  await expect(dialog.getByText("server ready")).toHaveCount(0);
  await expect(dialog.locator(".local-status-card")).toBeVisible();
  await expect
    .poll(() =>
      dialog
        .locator(".pod-detail-stage")
        .evaluate((element) => element.clientHeight),
    )
    .toBeLessThanOrEqual(226);
  await expect
    .poll(() =>
      dialog.evaluate((element) => element.scrollWidth <= element.clientWidth),
    )
    .toBe(true);
});

test("server controls delete a Pod after confirmation", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Local Lab/ }).click();
  const detail = page.getByRole("dialog");
  await detail.getByRole("button", { name: "Server controls" }).click();
  page.once("dialog", async (confirmation) => {
    expect(confirmation.message()).toBe("Delete this Pod and its local data?");
    await confirmation.accept();
  });
  await detail.getByRole("button", { name: "Delete Pod" }).click();
  await expect(detail).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Local Lab/ })).toHaveCount(0);
});

test("clicking the Pod name opens a name-only editor", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Local Lab/ }).click();
  const detail = page.getByRole("dialog");
  await detail.getByRole("button", { name: "Local Lab", exact: true }).click();
  const editor = page.locator(".settings-dialog");
  await expect(editor.getByLabel("Name")).toBeVisible();
  await expect(editor.getByLabel("Description")).toHaveCount(0);
  await expect(editor.getByLabel("Access Point")).toHaveCount(0);
  await editor.getByLabel("Name").fill("Renamed Lab");
  await editor.getByRole("button", { name: "Save configuration" }).click();
  await expect(
    detail.getByRole("heading", { level: 2, name: "Renamed Lab" }),
  ).toBeVisible();
});

test("Remote creation asks only for an access point and adds Servers later", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Add Pod" }).click();
  await page
    .locator(".create-dialog")
    .getByRole("button", { name: /^Remote/ })
    .click();
  await expect(page.getByLabel("Server ID")).toHaveCount(0);
  await page.getByLabel("Access Point").fill("ap.example.com:9820");
  await page.getByRole("button", { name: "Create Pod" }).click();
  const detail = page.getByRole("dialog");
  await expect(
    detail.getByRole("heading", { level: 2, name: "Remote Server" }),
  ).toBeVisible();
  await detail.getByRole("button", { name: "Manage Servers" }).click();
  await expect(
    detail.getByRole("button", { name: "Delete Pod" }),
  ).toBeVisible();
  await detail.getByRole("button", { name: "Add Server" }).click();
  const serverEditor = page.locator(".server-editor-dialog");
  await expect(serverEditor).toHaveAttribute(
    "data-slot",
    "desktop-dialog-content",
  );
  await page.keyboard.press("Escape");
  await expect(serverEditor).not.toBeVisible();
  await expect(detail).toBeVisible();
  await detail.getByRole("button", { name: "Add Server" }).click();
  const adminPrivateKey = page.getByLabel("Admin private key");
  await expect(adminPrivateKey).toHaveAttribute("type", "password");
  await expect(page.getByText("Admin public key")).toHaveCount(0);
  await page.getByLabel("Server Endpoint").fill("server.example.com:9820");
  await adminPrivateKey.fill("server-configured-admin-private-key");
  await page.getByRole("button", { name: "Save configuration" }).click();
  await expect(
    detail.getByText("server.example.com:9820").first(),
  ).toBeVisible();
  await expect(detail.getByRole("button", { name: "Admin" })).toBeVisible();
  await expect(detail.locator(".server-admin-action.configured")).toBeVisible();
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
    .evaluate((element) => getComputedStyle(element).backgroundColor);
  await page.emulateMedia({ colorScheme: "light" });
  const light = await page
    .locator(".desktop-shell")
    .evaluate((element) => getComputedStyle(element).backgroundColor);
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

test("launcher uses rounded transparent framing and ambient card depth", async ({
  page,
}) => {
  await page.goto("/");
  const gridCards = page.locator(".pod-grid > *");
  await expect(gridCards.first()).toHaveClass(/mobile-app-card/);
  await expect(gridCards.first()).toHaveAttribute("data-slot", "home-card");
  await expect(page.locator(".pod-card").first()).toHaveAttribute(
    "data-slot",
    "home-card",
  );
  await expect(gridCards.first()).toContainText("TestFlight");
  await expect(gridCards.first()).toContainText("Google Play");
  await gridCards.first().click();
  const mobileDialog = page.getByRole("dialog", { name: "GizClaw Mobile" });
  await expect(mobileDialog).toBeVisible();
  await expect(mobileDialog).toHaveAttribute(
    "data-slot",
    "desktop-dialog-content",
  );
  await expect(mobileDialog.locator(".qr-code")).toHaveAttribute(
    "data-qr-payload",
    /iOS \/ TestFlight/,
  );
  await mobileDialog.getByRole("button", { name: /Android/ }).click();
  await expect(mobileDialog.locator(".qr-code")).toHaveAttribute(
    "data-qr-payload",
    /Android \/ Google Play Beta/,
  );
  await mobileDialog.getByRole("button", { name: "Close" }).click();
  await expect(mobileDialog).toHaveCount(0);
  const shell = page.locator(".desktop-shell");
  const shellStyle = await shell.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      radius: Number.parseFloat(style.borderTopLeftRadius),
      width: element.getBoundingClientRect().width,
      viewport: window.innerWidth,
      margin: style.margin,
      shadow: style.boxShadow,
    };
  });
  expect(shellStyle.radius).toBeGreaterThanOrEqual(18);
  expect(shellStyle.width).toBe(shellStyle.viewport);
  expect(shellStyle.margin).toBe("0px");
  expect(shellStyle.shadow).toBe("none");
  const homeTitle = page.getByRole("heading", { name: "GizClaw" });
  await expect(homeTitle).toBeVisible();
  const titleLayout = await homeTitle.evaluate((element) => {
    const style = getComputedStyle(element);
    const bounds = element.getBoundingClientRect();
    const card = document.querySelector(".pod-card, .mobile-app-card");
    return {
      bottom: bounds.bottom,
      cardTop: card?.getBoundingClientRect().top ?? 0,
      fontFamily: style.fontFamily,
      fontSize: Number.parseFloat(style.fontSize),
    };
  });
  expect(titleLayout.fontFamily).toContain("Space Grotesk");
  expect(titleLayout.fontSize).toBeGreaterThanOrEqual(40);
  expect(titleLayout.bottom).toBeLessThan(titleLayout.cardTop);
  const subtitle = page.getByText("Your edge constellation", { exact: true });
  await expect(subtitle).toBeVisible();
  const subtitleLayout = await subtitle.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const card = document
      .querySelector(".pod-card, .mobile-app-card")
      ?.getBoundingClientRect();
    return {
      bottom: bounds.bottom,
      cardTop: card?.top ?? 0,
      top: bounds.top,
    };
  });
  expect(subtitleLayout.top).toBeGreaterThan(titleLayout.bottom);
  expect(subtitleLayout.bottom).toBeLessThan(subtitleLayout.cardTop);
  await expect(page.locator(".neat-waves-canvas")).toHaveAttribute(
    "data-target-fps",
    "24",
  );

  const cards = page.locator(".pod-card");
  const firstCard = await cards.first().evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      backdropFilter: style.backdropFilter,
      background: style.backgroundImage,
      hue: style.getPropertyValue("--card-hue"),
      shadow: style.boxShadow,
    };
  });
  const lastCardHue = await cards
    .last()
    .evaluate((element) =>
      getComputedStyle(element).getPropertyValue("--card-hue"),
    );
  expect(firstCard.background).toContain("linear-gradient");
  expect(firstCard.backdropFilter).toContain("blur(20px)");
  expect(firstCard.shadow).not.toBe("none");
  expect(firstCard.hue).not.toBe(lastCardHue);
  await expect(page.locator(".add-pod-card")).toHaveCSS(
    "background-image",
    /linear-gradient/,
  );
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
  await expect(page.getByRole("button", { name: /添加 Pod/ })).toBeVisible();
  await page.getByRole("button", { name: /添加 Pod/ }).click();
  await expect(page.getByRole("heading", { name: "添加 Pod" })).toBeVisible();
});

test("Pod details animate closed instead of navigating away", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Local Lab/ }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("button", { name: /Local Lab/ })).toBeVisible();
});
