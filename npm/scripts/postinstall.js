#!/usr/bin/env node

const path = require('path');
const fs = require('fs');

// The installer logic lives in TypeScript (src/postinstall.ts), compiled to
// dist/src/postinstall.js and shipped in the published package, so there is one
// source of truth for platform/download/checksum/checkout behavior.
//
// In a source checkout, npm runs `postinstall` before `prepare` builds dist;
// skip cleanly in that case — the build follows, and source checkouts use the
// local pinchtab-dev binary at runtime regardless.
const compiled = path.join(__dirname, '..', 'dist', 'src', 'postinstall.js');
if (!fs.existsSync(compiled)) {
  process.exit(0);
}

require(compiled).runPostinstall();
