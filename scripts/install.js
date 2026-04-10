#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');
const https = require('https');
const { spawn } = require('child_process');

function mapPlatform() {
  const p = process.platform;
  if (p === 'darwin') return 'darwin';
  if (p === 'linux') return 'linux';
  if (p === 'win32') return 'windows';
  throw new Error(`Unsupported platform: ${p}`);
}

function mapArch() {
  const a = process.arch;
  if (a === 'x64') return 'amd64';
  if (a === 'arm64') return 'arm64';
  throw new Error(`Unsupported architecture: ${a}`);
}

function getVersion(pkg) {
  const v = pkg && typeof pkg.version === 'string' ? pkg.version.trim() : '';
  if (!v) {
    throw new Error('version is missing in package.json');
  }
  return v;
}

function getDownloadUrl(version, osName, arch) {
  const archiveBaseName = `awiki-cli-${version}-${osName}-${arch}`;
  const ext = osName === 'windows' ? 'zip' : 'tar.gz';
  const fileName = `${archiveBaseName}.${ext}`;

  const mirror = (process.env.AWIKI_CLI_DOWNLOAD_MIRROR || '').trim();
  const base = mirror || 'https://github.com/AgentConnect/awiki-cli/releases/download';
  const baseNoSlash = base.replace(/\/+$/, '');
  const tag = `v${version}`;

  return {
    url: `${baseNoSlash}/${tag}/${fileName}`,
    fileName,
  };
}

function download(url, destPath) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destPath);
    let finished = false;

    const req = https.get(url, res => {
      if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        // handle redirect
        res.destroy();
        file.close(() => fs.unlink(destPath, () => {
          download(res.headers.location, destPath).then(resolve, reject);
        }));
        return;
      }

      if (res.statusCode !== 200) {
        res.resume();
        file.close(() => fs.unlink(destPath, () => {
          reject(new Error(`Download failed with status code ${res.statusCode}`));
        }));
        return;
      }

      res.pipe(file);
      file.on('finish', () => {
        finished = true;
        file.close(resolve);
      });
    });

    req.on('error', err => {
      if (!finished) {
        file.close(() => fs.unlink(destPath, () => reject(err)));
      } else {
        reject(err);
      }
    });
  });
}

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function extractArchive(archivePath, destDir, osName) {
  return new Promise((resolve, reject) => {
    let cmd;
    let args;

    if (osName === 'windows') {
      // Use PowerShell Expand-Archive; requires PowerShell 5+ (Windows 10/11)
      cmd = 'powershell';
      args = [
        '-NoProfile',
        '-NonInteractive',
        '-Command',
        `Expand-Archive -LiteralPath '${archivePath}' -DestinationPath '${destDir}' -Force`,
      ];
    } else {
      cmd = 'tar';
      args = ['-xzf', archivePath, '-C', destDir];
    }

    const child = spawn(cmd, args, { stdio: 'inherit' });
    child.on('error', reject);
    child.on('exit', code => {
      if (code === 0) resolve();
      else reject(new Error(`Extraction command ${cmd} exited with code ${code}`));
    });
  });
}

async function main() {
  const rootDir = path.resolve(__dirname, '..');
  const pkgPath = path.join(rootDir, 'package.json');
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

  const version = getVersion(pkg);
  const osName = mapPlatform();
  const arch = mapArch();
  const { url, fileName } = getDownloadUrl(version, osName, arch);

  const binDir = path.join(rootDir, 'bin');
  ensureDir(binDir);

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'awiki-cli-'));
  const archivePath = path.join(tmpDir, fileName);

  console.log(`Downloading awiki-cli ${version} for ${osName}/${arch} from ${url} ...`);
  await download(url, archivePath);

  console.log(`Extracting to ${binDir} ...`);
  await extractArchive(archivePath, binDir, osName);

  const exeName = osName === 'windows' ? 'awiki-cli.exe' : 'awiki-cli';
  const exePath = path.join(binDir, exeName);

  if (osName !== 'windows') {
    try {
      fs.chmodSync(exePath, 0o755);
    } catch (e) {
      // best effort
    }
  }

  console.log(`awiki-cli binary is installed at ${exePath}`);
}

if (require.main === module) {
  main().catch(err => {
    console.error(`[awiki-cli] Failed to install binary: ${err.message}`);
    process.exit(1);
  });
}

module.exports = {
  _internal: {
    mapPlatform,
    mapArch,
    getVersion,
    getDownloadUrl,
  },
};
