// Reformat all legacy .tsx/.ts files in features/chat/legacy/ via prettier,
// then write back via raw bytes (Node UTF-8) so PowerShell doesn't mangle
// the encoding.
const fs = require('fs');
const path = require('path');
const cp = require('child_process');

const PRETTIER = process.env.PRETTIER_BIN || require('os').homedir() + '\\AppData\\Local\\Temp\\prettier-tool\\node_modules\\.bin\\prettier.cmd';
const dir = path.join(__dirname, '..', 'src', 'features', 'chat', 'legacy');
const files = fs.readdirSync(dir).filter((f) => /\.(tsx|ts)$/.test(f));

for (const f of files) {
  const fp = path.join(dir, f);
  const r = cp.spawnSync(PRETTIER, [fp, '--parser', 'babel-ts', '--print-width', '100'], { encoding: 'utf8', shell: true });
  if (r.status !== 0) {
    console.error('PRETTIER FAILED for', f, (r.stderr || '').slice(-500));
    process.exit(1);
  }
  fs.writeFileSync(fp, r.stdout, 'utf8');
  console.log('OK', f, r.stdout.length, 'bytes');
}
