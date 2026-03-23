#!/usr/bin/env node

/**
 * Syncs bundled skill files to detected agent skill directories.
 * Only updates directories that already exist (doesn't create new ones).
 *
 * Used by:
 *   - postinstall (automatic on npm install/update)
 *   - `pinchtab skill update` (manual trigger)
 */

const fs = require('fs');
const path = require('path');
const os = require('os');

const SKILL_NAME = 'pinchtab';

/**
 * Returns the path to the bundled skills directory inside the npm package.
 */
function getBundledSkillsDir() {
  return path.join(__dirname, '..', 'skills', SKILL_NAME);
}

/**
 * Returns candidate agent skill directories to sync to.
 * Only returns paths whose parent directory already exists.
 */
function getTargetSkillDirs() {
  const home = os.homedir();

  const candidates = [
    // Claude Code / Claude Desktop
    path.join(home, '.claude', 'skills', SKILL_NAME),
    // OpenClaw
    path.join(home, '.openclaw', 'workspace', '.agents', 'skills', SKILL_NAME),
    // Cursor
    path.join(home, '.cursor', 'skills', SKILL_NAME),
    // Windsurf
    path.join(home, '.windsurf', 'skills', SKILL_NAME),
  ];

  return candidates.filter((dir) => {
    // Check if the parent skills directory exists
    const parentDir = path.dirname(dir);
    return fs.existsSync(parentDir);
  });
}

/**
 * Recursively copies a directory.
 */
function copyDirSync(src, dest) {
  if (!fs.existsSync(dest)) {
    fs.mkdirSync(dest, { recursive: true });
  }

  const entries = fs.readdirSync(src, { withFileTypes: true });
  for (const entry of entries) {
    const srcPath = path.join(src, entry.name);
    const destPath = path.join(dest, entry.name);

    if (entry.isDirectory()) {
      copyDirSync(srcPath, destPath);
    } else {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

/**
 * Syncs bundled skills to all detected agent directories.
 * @param {object} options
 * @param {boolean} options.verbose - Print detailed output
 * @returns {{ updated: string[], skipped: string[] }}
 */
function syncSkills({ verbose = false } = {}) {
  const bundledDir = getBundledSkillsDir();
  const updated = [];
  const skipped = [];

  if (!fs.existsSync(bundledDir)) {
    if (verbose) {
      console.error(`⚠ Bundled skills not found at ${bundledDir}`);
    }
    return { updated, skipped };
  }

  const targets = getTargetSkillDirs();

  if (targets.length === 0) {
    if (verbose) {
      console.log('No agent skill directories detected. Nothing to sync.');
    }
    return { updated, skipped };
  }

  for (const target of targets) {
    try {
      copyDirSync(bundledDir, target);
      updated.push(target);
      if (verbose) {
        console.log(`  ✓ ${target}`);
      }
    } catch (err) {
      skipped.push(target);
      if (verbose) {
        console.warn(`  ✗ ${target}: ${err.message}`);
      }
    }
  }

  return { updated, skipped };
}

// If run directly (not required as module)
if (require.main === module) {
  const verbose = process.argv.includes('--verbose') || process.argv.includes('-v');
  console.log('Syncing Pinchtab skill files...\n');
  const { updated, skipped } = syncSkills({ verbose });

  if (updated.length > 0) {
    console.log(`\n✓ Updated ${updated.length} skill director${updated.length === 1 ? 'y' : 'ies'}`);
  } else {
    console.log('\nNo agent skill directories found to update.');
    console.log('Skill directories are synced only where agents are already installed.');
  }

  if (skipped.length > 0) {
    console.log(`⚠ Skipped ${skipped.length} (permission errors)`);
  }
}

module.exports = { syncSkills, getTargetSkillDirs, getBundledSkillsDir };
