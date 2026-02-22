const fs = require("fs");
const path = require("path");

const ROOT = path.resolve(__dirname, "..");

const requiredPaths = [
  { p: ".aos/CHARTER.md", type: "file" },
  { p: ".aos/LAWS.md", type: "file" },
  { p: ".aos/GATES.md", type: "file" },
  { p: ".aos/PROMPTS/ARCHITECT.md", type: "file" },
  { p: ".aos/PROMPTS/BUILDER.md", type: "file" },
  { p: ".aos/PROMPTS/VERIFIER.md", type: "file" },
  { p: "governance/laws/LAWSET.md", type: "file" },
  { p: "docs/blueprint/ARCHITECTURE_BLUEPRINT.md", type: "file" },
  { p: "docs/plan/PHASE_PLAN.md", type: "file" },
  { p: "verify/repo.verify.cjs", type: "file" },
  { p: "verify/blueprint.verify.cjs", type: "file" },
  { p: "package.json", type: "file" },
  { p: "tsconfig.json", type: "file" },
  { p: "CLAUDE.md", type: "file" },
  { p: ".aos", type: "dir" },
  { p: "docs/blueprint", type: "dir" },
  { p: "docs/plan", type: "dir" },
  { p: "src", type: "dir" },
  { p: "verify", type: "dir" },
  { p: "governance", type: "dir" },
];

let failed = false;

for (const entry of requiredPaths) {
  const full = path.join(ROOT, entry.p);
  const exists = fs.existsSync(full);
  if (!exists) {
    console.error("FAIL  missing: " + entry.p);
    failed = true;
    continue;
  }
  const stat = fs.statSync(full);
  if (entry.type === "dir" && !stat.isDirectory()) {
    console.error("FAIL  expected directory: " + entry.p);
    failed = true;
  } else if (entry.type === "file" && !stat.isFile()) {
    console.error("FAIL  expected file: " + entry.p);
    failed = true;
  } else {
    console.log("PASS  " + entry.p);
  }
}

if (failed) {
  process.exit(1);
}

console.log("\n✓ repo structure gate passed");
