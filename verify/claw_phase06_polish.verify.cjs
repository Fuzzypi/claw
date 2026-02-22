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

console.log('\n=== Claw Phase 6: CLI Polish + README ===\n');

check('commands.go contains log command',
  fileContains(path.join(root, 'internal/cli/commands.go'), 'log'));

check('README.md exists and length > 500 chars',
  (() => {
    const p = path.join(root, 'README.md');
    return fs.existsSync(p) && fs.readFileSync(p, 'utf8').length > 500;
  })());

check('README.md contains install instructions',
  fileContains(path.join(root, 'README.md'), 'install') ||
  fileContains(path.join(root, 'README.md'), 'Install'));

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

// Build check
console.log('\n--- Running make build ---\n');
try {
  execSync('make build', {
    cwd: root,
    encoding: 'utf8',
    timeout: 120000,
    stdio: ['pipe', 'pipe', 'pipe']
  });
  check('make build exits 0', true);
} catch (e) {
  console.log(e.stdout || '');
  console.log(e.stderr || '');
  check('make build exits 0', false);
}

console.log(`\n=== Results: ${passed} passed, ${failed} failed ===\n`);
process.exit(failed > 0 ? 1 : 0);
