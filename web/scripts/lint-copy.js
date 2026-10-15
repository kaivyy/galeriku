#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Target directory to scan (defaults to web/src)
const targetDir = process.argv[2]
  ? path.resolve(process.cwd(), process.argv[2])
  : path.resolve(__dirname, '../src');

const VALID_EXTENSIONS = new Set(['.ts', '.tsx', '.js', '.jsx', '.json', '.mjs', '.cjs']);

function getSourceFiles(dir) {
  if (!fs.existsSync(dir)) {
    return [];
  }
  const entries = fs.readdirSync(dir, { withFileTypes: true, recursive: true });
  const files = [];
  for (const entry of entries) {
    if (entry.isFile()) {
      const ext = path.extname(entry.name);
      if (VALID_EXTENSIONS.has(ext)) {
        const parent = entry.parentPath || entry.path || dir;
        files.push(path.join(parent, entry.name));
      }
    }
  }
  return files.sort();
}

/**
 * Antislop Hard Gate R-02 carve-out:
 * Code/comments that define the R-02 ban, validate it, or quote the character in error messages are exempt.
 */
function isCarveOut(line) {
  return (
    line.includes('R-02') ||
    line.includes('validateAntislopCopy') ||
    line.includes("text.includes('—')") ||
    line.includes('forbidden em dash') ||
    line.includes('antislop-carveout')
  );
}

function lintCopy() {
  console.log(`[lint-copy] Scanning source files in: ${targetDir}`);
  const files = getSourceFiles(targetDir);

  if (files.length === 0) {
    console.log(`[lint-copy] No source files found to inspect.`);
    process.exit(0);
  }

  let violationCount = 0;
  const inspectedFiles = [];

  for (const filePath of files) {
    const relativePath = path.relative(process.cwd(), filePath);
    inspectedFiles.push(relativePath);
    console.log(`[lint-copy] Inspecting: ${relativePath}`);

    const content = fs.readFileSync(filePath, 'utf-8');
    const lines = content.split('\n');

    lines.forEach((line, index) => {
      if (line.includes('—') && !isCarveOut(line)) {
        violationCount++;
        console.error(
          `[lint-copy] Violation in ${relativePath}:${index + 1}: Forbidden em dash ('—') found per Antislop Hard Gate R-02.\n  > ${line.trim()}`
        );
      }
    });
  }

  console.log(`\n[lint-copy] Summary: ${inspectedFiles.length} file(s) inspected.`);

  if (violationCount > 0) {
    console.error(`[lint-copy] FAILED: ${violationCount} forbidden em dash violation(s) found.`);
    process.exit(1);
  } else {
    console.log(`[lint-copy] PASSED: All files are clean and comply with Antislop Hard Gate R-02.`);
    process.exit(0);
  }
}

lintCopy();
