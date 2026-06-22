const fs = require('fs');
const lines = fs.readFileSync('frontend/src/components/Fleet.tsx', 'utf8').split('\n');
const start = parseInt(process.argv[2]) || 0;
const end = parseInt(process.argv[3]) || 100;
lines.slice(start, end).forEach((l, i) => console.log((start + i + 1) + ': ' + l));
