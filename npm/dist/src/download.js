"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.ensureBinary = ensureBinary;
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
const https = __importStar(require("https"));
const GITHUB_REPO = 'pinchtab/pinchtab';
const VERSION = '0.7.0';
function detectPlatform() {
    const platform = process.platform;
    const arch = process.arch === 'arm64' ? 'arm64' : 'x64';
    const osMap = {
        darwin: 'darwin',
        linux: 'linux',
        win32: 'windows',
    };
    const os = osMap[platform];
    if (!os) {
        throw new Error(`Unsupported platform: ${platform}`);
    }
    return { os, arch };
}
function getBinaryName(platform) {
    const { os, arch } = platform;
    const archName = arch === 'arm64' ? 'arm64' : 'x64';
    if (os === 'windows') {
        return `pinchtab-${os}-${archName}.exe`;
    }
    return `pinchtab-${os}-${archName}`;
}
function getBinDir() {
    return path.join(process.env.HOME || process.env.USERPROFILE || '', '.pinchtab', 'bin');
}
async function downloadBinary(platform) {
    const binaryName = getBinaryName(platform);
    const binDir = getBinDir();
    const binaryPath = path.join(binDir, binaryName);
    // Skip if already exists
    if (fs.existsSync(binaryPath)) {
        console.log(`✓ Pinchtab binary already present: ${binaryPath}`);
        return;
    }
    const downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/v${VERSION}/${binaryName}`;
    console.log(`Downloading Pinchtab ${VERSION} for ${platform.os}-${platform.arch}...`);
    console.log(`URL: ${downloadUrl}`);
    // Ensure directory exists
    if (!fs.existsSync(binDir)) {
        fs.mkdirSync(binDir, { recursive: true });
    }
    return new Promise((resolve, reject) => {
        https
            .get(downloadUrl, (response) => {
            if (response.statusCode === 404) {
                reject(new Error(`Binary not found: ${downloadUrl}. Make sure v${VERSION} is released on GitHub.`));
                return;
            }
            if (response.statusCode !== 200) {
                reject(new Error(`Download failed with status ${response.statusCode}`));
                return;
            }
            const file = fs.createWriteStream(binaryPath);
            response.pipe(file);
            file.on('finish', () => {
                file.close();
                // Make executable
                fs.chmodSync(binaryPath, 0o755);
                console.log(`✓ Downloaded to ${binaryPath}`);
                resolve();
            });
            file.on('error', (err) => {
                fs.unlink(binaryPath, () => { });
                reject(err);
            });
        })
            .on('error', (err) => {
            reject(err);
        });
    });
}
async function ensureBinary() {
    const platform = detectPlatform();
    await downloadBinary(platform);
    const binDir = getBinDir();
    const binaryName = getBinaryName(platform);
    return path.join(binDir, binaryName);
}
