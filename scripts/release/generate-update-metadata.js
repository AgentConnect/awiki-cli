#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

function getPackageVersion(rootDir) {
  const pkg = JSON.parse(fs.readFileSync(path.join(rootDir, 'package.json'), 'utf8'));
  if (!pkg.version || typeof pkg.version !== 'string') {
    throw new Error('package.json version is missing');
  }
  return pkg.version.trim();
}

function deriveChannel(version) {
  return version.includes('-') ? 'prerelease' : 'stable';
}

function releaseBaseURL() {
  return (process.env.AWIKI_CLI_RELEASE_DOWNLOAD_BASE || 'https://github.com/AgentConnect/awiki-cli/releases/download').replace(/\/+$/, '');
}

function tagForVersion(version) {
  return `v${version}`;
}

function checksumMap(checksumFile) {
  const raw = fs.readFileSync(checksumFile, 'utf8');
  const result = new Map();
  for (const line of raw.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const parts = trimmed.split(/\s+/);
    if (parts.length < 2) continue;
    result.set(parts[1], parts[0]);
  }
  return result;
}

function artifactEntries(distDir, version) {
  const checksumFile = path.join(distDir, `awiki-cli-${version}-checksums.txt`);
  if (!fs.existsSync(checksumFile)) {
    throw new Error(`checksum file not found: ${checksumFile}`);
  }
  const checksums = checksumMap(checksumFile);
  const entries = [];
  for (const [fileName, sha] of checksums.entries()) {
    if (!/^awiki-cli-.*\.(tar\.gz|zip)$/.test(fileName)) {
      continue;
    }
    const fullPath = path.join(distDir, fileName);
    if (!fs.existsSync(fullPath)) {
      continue;
    }
    const match = fileName.match(/^awiki-cli-(.+?)-(darwin|linux|windows)-(amd64|arm64)\.(tar\.gz|zip)$/);
    if (!match) {
      continue;
    }
    const [, fileVersion, goos, goarch] = match;
    if (fileVersion !== version) {
      continue;
    }
    const stats = fs.statSync(fullPath);
    entries.push({
      platform: `${goos}-${goarch}`,
      url: `${releaseBaseURL()}/${tagForVersion(version)}/${fileName}`,
      sha256: sha,
      size: stats.size,
      signature: '',
    });
  }
  entries.sort((a, b) => a.platform.localeCompare(b.platform));
  return entries;
}

function sha256File(filePath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filePath));
  return hash.digest('hex');
}

function skillBundleMetadata(publicSkillDir, version) {
  const rootSkillPath = path.join(publicSkillDir, 'awiki-cli', 'SKILL.md');
  const skillManifestPath = path.join(publicSkillDir, 'skill-manifest.json');
  if (!fs.existsSync(rootSkillPath)) {
    throw new Error(`root skill file not found: ${rootSkillPath}`);
  }
  if (!fs.existsSync(skillManifestPath)) {
    throw new Error(`skill manifest file not found: ${skillManifestPath}`);
  }
  const files = [];
  function walk(currentDir) {
    for (const entry of fs.readdirSync(currentDir, { withFileTypes: true })) {
      const fullPath = path.join(currentDir, entry.name);
      if (entry.isDirectory()) {
        walk(fullPath);
      } else {
        files.push(path.relative(publicSkillDir, fullPath));
      }
    }
  }
  walk(publicSkillDir);
  files.sort();
  const bundleHash = crypto.createHash('sha256');
  for (const relativePath of files) {
    bundleHash.update(relativePath);
    bundleHash.update(':');
    bundleHash.update(sha256File(path.join(publicSkillDir, relativePath)));
    bundleHash.update('\n');
  }
  return {
    bundle_version: version,
    bundle_sha256: bundleHash.digest('hex'),
    root_skill_sha256: sha256File(rootSkillPath),
  };
}

function generateMetadata({ rootDir, distDir, publicSkillDir, version, publishedAt }) {
  const entries = artifactEntries(distDir, version);
  if (entries.length === 0) {
    throw new Error('no artifacts found in dist/ for update metadata generation');
  }
  return {
    schema_version: 1,
    channel: deriveChannel(version),
    latest_version: version,
    min_supported_version: process.env.AWIKI_CLI_MIN_SUPPORTED_VERSION || version,
    published_at: publishedAt,
    artifacts: entries,
    skill_bundle: skillBundleMetadata(publicSkillDir, version),
  };
}

function main() {
  const rootDir = path.resolve(__dirname, '..', '..');
  const distDir = path.join(rootDir, 'dist');
  const publicSkillDir = path.join(distDir, 'public-skill');
  const version = getPackageVersion(rootDir);
  const metadata = generateMetadata({
    rootDir,
    distDir,
    publicSkillDir,
    version,
    publishedAt: new Date().toISOString(),
  });
  const outputPath = path.join(distDir, 'update-metadata.json');
  fs.writeFileSync(outputPath, JSON.stringify(metadata, null, 2));
  console.log(`Generated update metadata at ${outputPath}`);
}

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(`[awiki-cli] Failed to generate update metadata: ${err.message}`);
    process.exit(1);
  }
}

module.exports = {
  _internal: {
    deriveChannel,
    tagForVersion,
    checksumMap,
    artifactEntries,
    skillBundleMetadata,
    generateMetadata,
  },
};
