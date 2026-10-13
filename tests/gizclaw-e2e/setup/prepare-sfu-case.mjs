import fs from "node:fs";
import { parse } from "yaml";

const [
  source,
  output,
  scenario,
  repeats,
  acceptRevocation,
  terminalDelay = "0",
] = process.argv.slice(2);
const document = parse(fs.readFileSync(source, "utf8"));
document.repeat = Number(repeats);
if (scenario === "startup") {
  document.name = "sfu.startup.burst.audio-bytes";
  const end = document.steps.findIndex(
    (step) => step.id === "bob_speaks_before_removal",
  );
  const join = document.steps.findIndex(
    (step) => step.id === "alice_select_shared_workspace",
  );
  if (end < 0 || join < 0)
    throw new Error("SFU fixture is missing its startup/media boundaries");
  document.steps = document.steps.slice(0, end);
  if (document.repeat > 1) {
    document.steps.splice(join, 0, {
      id: "join_burst",
      barrier: { participants: document.repeat },
      timeout: "60s",
    });
  }
} else if (scenario.startsWith("logging-")) {
  document.name = `sfu.${scenario}.audio-bytes`;
  const control = (id, path) => ({
    id,
    client: "carol",
    timeout: "3s",
    http: { endpoint: "http://server-b:9789", method: "POST", path },
  });
  let boundary;
  if (scenario === "logging-join") {
    boundary = "carol_select_shared_workspace";
    document.steps.find((step) => step.id === boundary).timeout = "1s";
  } else if (scenario === "logging-media") {
    boundary = "alice_speaks_before_removal";
    const media = document.steps.find((step) => step.id === boundary);
    media.expect["/bob_listen_before_removal/first_audio_ms"] = {
      minimum: 1,
      maximum: 1500,
    };
    media.expect["/carol_listen_alice_before_removal/first_audio_ms"] = {
      minimum: 1,
      maximum: 1500,
    };
  } else if (scenario === "logging-stop") {
    boundary = "bob_stop_under_pressure";
    const at = document.steps.findIndex(
      (step) => step.id === "alice_remove_bob",
    );
    document.steps.splice(
      at,
      0,
      {
        id: boundary,
        client: "bob",
        timeout: "1s",
        rpc: { method: "server.run.stop", request: {} },
      },
      control("release_after_stop", "/release"),
      {
        id: "bob_restart_after_pressure",
        client: "bob",
        rpc: {
          method: "server.run.workspace.set",
          request: { workspace_name: "${alice_workspace}" },
        },
      },
    );
  } else throw new Error("unknown logging pressure phase");
  const at = document.steps.findIndex((step) => step.id === boundary);
  const block = control("saturate_system_log_queue", "/block");
  block.expect = { "/queued": { equals: 1024 }, "/blocked": { equals: true } };
  document.steps.splice(at, 0, block);
  // Recovery runs the original complete membership and media contract.
  if (scenario !== "logging-stop") {
    const after =
      scenario === "logging-join"
        ? "carol_select_shared_workspace_running"
        : boundary;
    const index = document.steps.findIndex((step) => step.id === after);
    document.steps.splice(
      index + 1,
      0,
      control("release_system_log_queue", "/release"),
    );
  }
  document.finally.unshift(control("release_logs_before_cleanup", "/release"));
} else if (scenario !== "removal") {
  throw new Error("SFU scenario must be startup or removal");
}
if (
  scenario === "removal" &&
  acceptRevocation !== "0" &&
  Number(terminalDelay) > 0
) {
  const removed = document.steps.find(
    (step) => step.id === "alice_speaks_while_bob_removed",
  );
  if (!removed) throw new Error("SFU fixture is missing its removal window");
  removed.expect["/bob_listen_removed/accepted_terminal_errors"] = {
    equals: 1,
  };
}
if (acceptRevocation === "0") {
  for (const step of document.steps) {
    for (const child of step.parallel ?? []) {
      if (child.peer_stream) delete child.peer_stream.allowed_terminal_error;
    }
  }
}
fs.writeFileSync(
  output,
  "# User Story:\n# As a local SFU operator,\n" +
    "# I want real cross-Server media and membership checks under repeatable bursts,\n" +
    "# So that ICE capacity and delayed revocation events remain observable.\n" +
    JSON.stringify(document, null, 2) +
    "\n",
);
