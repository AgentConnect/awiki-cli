#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

function findBinary() {
  const rootDir = path.resolve(__dirname, '..');
  const binDir = path.join(rootDir, 'bin');
  const exeName = process.platform === 'win32' ? 'awiki-cli.exe' : 'awiki-cli';
  return path.join(binDir, exeName);
}

function fileExists(p) {
  try {
    fs.accessSync(p, fs.constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

function getPackageVersion() {
  try {
    const pkg = require(path.resolve(__dirname, '..', 'package.json'));
    return typeof pkg.version === 'string' ? pkg.version : 'unknown';
  } catch {
    return 'unknown';
  }
}

function run() {
  const binPath = findBinary();

  if (!fileExists(binPath)) {
    const version = getPackageVersion();
    console.error(`[awiki-cli] Bootstrap binary not found at ${binPath}.`);
    console.error('[awiki-cli] This npm package only provides the bootstrap installer and wrapper.');
    console.error('[awiki-cli] Reinstall the packaged binary first, for example:');
    console.error('  npm run install-binary');
    console.error('');
    console.error('[awiki-cli] After the bootstrap binary is installed, runtime upgrades are managed by:');
    console.error('  awiki-cli upgrade apply');
    console.error('');
    console.error('If you installed this package globally, rerun the same package manager command (npm/pnpm/yarn) in that environment.');
    console.error(`Current package version: ${version}`);
    process.exit(1);
  }

  const args = process.argv.slice(2);
  const child = spawn(binPath, args, { stdio: 'inherit' });

  child.on('exit', code => {
    process.exit(code ?? 1);
  });

  child.on('error', err => {
    console.error(`[awiki-cli] Failed to start binary: ${err.message}`);
    process.exit(1);
  });
}

if (require.main === module) {
  run();
}

module.exports = {
  _internal: {
    findBinary,
    fileExists,
  },
};
