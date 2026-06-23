import * as fs from 'fs';
import * as path from 'path';
import { ensureBinary } from './download';
import { getCheckoutBinaryPath } from './platform';

interface SkillSyncResult {
  updated: string[];
}

// syncBundledSkills mirrors bundled skill files into detected agent directories.
// Non-fatal: a sync failure must never block the install. sync-skills is a
// CommonJS script shipped alongside the package, loaded relative to the compiled
// location (dist/src -> ../../scripts).
function syncBundledSkills(): void {
  try {
    const syncSkillsPath = path.join(__dirname, '..', '..', 'scripts', 'sync-skills');
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { syncSkills } = require(syncSkillsPath) as {
      syncSkills: (opts: { verbose: boolean }) => SkillSyncResult;
    };
    const { updated } = syncSkills({ verbose: false });
    if (updated.length > 0) {
      console.log(
        `✓ Synced skill files to ${updated.length} agent director${updated.length === 1 ? 'y' : 'ies'}`
      );
    }
  } catch (_err) {
    // Non-fatal: skill sync failure shouldn't block install
  }
}

// runPostinstall is the single npm install entrypoint. It uses the shared
// platform/download helpers (the one source of truth) rather than a parallel
// installer: source-checkout skip, binary download (versioned path, proxy +
// checksum from ./download), skill sync, and the install UX / exit codes.
export async function runPostinstall(): Promise<void> {
  try {
    const checkoutBinaryPath = getCheckoutBinaryPath(__dirname);
    if (checkoutBinaryPath) {
      if (!fs.existsSync(checkoutBinaryPath)) {
        throw new Error(
          `Expected local source-checkout binary at ${checkoutBinaryPath}\n` +
            `Build it first with: bash scripts/npm-dev-binary.sh`
        );
      }
      console.log(`✓ Using source-checkout Pinchtab binary: ${checkoutBinaryPath}`);
    } else {
      let binaryPath: string;
      try {
        binaryPath = await ensureBinary();
      } catch (downloadErr) {
        const errMsg = downloadErr instanceof Error ? downloadErr.message : String(downloadErr);
        // A 404 means the release isn't published yet (e.g. CI/release window):
        // warn but don't fail the install.
        if (errMsg.includes('404') || errMsg.includes('Not found')) {
          console.warn('\n⚠ Pinchtab binary not yet available (release in progress).');
          console.warn('  On first use, run: npm rebuild pinchtab');
          process.exit(0);
        }
        throw downloadErr;
      }

      if (!fs.existsSync(binaryPath)) {
        throw new Error(
          `Binary was not successfully downloaded to ${binaryPath}\n` +
            `This usually means the GitHub release doesn't have the binary for your platform.`
        );
      }
    }

    syncBundledSkills();

    console.log('✓ Pinchtab setup complete');
    process.exit(0);
  } catch (err) {
    console.error('\n✗ Failed to setup Pinchtab:');
    console.error(
      `  ${(err instanceof Error ? err.message : String(err)).split('\n').join('\n  ')}`
    );
    console.error('\nTroubleshooting:');
    console.error('  • Check your internet connection');
    console.error('  • Verify the release exists: https://github.com/pinchtab/pinchtab/releases');
    console.error('  • Try again: npm rebuild pinchtab');
    if (process.env.HTTPS_PROXY || process.env.HTTP_PROXY) {
      console.error('  • Check proxy settings (HTTPS_PROXY / HTTP_PROXY)');
    }
    process.exit(1);
  }
}
