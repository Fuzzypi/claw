const fs = require("fs");
const path = require("path");

const ROOT = path.resolve(__dirname, "..");

let failed = false;

const blueprintPath = path.join(ROOT, "docs/blueprint/ARCHITECTURE_BLUEPRINT.md");
if (!fs.existsSync(blueprintPath)) {
  console.error("FAIL  missing: docs/blueprint/ARCHITECTURE_BLUEPRINT.md");
  failed = true;
} else {
  console.log("PASS  docs/blueprint/ARCHITECTURE_BLUEPRINT.md exists");

  const content = fs.readFileSync(blueprintPath, "utf-8");

  if (content.length <= 50) {
    console.error("FAIL  blueprint is too short (must be > 50 characters)");
    failed = true;
  } else {
    console.log("PASS  blueprint has content (> 50 chars)");
  }

  if (!content.match(/^#/m)) {
    console.error("FAIL  blueprint has no markdown heading");
    failed = true;
  } else {
    console.log("PASS  blueprint contains markdown heading");
  }
}

const referenceFiles = ["CLAUDE.md", "README.md"];
let referenced = false;

for (const ref of referenceFiles) {
  const full = path.join(ROOT, ref);
  if (fs.existsSync(full)) {
    const refContent = fs.readFileSync(full, "utf-8");
    if (refContent.includes("ARCHITECTURE_BLUEPRINT")) {
      referenced = true;
      console.log("PASS  blueprint referenced in " + ref);
      break;
    }
  }
}

if (!referenced) {
  console.error("FAIL  blueprint not referenced in CLAUDE.md or README.md");
  failed = true;
}

if (failed) {
  process.exit(1);
} else {
  console.log("\n✓ blueprint gate passed");
}
