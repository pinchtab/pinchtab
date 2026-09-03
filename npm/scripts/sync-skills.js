#!/usr/bin/env node

/**
 * The USER path for the pinchtab agent skill: copies the skill bundled in the npm
 * package into every detected agent home, and reports whether each copy matches
 * the bundled one. Runs on npm install/update (postinstall) and on
 * `pinchtab skill update`; `pinchtab skill status` reports without writing.
 *
 * In a source checkout use scripts/install-skills.sh instead: it symlinks the
 * repo skills so edits are reflected immediately. A symlinked target is never
 * written to here.
 */

const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');

const SKILL_NAME = 'pinchtab';
const DEV_VERSION = 'dev';

const stampBlock = /^ {2}pinchtab:\n {4}version: .*\n {4}contentHash: .*\n/m;
const stampFields = /^ {2}pinchtab:\n {4}version: (.*)\n {4}contentHash: (.*)\n/m;

function getBundledSkillsDir() {
  const packagedDir = path.join(__dirname, '..', 'skills', SKILL_NAME);
  if (fs.existsSync(packagedDir)) {
    return packagedDir;
  }
  return path.join(__dirname, '..', '..', 'skills', SKILL_NAME);
}

function getTargetSkillDirs() {
  const home = os.homedir();
  const candidates = [
    path.join(home, '.claude', 'skills', SKILL_NAME),
    path.join(home, '.openclaw', 'workspace', '.agents', 'skills', SKILL_NAME),
    path.join(home, '.cursor', 'skills', SKILL_NAME),
    path.join(home, '.windsurf', 'skills', SKILL_NAME),
    path.join(home, '.codex', 'skills', SKILL_NAME),
  ];
  return candidates.filter((dir) => fs.existsSync(path.dirname(dir)));
}

function listFiles(dir, prefix = '') {
  const names = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === '.DS_Store') {
      continue;
    }
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) {
      names.push(...listFiles(path.join(dir, entry.name), rel));
    } else {
      names.push(rel);
    }
  }
  return names.sort();
}

function stripStamp(skillMd) {
  return skillMd.replace(stampBlock, '').replace(/^metadata:\n(?=\S)/m, '');
}

function readStamp(skillDir) {
  const skillPath = path.join(skillDir, 'SKILL.md');
  if (!fs.existsSync(skillPath)) {
    return null;
  }
  const match = stampFields.exec(fs.readFileSync(skillPath, 'utf8'));
  if (!match) {
    return null;
  }
  return { version: match[1].trim(), contentHash: match[2].trim() };
}

function contentHash(skillDir) {
  const hash = crypto.createHash('sha256');
  for (const rel of listFiles(skillDir)) {
    hash.update(rel);
    hash.update('\0');
    const body = fs.readFileSync(path.join(skillDir, rel));
    hash.update(rel === 'SKILL.md' ? stripStamp(body.toString('utf8')) : body);
    hash.update('\0');
  }
  return `sha256:${hash.digest('hex')}`;
}

function stampSkill(skillDir, version) {
  const skillPath = path.join(skillDir, 'SKILL.md');
  const stamp = { version, contentHash: contentHash(skillDir) };
  const block = `  pinchtab:\n    version: ${stamp.version}\n    contentHash: ${stamp.contentHash}\n`;
  let body = stripStamp(fs.readFileSync(skillPath, 'utf8'));
  if (/^metadata:\n/m.test(body)) {
    body = body.replace(/^metadata:\n/m, `metadata:\n${block}`);
  } else {
    body = body.replace(/^---\n([\s\S]*?)^---\n/m, `---\n$1metadata:\n${block}---\n`);
  }
  fs.writeFileSync(skillPath, body);
  return stamp;
}

function describeBundle(bundledDir) {
  const stamp = readStamp(bundledDir);
  return {
    dir: bundledDir,
    version: stamp ? stamp.version : DEV_VERSION,
    contentHash: stamp ? stamp.contentHash : contentHash(bundledDir),
  };
}

function inspectTarget(target, bundle) {
  let stat;
  try {
    stat = fs.lstatSync(target);
  } catch {
    return { target, state: 'absent', installed: null };
  }
  if (stat.isSymbolicLink()) {
    return { target, state: 'symlink', installed: null };
  }
  const installed = readStamp(target);
  const actual = contentHash(target);
  if (actual === bundle.contentHash) {
    return { target, state: 'current', installed };
  }
  if (!installed) {
    return { target, state: 'unversioned', installed };
  }
  if (actual !== installed.contentHash) {
    return { target, state: 'edited', installed };
  }
  return { target, state: 'stale', installed };
}

function copyDirSync(src, dest) {
  fs.mkdirSync(dest, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const srcPath = path.join(src, entry.name);
    const destPath = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDirSync(srcPath, destPath);
    } else {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

function replaceSkill(bundle, target) {
  fs.rmSync(target, { recursive: true, force: true });
  copyDirSync(bundle.dir, target);
  if (!readStamp(target)) {
    stampSkill(target, bundle.version);
  }
}

const writesFor = {
  absent: () => 'installed',
  stale: () => 'updated',
  edited: (force) => (force ? 'updated' : 'kept'),
  unversioned: (force) => (force ? 'updated' : 'kept'),
  current: () => 'unchanged',
  symlink: () => 'unchanged',
};

function syncSkills({ force = false, check = false, bundledDir = getBundledSkillsDir() } = {}) {
  if (!fs.existsSync(bundledDir)) {
    throw new Error(`bundled skill not found at ${bundledDir}`);
  }
  const bundle = describeBundle(bundledDir);
  const results = getTargetSkillDirs().map((target) => {
    const found = inspectTarget(target, bundle);
    const action = check ? 'checked' : writesFor[found.state](force);
    if (action === 'installed' || action === 'updated') {
      try {
        replaceSkill(bundle, target);
      } catch (err) {
        return { ...found, action: 'failed', error: err.message };
      }
    }
    return { ...found, action };
  });
  return { bundle, results };
}

function isOutOfDate(result) {
  return result.state === 'stale' || result.state === 'edited' || result.state === 'unversioned';
}

function shortHash(contentHash) {
  return contentHash.replace(/^sha256:/, '').slice(0, 12);
}

function describeResult(result, bundle) {
  const from = result.installed ? result.installed.version : 'unversioned';
  switch (result.action) {
    case 'installed':
      return `installed ${bundle.version}`;
    case 'updated':
      return `updated ${from} -> ${bundle.version}`;
    case 'failed':
      return `failed: ${result.error}`;
    default:
      break;
  }
  switch (result.state) {
    case 'current':
      return `current (${bundle.version})`;
    case 'symlink':
      return 'symlink to a checkout; left alone (scripts/install-skills.sh manages it)';
    case 'stale':
      return `stale ${from}; run: pinchtab skill update`;
    case 'edited':
      return `edited locally since ${from}; kept. Replace it with: pinchtab skill update --force`;
    case 'unversioned':
      return 'unversioned copy that differs from the bundled skill; kept. Replace it with: pinchtab skill update --force';
    case 'absent':
      return `not installed; run: pinchtab skill update`;
    default:
      return result.state;
  }
}

function formatReport({ bundle, results }) {
  const lines = [`pinchtab skill ${bundle.version} (${shortHash(bundle.contentHash)})`];
  if (results.length === 0) {
    lines.push(
      '  no agent skill directories detected; the skill is synced only where an agent is already installed'
    );
    return lines;
  }
  for (const result of results) {
    lines.push(`  ${result.target}: ${describeResult(result, bundle)}`);
  }
  return lines;
}

if (require.main === module) {
  const argv = process.argv.slice(2);
  const outcome = syncSkills({
    force: argv.includes('--force'),
    check: argv.includes('--check'),
  });
  console.log(formatReport(outcome).join('\n'));
  const failed = outcome.results.some((r) => r.action === 'failed');
  const outOfDate = outcome.results.some(isOutOfDate);
  process.exit(failed || (argv.includes('--check') && outOfDate) ? 1 : 0);
}

module.exports = {
  SKILL_NAME,
  syncSkills,
  formatReport,
  isOutOfDate,
  getTargetSkillDirs,
  getBundledSkillsDir,
  contentHash,
  stampSkill,
  readStamp,
  stripStamp,
};
