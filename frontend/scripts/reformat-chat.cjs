// Split a minified-on-one-line .tsx into readable form by inserting newlines
// after statement boundaries (;, {, }, before return, after =>).
// Preserves strings, regexes, comments, and template literals.
const fs = require('fs');
const path = process.argv[2];
const src = fs.readFileSync(path, 'utf8');

// We tokenize by walking the string and tracking string/comment state.
const out = [];
let depth = 0; // brace depth
let parenDepth = 0;
let i = 0;
let inString = null; // ', ", `, or null
let inComment = null; // //, /*, or null

const isWS = (c) => c === ' ' || c === '\t' || c === '\n' || c === '\r';

function pushNewline() {
  // collapse any existing trailing whitespace before adding \n
  while (out.length && isWS(out[out.length - 1])) out.pop();
  out.push('\n');
  out.push('  '.repeat(Math.max(0, depth - 1)));
}

while (i < src.length) {
  const c = src[i];
  const next = src[i + 1];

  // Comments
  if (!inString && c === '/' && next === '/') {
    inComment = '//';
    out.push(c);
    i++;
    continue;
  }
  if (inComment === '//') {
    out.push(c);
    if (c === '\n') inComment = null;
    i++;
    continue;
  }
  if (!inString && !inComment && c === '/' && next === '*') {
    inComment = '/*';
    out.push(c); out.push(next); i += 2;
    continue;
  }
  if (inComment === '/*') {
    out.push(c);
    if (c === '*' && next === '/') { out.push(next); i += 2; inComment = null; continue; }
    i++;
    continue;
  }

  // Strings
  if (!inString && (c === '"' || c === "'" || c === '`')) {
    inString = c;
    out.push(c); i++;
    continue;
  }
  if (inString === '`') {
    out.push(c);
    if (c === '\\') { out.push(next); i += 2; continue; }
    if (c === '`') { inString = null; i++; continue; }
    // Template substitution ${...}
    if (c === '$' && next === '{') {
      out.push(next); i += 2;
      // skip until matching } respecting nested braces/strings
      let subDepth = 1;
      while (i < src.length && subDepth > 0) {
        const cc = src[i];
        out.push(cc);
        if (cc === '{') subDepth++;
        else if (cc === '}') subDepth--;
        else if (cc === '"' || cc === "'" || cc === '`') {
          const q = cc; out.push(cc); i++;
          while (i < src.length && src[i] !== q) {
            if (src[i] === '\\') { out.push(src[i]); out.push(src[i+1]); i += 2; continue; }
            out.push(src[i]); i++;
          }
          if (i < src.length) { out.push(src[i]); i++; }
          continue;
        }
        i++;
      }
      continue;
    }
    i++;
    continue;
  }
  if (inString === '"' || inString === "'") {
    out.push(c);
    if (c === '\\') { out.push(next); i += 2; continue; }
    if (c === inString) { inString = null; }
    i++;
    continue;
  }

  // Structural
  if (c === '{') {
    depth++;
    out.push(c);
    i++;
    // collapse trailing whitespace, ensure newline + indent follows
    pushNewline();
    continue;
  }
  if (c === '}') {
    depth--;
    // pop current indent if any
    while (out.length && (out[out.length - 1] === ' ' || out[out.length - 1] === '\t')) out.pop();
    out.push('\n');
    out.push('  '.repeat(Math.max(0, depth - 1)));
    out.push(c);
    i++;
    // after }, if next char is not end of stmt/closing/operator, add newline
    if (i < src.length) {
      const nx = src[i];
      if (nx !== ';' && nx !== ',' && nx !== ')' && nx !== '}' && nx !== '.' && nx !== '[' && nx !== ':' && !isWS(nx)) {
        pushNewline();
      }
    }
    continue;
  }
  if (c === ';') {
    out.push(c);
    i++;
    // Skip existing whitespace and add our own newline
    while (i < src.length && isWS(src[i])) i++;
    if (i < src.length && src[i] !== '}') {
      pushNewline();
    }
    continue;
  }
  if (c === ',' && depth > 0) {
    out.push(c);
    i++;
    while (i < src.length && isWS(src[i])) i++;
    if (i < src.length && src[i] !== ')' && src[i] !== ']' && src[i] !== '}') {
      pushNewline();
    }
    continue;
  }
  if (c === '(') {
    parenDepth++;
    out.push(c); i++;
    continue;
  }
  if (c === ')') {
    parenDepth--;
    out.push(c); i++;
    continue;
  }

  out.push(c);
  i++;
}

const formatted = out.join('').replace(/\n\s*\n\s*\n+/g, '\n\n');
fs.writeFileSync(path, formatted);
console.log('Reformatted', path, '->', formatted.length, 'bytes');
