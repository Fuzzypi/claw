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

console.log('\n=== Claw Phase 5: AOS Integration + Pipeline Generator ===\n');

check('reader.go exists and contains gate parsing',
  fileContainsAny(path.join(root, 'internal/aos/reader.go'), 'GATES', 'gates', 'Gate'));

check('commands.go contains init command',
  fileContains(path.join(root, 'internal/cli/commands.go'), 'init'));

check('reader_test.go exists and contains tests',
  fileContains(path.join(root, 'internal/aos/reader_test.go'), 'func Test'));

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
