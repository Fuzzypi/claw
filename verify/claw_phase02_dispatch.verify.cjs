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

console.log('\n=== Claw Phase 2: Dispatch Engine + Shell Executor ===\n');

check('dispatcher.go exists and contains dispatch logic',
  fileContainsAny(path.join(root, 'internal/dispatch/dispatcher.go'), 'dispatch', 'Dispatch'));

check('shell.go exists and contains exec logic',
  fileContainsAny(path.join(root, 'internal/dispatch/shell.go'), 'Command', 'exec', 'Exec'));

check('commands.go exists and contains run and status',
  fileContains(path.join(root, 'internal/cli/commands.go'), 'run', 'status'));

check('dispatch_test.go exists and contains tests',
  fileContains(path.join(root, 'internal/dispatch/dispatch_test.go'), 'func Test'));

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

console.log(`\n=== Results: ${passed} passed, ${failed} failed ===\n`);
process.exit(failed > 0 ? 1 : 0);
