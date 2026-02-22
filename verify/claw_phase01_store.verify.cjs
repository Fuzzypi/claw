const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
let passed = 0;
let failed = 0;

function check(label, condition) {
  if (condition) {
    console.log(`  ✓ ${label}`);
    passed++;
  } else {
    console.log(`  ✗ ${label}`);
    failed++;
  }
}

function fileContains(filePath, ...terms) {
  if (!fs.existsSync(filePath)) return false;
  const content = fs.readFileSync(filePath, 'utf8');
  return terms.every(t => content.includes(t));
}

console.log('\n=== Claw Phase 1: Store + Job Model ===\n');

// File existence and content checks
check('go.mod exists and contains module name',
  fileContains(path.join(root, 'go.mod'), 'github.com/fuzzypi/claw'));

check('db.go exists and contains "jobs"',
  fileContains(path.join(root, 'internal/store/db.go'), 'jobs'));

check('jobs.go exists and contains "Create" and "Status"',
  fileContains(path.join(root, 'internal/store/jobs.go'), 'Create', 'Status'));

check('pipelines.go exists and contains "Pipeline"',
  fileContains(path.join(root, 'internal/store/pipelines.go'), 'Pipeline'));

check('agents.go exists and contains "Register"',
  fileContains(path.join(root, 'internal/store/agents.go'), 'Register'));

check('context.go exists and contains context operations',
  fileContains(path.join(root, 'internal/store/context.go'), 'Context'));

check('dependencies.go exists and contains "depends"',
  fileContains(path.join(root, 'internal/store/dependencies.go'), 'depends'));

check('store_test.go exists and contains "func Test"',
  fileContains(path.join(root, 'internal/store/store_test.go'), 'func Test'));

check('Makefile exists and contains "test"',
  fileContains(path.join(root, 'Makefile'), 'test'));

// Run tests
console.log('\n--- Running make test ---\n');
try {
  const result = execSync('make test', {
    cwd: root,
    encoding: 'utf8',
    timeout: 120000,
    stdio: ['pipe', 'pipe', 'pipe']
  });
  console.log(result);
  check('make test exits 0', true);
} catch (e) {
  console.log(e.stdout || '');
  console.log(e.stderr || '');
  check('make test exits 0', false);
}

console.log(`\n=== Results: ${passed} passed, ${failed} failed ===\n`);
process.exit(failed > 0 ? 1 : 0);
