# Security policy

## Supported scope

RustRush is an early-stage local CLI. Please report suspected vulnerabilities privately to the repository owner rather than posting exploit details publicly.

## Current security properties

- `analyze` reads supported source files as text and does not execute project code, read repository instructions as commands, or contact remote services.
- Analysis excludes common dependency/build directories, symlink entries, and files or directories with credential-like names. It caps the amount of source data read.
- Reports do not include source contents, environment variable values, or absolute project paths.
- `demo` runs only the repository's built-in workload implementations. It invokes subprocesses without a shell, passes a restricted environment, and applies timeouts.
- Rust compilation output and candidate binaries are placed in a temporary directory that is removed when the command completes.

## Limitations

A temporary directory and timeout are not a security sandbox and do not provide CPU, memory, filesystem, or network isolation. The current CLI does not execute scanned projects or arbitrary generated candidates. Do not adapt the demo to run untrusted code without a separate threat model and an appropriate sandbox. Windows process termination and resource isolation may have platform-specific limitations.

The scanner uses filename and directory heuristics to avoid credentials; it is not a secret-scanning product. Review reports before sharing them.

## Reporting

Include the RustRush version, operating system, affected command, and a concise impact description. Do not include live credentials or private source code in the report.
