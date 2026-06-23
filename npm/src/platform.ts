import * as path from 'path';
import * as fs from 'fs';

export interface Platform {
  os: 'darwin' | 'linux' | 'windows';
  arch: 'amd64' | 'arm64';
}

// detectPlatform maps Node's process.platform/process.arch to the goreleaser
// binary triple. The values are injectable so tests can drive the full matrix
// without re-declaring the mapping.
export function detectPlatform(
  platform: string = process.platform,
  nodeArch: string = process.arch
): Platform {
  // Only support x64 (amd64) and arm64
  let arch: 'amd64' | 'arm64';
  if (nodeArch === 'x64') {
    arch = 'amd64';
  } else if (nodeArch === 'arm64') {
    arch = 'arm64';
  } else {
    throw new Error(
      `Unsupported architecture: ${nodeArch}. ` + `Only x64 (amd64) and arm64 are supported.`
    );
  }

  const osMap: Record<string, 'darwin' | 'linux' | 'windows'> = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const os_name = osMap[platform];
  if (!os_name) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os: os_name, arch };
}

export function getBinaryName(platform: Platform): string {
  const { os, arch } = platform;
  const archName = arch === 'arm64' ? 'arm64' : 'amd64';

  if (os === 'windows') {
    return `pinchtab-${os}-${archName}.exe`;
  }
  return `pinchtab-${os}-${archName}`;
}

export function getBinDir(): string {
  return path.join(process.env.HOME || process.env.USERPROFILE || '', '.pinchtab', 'bin');
}

export function findRepoRoot(fromDir: string): string | null {
  let dir = path.resolve(fromDir);

  while (dir) {
    if (
      fs.existsSync(path.join(dir, 'go.mod')) &&
      fs.existsSync(path.join(dir, 'cmd', 'pinchtab'))
    ) {
      return dir;
    }

    const parent = path.dirname(dir);
    if (parent === dir) {
      break;
    }
    dir = parent;
  }

  return null;
}

export function getCheckoutBinaryPath(fromDir: string): string | null {
  const repoRoot = findRepoRoot(fromDir);
  if (!repoRoot) {
    return null;
  }
  return path.join(repoRoot, 'pinchtab-dev');
}

export function getBinaryPath(binaryName: string, version?: string): string {
  // Version-specific path: ~/.pinchtab/bin/0.7.0/pinchtab-darwin-arm64
  // This allows multiple versions to coexist and prevents silent overwrites
  if (version) {
    return path.join(getBinDir(), version, binaryName);
  }

  // Fallback to version-less for backwards compat
  return path.join(getBinDir(), binaryName);
}

// readPackageVersion walks up from fromDir to the nearest package.json and
// returns its version. Shared so the wrapper, SDK, and installer agree on which
// versioned binary directory to look in.
export function readPackageVersion(fromDir: string): string {
  let dir = path.resolve(fromDir);

  while (dir) {
    const pkgPath = path.join(dir, 'package.json');
    if (fs.existsSync(pkgPath)) {
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
      if (typeof pkg.version === 'string' && pkg.version.trim() !== '') {
        return pkg.version;
      }
    }

    const parent = path.dirname(dir);
    if (parent === dir) {
      break;
    }
    dir = parent;
  }

  throw new Error(`package.json with a version not found above ${fromDir}`);
}

// resolveManagedBinaryPath returns the path of the downloaded binary for the
// current platform: the version-specific path when it exists, otherwise the
// version-less path for backwards compatibility. It does NOT assert existence —
// callers decide how to report a missing binary.
export function resolveManagedBinaryPath(fromDir: string): string {
  const binaryName = getBinaryName(detectPlatform());

  let version: string | undefined;
  try {
    version = readPackageVersion(fromDir);
  } catch {
    // Fall back to the version-less path when no package.json version is found.
  }

  if (version) {
    const versioned = getBinaryPath(binaryName, version);
    if (fs.existsSync(versioned)) {
      return versioned;
    }
  }

  return getBinaryPath(binaryName);
}

// firstSubcommand returns the first non-flag argument, skipping the global
// `--server <url>` / `--server=<url>` option so callers can detect the
// subcommand (e.g. `mcp`) regardless of preceding flags.
export function firstSubcommand(argv: string[]): string | null {
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--server') {
      i += 1;
      continue;
    }
    if (arg.startsWith('--server=')) continue;
    if (!arg.startsWith('-')) return arg;
  }
  return null;
}
