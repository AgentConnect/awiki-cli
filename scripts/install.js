#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');
const { spawn, execSync } = require('child_process');

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

function deriveBootstrapChannel(version) {
  const override = (process.env.AWIKI_CLI_BOOTSTRAP_CHANNEL || '').trim();
  if (override) {
    return override;
  }
  return version.includes('-') ? 'prerelease' : 'stable';
}

function getUpdateCheckURL() {
  const explicit = (process.env.AWIKI_CLI_UPDATE_CHECK_URL || '').trim();
  if (explicit) {
    return explicit;
  }
  const base = (process.env.AWIKI_CLI_UPDATE_BASE_URL || 'https://awiki.ai').trim().replace(/\/+$/, '');
  return `${base}/api/cli/updates/check`;
}

function buildUpdateCheckRequest(pkg, osName, arch) {
  const version = getVersion(pkg);
  return {
    schema_version: 1,
    current_version: version,
    channel: deriveBootstrapChannel(version),
    goos: osName,
    goarch: arch,
    host_agent: 'unknown',
    host_version: '',
    host_capabilities: [],
    skill_format_version: 'v1',
  };
}

function requestUpdateMetadata(url, payload) {
  return new Promise((resolve, reject) => {
    const curlCmd = process.env.AWIKI_CLI_CURL || 'curl';
    const isWindows = process.platform === 'win32';
    const args = [];

    if (isWindows) {
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
      '--header',
      'Content-Type: application/json',
      '--data',
      JSON.stringify(payload),
      url
    );

    const child = spawn(curlCmd, args, { stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';

    child.stdout.on('data', chunk => {
      stdout += chunk.toString();
    });
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
      if (code !== 0) {
        reject(new Error(stderr.trim() || `curl exited with code ${code}`));
        return;
      }
      try {
        resolve(JSON.parse(stdout));
      } catch (err) {
        reject(new Error(`Failed to parse update metadata JSON: ${err.message}`));
      }
    });
  });
}

function resolveArtifact(metadata, osName, arch) {
  if (metadata && metadata.artifact && typeof metadata.artifact.url === 'string') {
    return metadata.artifact;
  }
  const platformKey = `${osName}-${arch}`;
  if (metadata && Array.isArray(metadata.artifacts) && metadata.artifacts.length > 0) {
    const direct = metadata.artifacts.find(item => item && item.platform === platformKey);
    if (direct) {
      return direct;
    }
  }
  if (metadata && metadata.artifacts && typeof metadata.artifacts === 'object') {
    if (metadata.artifacts[platformKey]) {
      return metadata.artifacts[platformKey];
    }
  }
  throw new Error('update metadata is missing an installable artifact');
}

function rewriteURLWithMirror(url) {
  const mirror = (process.env.AWIKI_CLI_DOWNLOAD_MIRROR || '').trim();
  if (!mirror) {
    return [];
  }
  try {
    const parsed = new URL(url);
    const mirrorBase = mirror.replace(/\/+$/, '');
    const rewritten = `${mirrorBase}${parsed.pathname}${parsed.search}`;
    return [rewritten];
  } catch {
    return [];
  }
}

function buildArtifactDownloadURLs(artifactURL) {
  const candidates = [];
  for (const url of rewriteURLWithMirror(artifactURL)) {
    candidates.push(url);
  }
  candidates.push(artifactURL);
  return [...new Set(candidates.filter(Boolean))];
}

function download(url, destPath) {
  return new Promise((resolve, reject) => {
    const curlCmd = process.env.AWIKI_CLI_CURL || 'curl';
    const isWindows = process.platform === 'win32';
    const args = [];

    if (isWindows) {
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

function verifySHA256(filePath, expected) {
  const normalized = String(expected || '').trim().toLowerCase();
  if (!normalized) {
    throw new Error('artifact sha256 is missing in update metadata');
  }
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filePath));
  const actual = hash.digest('hex');
  if (actual !== normalized) {
    throw new Error(`artifact sha256 mismatch: got ${actual} want ${normalized}`);
  }
}

function applySystemProxyFromMacOS() {
  if (process.env.HTTPS_PROXY || process.env.HTTP_PROXY) {
    return;
  }
  if (process.platform !== 'darwin') {
    return;
  }

  try {
    const output = execSync('scutil --proxy', { encoding: 'utf8' });
    const lines = output.split('\n').map(line => line.trim()).filter(Boolean);
    const values = {};

    for (const line of lines) {
      const parts = line.split(':');
      if (parts.length < 2) continue;
      const key = parts[0].trim();
      const value = parts.slice(1).join(':').trim();
      values[key] = value;
    }

    const httpsEnabled = values.HTTPSEnable === '1';
    const httpsHost = values.HTTPSProxy || '';
    const httpsPort = values.HTTPSPort || '';

    let proxyURL = '';
    if (httpsEnabled && httpsHost && httpsPort) {
      proxyURL = `http://${httpsHost}:${httpsPort}`;
    } else {
      const httpEnabled = values.HTTPEnable === '1';
      const httpHost = values.HTTPProxy || '';
      const httpPort = values.HTTPPort || '';
      if (httpEnabled && httpHost && httpPort) {
        proxyURL = `http://${httpHost}:${httpPort}`;
      }
    }

    if (proxyURL) {
      if (!process.env.HTTPS_PROXY) {
        process.env.HTTPS_PROXY = proxyURL;
      }
      if (!process.env.HTTP_PROXY) {
        process.env.HTTP_PROXY = proxyURL;
      }
    }
  } catch {
    // Best effort only.
  }
}

async function main() {
  applySystemProxyFromMacOS();

  const rootDir = path.resolve(__dirname, '..');
  const pkgPath = path.join(rootDir, 'package.json');
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

  const osName = mapPlatform();
  const arch = mapArch();
  const updateURL = getUpdateCheckURL();
  const payload = buildUpdateCheckRequest(pkg, osName, arch);

  console.log(`Requesting awiki bootstrap metadata from ${updateURL} ...`);
  const metadata = await requestUpdateMetadata(updateURL, payload);
  const artifact = resolveArtifact(metadata, osName, arch);
  const artifactURL = String(artifact.url || '').trim();
  if (!artifactURL) {
    throw new Error('update metadata does not contain an artifact URL');
  }

  const urls = buildArtifactDownloadURLs(artifactURL);
  const fileName = path.basename(new URL(artifactURL).pathname) || `awiki-cli-${osName}-${arch}`;

  const binDir = path.join(rootDir, 'bin');
  ensureDir(binDir);

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'awiki-cli-'));
  const archivePath = path.join(tmpDir, fileName);

  let lastError;
  for (const url of urls) {
    console.log(`Downloading awiki-cli bootstrap binary for ${osName}/${arch} from ${url} ...`);
    try {
      await download(url, archivePath);
      verifySHA256(archivePath, artifact.sha256);
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
    } catch {
      // best effort cleanup
    }
  }

  const exeName = osName === 'windows' ? 'awiki-cli.exe' : 'awiki-cli';
  const exePath = path.join(binDir, exeName);

  if (osName !== 'windows') {
    try {
      fs.chmodSync(exePath, 0o755);
    } catch {
      // best effort
    }
  }

  console.log(`awiki-cli bootstrap binary is installed at ${exePath}`);
}

if (require.main === module) {
  main().catch(err => {
    console.error(`[awiki-cli] Failed to install bootstrap binary: ${err.message}`);
    console.error(
      '\nIf you are behind a firewall or using a restricted network, you can:\n' +
      '  - Set AWIKI_CLI_UPDATE_CHECK_URL or AWIKI_CLI_UPDATE_BASE_URL to a reachable update service,\n' +
      '  - Set AWIKI_CLI_DOWNLOAD_MIRROR to a reachable artifact mirror,\n' +
      '  - Or manually download the archive returned by the update API and extract it into the awiki-cli bin directory.\n'
    );
    process.exit(1);
  });
}

module.exports = {
  _internal: {
    mapPlatform,
    mapArch,
    getVersion,
    deriveBootstrapChannel,
    getUpdateCheckURL,
    buildUpdateCheckRequest,
    resolveArtifact,
    buildArtifactDownloadURLs,
  },
};
