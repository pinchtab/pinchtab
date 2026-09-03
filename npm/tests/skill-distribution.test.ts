/**
 * Skill distribution: the npm user path (scripts/sync-skills.js).
 *
 * The skill an agent loads is a copy in an agent home, so a stale copy teaches a
 * contract the server no longer produces. These tests drive the sync against a
 * fake HOME and a fake bundle and pin: the stamp, the hash that ignores the
 * stamp, install / refresh / keep-an-edited-copy / leave-a-symlink, and that
 * status mode writes nothing.
 */

import { describe, test, beforeEach, afterEach } from 'node:test';
import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

// eslint-disable-next-line @typescript-eslint/no-require-imports
const sync = require('../../scripts/sync-skills') as {
  syncSkills: (opts: { force?: boolean; check?: boolean; bundledDir: string }) => Outcome;
  formatReport: (outcome: Outcome) => string[];
  isOutOfDate: (result: Result) => boolean;
  contentHash: (dir: string) => string;
  stampSkill: (dir: string, version: string) => { version: string; contentHash: string };
  readStamp: (dir: string) => { version: string; contentHash: string } | null;
};

interface Result {
  target: string;
  state: string;
  action: string;
  installed: { version: string; contentHash: string } | null;
}
interface Outcome {
  bundle: { dir: string; version: string; contentHash: string };
  results: Result[];
}

const skillMd = (body: string) =>
  `---\nname: pinchtab\ndescription: "browser control"\nmetadata:\n  openclaw:\n    requires:\n      bins:\n        - pinchtab\n---\n\n${body}\n`;

let root: string;
let savedHome: string | undefined;

function writeBundle(name: string, body: string, version: string): string {
  const dir = path.join(root, name);
  fs.mkdirSync(path.join(dir, 'references'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'SKILL.md'), skillMd(body));
  fs.writeFileSync(path.join(dir, 'references', 'commands.md'), `# commands\n${body}\n`);
  sync.stampSkill(dir, version);
  return dir;
}

function claudeSkillDir(): string {
  const skills = path.join(root, 'home', '.claude', 'skills');
  fs.mkdirSync(skills, { recursive: true });
  return path.join(skills, 'pinchtab');
}

function only(outcome: Outcome): Result {
  assert.strictEqual(outcome.results.length, 1, JSON.stringify(outcome.results));
  return outcome.results[0];
}

beforeEach(() => {
  root = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-skill-sync-'));
  savedHome = process.env.HOME;
  process.env.HOME = path.join(root, 'home');
  fs.mkdirSync(process.env.HOME, { recursive: true });
});

afterEach(() => {
  process.env.HOME = savedHome;
  fs.rmSync(root, { recursive: true, force: true });
});

describe('the version stamp', () => {
  test('lands under metadata and hashes the content it is not part of', () => {
    const dir = writeBundle('bundle', 'click navigates: OK navigated', '1.2.3');
    const stamped = fs.readFileSync(path.join(dir, 'SKILL.md'), 'utf8');
    assert.match(
      stamped,
      /^metadata:\n {2}pinchtab:\n {4}version: 1\.2\.3\n {4}contentHash: sha256:[0-9a-f]{64}\n {2}openclaw:/m
    );
    assert.deepStrictEqual(sync.readStamp(dir), {
      version: '1.2.3',
      contentHash: sync.contentHash(dir),
    });

    const restamped = sync.stampSkill(dir, '9.9.9');
    assert.strictEqual(
      restamped.contentHash,
      sync.contentHash(dir),
      'restamping must not move the hash'
    );
    assert.strictEqual(sync.readStamp(dir)?.version, '9.9.9');
  });

  test('is added to a frontmatter with no metadata block', () => {
    const dir = path.join(root, 'bare');
    fs.mkdirSync(dir);
    fs.writeFileSync(path.join(dir, 'SKILL.md'), '---\nname: pinchtab\n---\n\nbody\n');
    sync.stampSkill(dir, '0.1.0');
    assert.deepStrictEqual(sync.readStamp(dir), {
      version: '0.1.0',
      contentHash: sync.contentHash(dir),
    });
    assert.match(
      fs.readFileSync(path.join(dir, 'SKILL.md'), 'utf8'),
      /^---\nname: pinchtab\nmetadata:\n {2}pinchtab:\n/
    );
  });

  test('changes with any file in the tree, not just SKILL.md', () => {
    const dir = writeBundle('bundle', 'v1', '1.0.0');
    const before = sync.contentHash(dir);
    fs.appendFileSync(path.join(dir, 'references', 'commands.md'), 'nav --timeout <seconds>\n');
    assert.notStrictEqual(sync.contentHash(dir), before);
  });
});

describe('pinchtab skill update', () => {
  test('detects nothing when no agent home exists', () => {
    const bundledDir = writeBundle('bundle', 'v1', '1.0.0');
    const outcome = sync.syncSkills({ bundledDir });
    assert.deepStrictEqual(outcome.results, []);
    assert.match(sync.formatReport(outcome).join('\n'), /no agent skill directories detected/);
  });

  test('installs into a detected home and the copy carries the bundled stamp', () => {
    const bundledDir = writeBundle('bundle', 'v1', '1.0.0');
    const target = claudeSkillDir();
    const result = only(sync.syncSkills({ bundledDir }));
    assert.strictEqual(result.action, 'installed');
    assert.deepStrictEqual(sync.readStamp(target), sync.readStamp(bundledDir));
    assert.strictEqual(
      fs.readFileSync(path.join(target, 'references', 'commands.md'), 'utf8'),
      '# commands\nv1\n'
    );
    assert.strictEqual(only(sync.syncSkills({ bundledDir })).action, 'unchanged');
  });

  test('refreshes an untouched copy of an older bundle and drops files the new bundle removed', () => {
    const target = claudeSkillDir();
    only(sync.syncSkills({ bundledDir: writeBundle('old', 'treat 409 as success', '0.9.0') }));
    fs.writeFileSync(path.join(target, 'references', 'gone.md'), 'obsolete\n');
    const stale = sync.stampSkill(target, '0.9.0');
    assert.strictEqual(sync.readStamp(target)?.contentHash, stale.contentHash);

    const bundledDir = writeBundle('new', 'OK navigated; JSON adds refsStale', '1.0.0');
    const result = only(sync.syncSkills({ bundledDir }));
    assert.strictEqual(result.state, 'stale');
    assert.strictEqual(result.action, 'updated');
    assert.strictEqual(result.installed?.version, '0.9.0');
    assert.match(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8'), /refsStale/);
    assert.ok(!fs.existsSync(path.join(target, 'references', 'gone.md')));
    assert.match(
      sync.formatReport(sync.syncSkills({ bundledDir, check: true })).join('\n'),
      /current \(1\.0\.0\)/
    );
  });

  test('keeps a copy the user edited, says so, and replaces it only under --force', () => {
    const bundledDir = writeBundle('bundle', 'v1', '1.0.0');
    const target = claudeSkillDir();
    only(sync.syncSkills({ bundledDir }));
    fs.appendFileSync(path.join(target, 'SKILL.md'), '\nMy own note.\n');
    const newer = writeBundle('newer', 'v2', '1.1.0');

    const keptOutcome = sync.syncSkills({ bundledDir: newer });
    const kept = only(keptOutcome);
    assert.strictEqual(kept.state, 'edited');
    assert.strictEqual(kept.action, 'kept');
    assert.match(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8'), /My own note/);
    assert.match(
      sync.formatReport(keptOutcome).join('\n'),
      /edited locally since 1\.0\.0.*--force/
    );

    const forced = only(sync.syncSkills({ bundledDir: newer, force: true }));
    assert.strictEqual(forced.action, 'updated');
    assert.doesNotMatch(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8'), /My own note/);
    assert.strictEqual(sync.readStamp(target)?.version, '1.1.0');
  });

  test('keeps an unversioned copy that differs, since it cannot tell edited from stale', () => {
    const target = claudeSkillDir();
    fs.mkdirSync(target, { recursive: true });
    fs.writeFileSync(path.join(target, 'SKILL.md'), skillMd('treat 409 as success'));
    const result = only(sync.syncSkills({ bundledDir: writeBundle('bundle', 'v1', '1.0.0') }));
    assert.strictEqual(result.state, 'unversioned');
    assert.strictEqual(result.action, 'kept');
    assert.ok(sync.isOutOfDate(result));
    assert.match(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8'), /409/);
  });

  test('never writes through a symlink, which is the developer path', () => {
    const checkout = writeBundle('checkout', 'edited live', '1.0.0');
    const target = claudeSkillDir();
    fs.symlinkSync(checkout, target);
    const result = only(
      sync.syncSkills({ bundledDir: writeBundle('bundle', 'v2', '2.0.0'), force: true })
    );
    assert.strictEqual(result.state, 'symlink');
    assert.strictEqual(result.action, 'unchanged');
    assert.ok(fs.lstatSync(target).isSymbolicLink());
    assert.match(fs.readFileSync(path.join(checkout, 'SKILL.md'), 'utf8'), /edited live/);
  });
});

describe('pinchtab skill status', () => {
  test('reports a stale copy without touching it', () => {
    const target = claudeSkillDir();
    only(sync.syncSkills({ bundledDir: writeBundle('old', 'v1', '0.9.0') }));
    const before = fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8');
    const outcome = sync.syncSkills({ bundledDir: writeBundle('new', 'v2', '1.0.0'), check: true });
    const result = only(outcome);
    assert.strictEqual(result.state, 'stale');
    assert.strictEqual(result.action, 'checked');
    assert.ok(sync.isOutOfDate(result));
    assert.strictEqual(fs.readFileSync(path.join(target, 'SKILL.md'), 'utf8'), before);
    assert.match(
      sync.formatReport(outcome).join('\n'),
      /stale 0\.9\.0; run: pinchtab skill update/
    );
  });

  test('a bundle without a stamp reads as the dev version', () => {
    const dir = path.join(root, 'src');
    fs.mkdirSync(dir);
    fs.writeFileSync(path.join(dir, 'SKILL.md'), skillMd('from a checkout'));
    const outcome = sync.syncSkills({ bundledDir: dir, check: true });
    assert.strictEqual(outcome.bundle.version, 'dev');
    assert.strictEqual(outcome.bundle.contentHash, sync.contentHash(dir));
  });
});
