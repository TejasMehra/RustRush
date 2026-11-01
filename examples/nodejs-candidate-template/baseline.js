"use strict";
// Node.js baseline for `rustrush candidate --baseline-language nodejs`.
// Contract: dependency-free single file; accepts --iterations N and optional
// --input FILE; prints exactly one deterministic result line on stdout; exits
// non-zero with a message on stderr for invalid arguments.
const fs = require("node:fs");

const MODULUS = 1000000007;

function parseArguments(argv) {
  let iterations = null;
  let input = null;
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--iterations" && index + 1 < argv.length) {
      const text = argv[(index += 1)];
      if (!/^\d+$/.test(text)) throw new Error(`invalid --iterations value: ${text}`);
      iterations = Number(text);
    } else if (argument === "--input" && index + 1 < argv.length) {
      input = argv[(index += 1)];
    } else {
      throw new Error(`unexpected argument: ${argument}`);
    }
  }
  if (iterations === null) throw new Error("--iterations is required");
  return { iterations, input };
}

function main() {
  const { iterations, input } = parseArguments(process.argv.slice(2));
  let value = 0;
  if (input !== null) {
    for (const line of fs.readFileSync(input, "utf8").split(/\r?\n/)) {
      if (line === "") continue;
      if (!/^-?\d+$/.test(line)) throw new Error(`integer input expected, got: ${line}`);
      value += Number(line);
    }
  }
  // All intermediate values stay below 2^53, so Number arithmetic is exact and
  // matches the Rust candidate's i64 arithmetic.
  for (let index = 0; index < iterations; index += 1) {
    value = (value * 31 + (index % 97) * 13 + 7) % MODULUS;
  }
  console.log(String(value));
}

try {
  main();
} catch (error) {
  console.error(error.message);
  process.exit(2);
}
