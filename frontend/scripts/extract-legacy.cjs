// Extract legacy Chat/ files from git blob, then format with prettier,
// preserving exact UTF-8 bytes (no BOM insertion).
const fs = require('fs');
const path = require('path');
const cp = require('child_process');

const FILES = [
  'frontend/src/views/Chat/Chat.tsx',
  'frontend/src/views/Chat/context.tsx',
  'frontend/src/views/Chat/conversation.tsx',
  'frontend/src/views/Chat/message.tsx',
  'frontend/src/views/Chat/pickers.tsx',
  'frontend/src/views/Chat/useChatSession.ts',
  'frontend/src/views/Chat/useComposer.ts',
  'frontend/src/views/Chat/useContextWindow.ts',
  'frontend/src/views/Chat/useConversationControls.ts',
  'frontend/src/views/Chat/useConversationRouting.ts',
  'frontend/src/views/Chat/useSteering.ts',
  'frontend/src/views/Chat/useVoice.ts',
];
const REV = '1d1136f6';
const PRETTIER = (process.env.PRETTIER_BIN ||
  require('os').homedir() + '\\AppData\\Local\\Temp\\prettier-tool\\node_modules\\.bin\\prettier.cmd');

const outDir = path.join(__dirname, '..', 'src', 'features', 'chat', 'legacy');
fs.mkdirSync(outDir, { recursive: true });

for (const f of FILES) {
  const name = path.basename(f);
  // Extract the raw blob bytes (no BOM, no encoding munging)
  const blob = cp.execSync(`git show ${REV}:${f}`, { encoding: 'buffer' });
  const fp = path.join(outDir, name);
  fs.writeFileSync(fp, blob);
  // Now format in place
  const r = cp.spawnSync(PRETTIER, [fp, '--parser', 'babel-ts', '--print-width', '100'], {
    encoding: 'utf8',
    shell: true,
  });
  if (r.status !== 0) {
    console.error('PRETTIER FAILED for', name, '\n', (r.stderr || '').slice(-400));
    process.exit(1);
  }
  fs.writeFileSync(fp, r.stdout, 'utf8');
  const lines = r.stdout.split('\n').length;
  console.log('OK', name, r.stdout.length, 'bytes', lines, 'lines');
}
