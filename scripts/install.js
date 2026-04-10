#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');
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
  const mirrorBase = mirror ? mirror.replace(/\/+$/, '') : '';
  const githubBase = 'https://github.com/AgentConnect/awiki-cli/releases/download'.replace(/\/+$/, '');
  const tag = `v${version}`;

  const urls = [];
  // If a mirror is configured, try it first.
  if (mirrorBase) {
    urls.push(`${mirrorBase}/${tag}/${fileName}`);
  }
  // Always fall back to GitHub.
  urls.push(`${githubBase}/${tag}/${fileName}`);

  return {
    urls,
    fileName,
  };
}

function download(url, destPath) {
  return new Promise((resolve, reject) => {
    const curlCmd = process.env.AWIKI_CLI_CURL || 'curl';
    const isWindows = process.platform === 'win32';
    const args = [];

    if (isWindows) {
      // On Windows, avoid CRYPT_E_REVOCATION_OFFLINE errors when the
      // certificate revocation list server is unreachable.
      args.push('--ssl-revoke-best-effort');
    }

    args.push(
      '--fail',
      '--location',
      '--silent',
      '--show-error',
      '--connect-timeout',
      '10',
      '--max-time',
      '60',
      '--output',
      destPath,
      url
    );

    const child = spawn(curlCmd, args, { stdio: ['ignore', 'ignore', 'pipe'] });
    let stderr = '';

    child.stderr.on('data', chunk => {
      stderr += chunk.toString();
    });

    child.on('error', err => {
      if (err && err.code === 'ENOENT') {
        reject(new Error('curl not found. Please install curl or set AWIKI_CLI_CURL to a valid curl binary.'));
      } else {
        reject(err);
      }
    });

    child.on('exit', code => {
      if (code === 0) {
        resolve();
      } else {
        const msg = stderr.trim() || `curl exited with code ${code}`;
        reject(new Error(msg));
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
  const { urls, fileName } = getDownloadUrl(version, osName, arch);

  const binDir = path.join(rootDir, 'bin');
  ensureDir(binDir);

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'awiki-cli-'));
  const archivePath = path.join(tmpDir, fileName);

  let lastError;
  for (const url of urls) {
    console.log(`Downloading awiki-cli ${version} for ${osName}/${arch} from ${url} ...`);
    try {
      await download(url, archivePath);
      lastError = undefined;
      break;
    } catch (err) {
      lastError = err;
      console.error(`[awiki-cli] Download failed from ${url}: ${err.message}`);
    }
  }

  if (lastError) {
    throw lastError;
  }

  console.log(`Extracting to ${binDir} ...`);
  try {
    await extractArchive(archivePath, binDir, osName);
  } finally {
    try {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    } catch (e) {
      // best effort cleanup
    }
  }

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
     console.error(
       '\nIf you are behind a firewall or using a restricted network, you can:\n' +
       '  - Set AWIKI_CLI_DOWNLOAD_MIRROR to a reachable HTTPS base URL,\n' +
       '  - Or manually download the archive and extract it into the awiki-cli bin directory.\n'
     );
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
