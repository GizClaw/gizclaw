import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    let selected = "local";
    let selectedView = "admin";
    let session = { active: false };
    const contexts = [
      {
        current: true,
        description: "Local server",
        endpoint: "127.0.0.1:9820",
        local_public_key: "local-public-key",
        name: "local",
        server_public_key: "server-public-key",
      },
      {
        current: false,
        description: "Remote server",
        endpoint: "127.0.0.1:19820",
        local_public_key: "remote-public-key",
        name: "remote",
        server_public_key: "remote-server-public-key",
      },
    ];
    const views = [
      { description: "Manage GizClaw server resources.", id: "admin", title: "Admin" },
      { description: "Use workspaces, chat history, social, and firmware flows.", id: "play", title: "Play" },
    ];
    const runtime = (name: string) => {
      const context = contexts.find((item) => item.name === name) ?? contexts[0];
      return {
        context,
        private_key_base64: "cHJpdmF0ZS1rZXktbWF0ZXJpYWw=",
        signaling_url: `http://${context.endpoint}/webrtc/v1/offer`,
      };
    };
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return {
          contexts,
          state: {
            last_context: selected,
            last_view: selectedView,
          },
          view_session: session,
          views,
        };
      },
      async CreateContext(req) {
        selected = req.name;
        contexts.forEach((item) => {
          item.current = item.name === selected;
        });
        const created = {
          current: true,
          description: req.description ?? "",
          endpoint: req.endpoint,
          local_public_key: "created-public-key",
          name: req.name,
          server_public_key: req.server_public_key,
        };
        contexts.push(created);
        return created;
      },
      async EndViewSession() {
        session = { active: false };
        return session;
      },
      async GetViewSession() {
        return session;
      },
      async InjectedRuntime() {
        return runtime(selected);
      },
      async ListContexts() {
        return contexts;
      },
      async ListViews() {
        return views;
      },
      async SelectContext(name) {
        selected = name;
        contexts.forEach((item) => {
          item.current = item.name === selected;
        });
        return contexts.find((item) => item.name === selected) ?? contexts[0];
      },
      async StartViewSession(req) {
        selected = req.context_name;
        selectedView = req.view;
        session = { active: true, context_name: selected, view: selectedView };
        return session;
      },
    };
  });
});

test("shell opens from welcome and injects runtime only after get started", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByText("GizClaw Desktop")).toBeVisible();
  await expect(page.getByRole("button", { name: /local/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Admin selected/ })).toBeVisible();
  await expect(page.getByText("Runtime Injection")).not.toBeVisible();

  await page.getByRole("button", { name: /remote/i }).click();
  await page.getByRole("button", { name: /Play/ }).click();
  await page.getByRole("button", { name: "Get Started" }).click();

  await expect(page.getByText("Play Console")).toBeVisible();
  await expect(page.getByText("Context: remote")).toBeVisible();
  await expect(page.getByText("http://127.0.0.1:19820/webrtc/v1/offer")).toBeVisible();
  await expect(page.getByText("Injected in memory")).toBeVisible();

  await page.getByRole("button", { name: /Sign out/ }).click();
  await expect(page.getByText("GizClaw Desktop")).toBeVisible();
  await expect(page.getByText("Runtime Injection")).not.toBeVisible();
});
