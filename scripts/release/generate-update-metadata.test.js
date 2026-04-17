'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { _internal } = require('./generate-update-metadata.js');

test('generateMetadata builds canonical metadata payload', () => {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'awiki-cli-release-'));
  const distDir = path.join(tempRoot, 'dist');
  const publicSkillDir = path.join(distDir, 'public-skill');
  fs.mkdirSync(path.join(publicSkillDir, 'awiki-cli'), { recursive: true });

  const archivePath = path.join(distDir, 'awiki-cli-1.2.3-linux-amd64.tar.gz');
  fs.mkdirSync(distDir, { recursive: true });
  fs.writeFileSync(archivePath, 'artifact');
  const checksum = '0123456789abcdef';
  fs.writeFileSync(path.join(distDir, 'awiki-cli-1.2.3-checksums.txt'), `${checksum}  awiki-cli-1.2.3-linux-amd64.tar.gz\n`);
  fs.writeFileSync(path.join(publicSkillDir, 'awiki-cli', 'SKILL.md'), 'root skill');
  fs.writeFileSync(path.join(publicSkillDir, 'skill-manifest.json'), '{}');

  const metadata = _internal.generateMetadata({
    rootDir: tempRoot,
    distDir,
    publicSkillDir,
    version: '1.2.3',
    publishedAt: '2026-04-18T00:00:00Z',
  });

  assert.equal(metadata.channel, 'stable');
  assert.equal(metadata.latest_version, '1.2.3');
  assert.equal(metadata.min_supported_version, '1.2.3');
  assert.equal(metadata.published_at, '2026-04-18T00:00:00Z');
  assert.equal(metadata.artifacts.length, 1);
  assert.equal(metadata.artifacts[0].platform, 'linux-amd64');
  assert.equal(metadata.artifacts[0].sha256, checksum);
  assert.ok(metadata.skill_bundle.root_skill_sha256);
  assert.ok(metadata.skill_bundle.bundle_sha256);
});
