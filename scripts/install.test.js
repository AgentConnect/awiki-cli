'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { _internal } = require('./install.js');

test('deriveBootstrapChannel chooses prerelease for prerelease versions', () => {
  assert.equal(_internal.deriveBootstrapChannel('1.2.3-beta.1'), 'prerelease');
  assert.equal(_internal.deriveBootstrapChannel('1.2.3'), 'stable');
});

test('getUpdateCheckURL prefers explicit URL over base URL', () => {
  const originalExplicit = process.env.AWIKI_CLI_UPDATE_CHECK_URL;
  const originalBase = process.env.AWIKI_CLI_UPDATE_BASE_URL;
  process.env.AWIKI_CLI_UPDATE_CHECK_URL = 'https://updates.example.com/custom';
  process.env.AWIKI_CLI_UPDATE_BASE_URL = 'https://ignored.example.com';
  try {
    assert.equal(_internal.getUpdateCheckURL(), 'https://updates.example.com/custom');
  } finally {
    if (originalExplicit === undefined) delete process.env.AWIKI_CLI_UPDATE_CHECK_URL; else process.env.AWIKI_CLI_UPDATE_CHECK_URL = originalExplicit;
    if (originalBase === undefined) delete process.env.AWIKI_CLI_UPDATE_BASE_URL; else process.env.AWIKI_CLI_UPDATE_BASE_URL = originalBase;
  }
});

test('buildUpdateCheckRequest emits anonymous bootstrap payload', () => {
  const payload = _internal.buildUpdateCheckRequest({ version: '0.0.1-beta.10' }, 'linux', 'amd64');
  assert.deepEqual(payload, {
    schema_version: 1,
    current_version: '0.0.1-beta.10',
    channel: 'prerelease',
    goos: 'linux',
    goarch: 'amd64',
    host_agent: 'unknown',
    host_version: '',
    host_capabilities: [],
    skill_format_version: 'v1',
  });
});

test('resolveArtifact selects matching platform from metadata artifacts', () => {
  const artifact = _internal.resolveArtifact({
    artifacts: [
      { platform: 'darwin-arm64', url: 'https://downloads.example.com/darwin.tar.gz' },
      { platform: 'linux-amd64', url: 'https://downloads.example.com/linux.tar.gz' },
    ],
  }, 'linux', 'amd64');
  assert.equal(artifact.url, 'https://downloads.example.com/linux.tar.gz');
});
