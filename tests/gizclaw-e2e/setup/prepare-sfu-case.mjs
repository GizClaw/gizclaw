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
