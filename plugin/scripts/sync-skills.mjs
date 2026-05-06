import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const pluginDir = resolve(scriptDir, '..');
const sourceDir = resolve(pluginDir, '..', 'skills', 'pinchtab');
const destinationDir = resolve(pluginDir, 'skills', 'pinchtab');

if (!existsSync(sourceDir)) {
  console.error(`missing source skill directory: ${sourceDir}`);
  process.exit(1);
}

mkdirSync(resolve(pluginDir, 'skills'), { recursive: true });
rmSync(destinationDir, { recursive: true, force: true });
cpSync(sourceDir, destinationDir, { recursive: true });

console.log(`synced ${sourceDir} -> ${destinationDir}`);
