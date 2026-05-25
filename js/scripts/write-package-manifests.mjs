import { writeFile } from "node:fs/promises";

const packages = [
  "adminservice",
  "gearservice",
  "serverpublic",
];

for (const name of packages) {
  const manifest = {
    name: `@gizclaw/${name}`,
    private: true,
    type: "module",
    exports: {
      ".": "./index.ts",
      "./client.gen": "./client.gen.ts",
    },
  };
  await writeFile(
    new URL(`../packages/${name}/package.json`, import.meta.url),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
}
