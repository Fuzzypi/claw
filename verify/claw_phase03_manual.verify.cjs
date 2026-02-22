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

function fileContainsAny(filePath, ...terms) {
  if (!fs.existsSync(filePath)) return false;
  const content = fs.readFileSync(filePath, 'utf8');
  return terms.some(t => content.includes(t));
}

console.log('\n=== Claw Phase 3: Manual Executor + Agent Management ===\n');

check('manual.go exists and contains manual executor',
  fileContainsAny(path.join(root, 'internal/dispatch/manual.go'), 'manual', 'Manual'));

check('commands.go contains agent and pipeline commands',
  fileContains(path.join(root, 'internal/cli/commands.go'), 'agent', 'pipeline'));

// Run tests
console.log('\n--- Running make test ---\n');
try {
  const result = execSync('make test', {
    cwd: root,
    encoding: 'utf8',
    timeout: 180000,
    stdio: ['pipe', 'pipe', 'pipe']
  });
  console.log(result);
  check('make test exits 0', true);
} catch (e) {
  console.log(e.stdout || '');
  console.log(e.stderr || '');
  check('make test exits 0', false);
}

// Build and check help output
console.log('\n--- Running make build + help ---\n');
try {
  const result = execSync('make build && ./claw --help', {
    cwd: root,
    encoding: 'utf8',
    timeout: 120000,
    stdio: ['pipe', 'pipe', 'pipe']
  });
  console.log(result);
  check('make build + help exits 0 and contains "agent"', result.includes('agent'));
} catch (e) {
  console.log(e.stdout || '');
  console.log(e.stderr || '');
  check('make build + help exits 0 and contains "agent"', false);
}

console.log(`\n=== Results: ${passed} passed, ${failed} failed ===\n`);
process.exit(failed > 0 ? 1 : 0);
