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

A temporary directory and timeout (the default `process` mode) are not a security sandbox and do not provide CPU, memory, filesystem, or network isolation. The `candidate` command with `--isolation container` runs builds and processes in hardened Docker containers (no network, no capabilities, read-only root, non-root, resource limits, no Docker socket, sanitized env) and fails closed rather than falling back to local execution. Its runtime properties are probed in the CI `container-isolation` job, but containers share the host kernel: this is not a VM-grade boundary, a kernel or runtime vulnerability could defeat it, and Docker Desktop is untested. Anyone who can use the Docker daemon is already highly privileged on the host. Do not run untrusted code without your own threat model. Windows process termination and resource isolation may have platform-specific limitations.

The scanner uses filename and directory heuristics to avoid credentials; it is not a secret-scanning product. Review reports before sharing them.

## Reporting

Include the RustRush version, operating system, affected command, and a concise impact description. Do not include live credentials or private source code in the report.

Container mode uses one fresh hardened `docker run --rm` per build, per-implementation warm-up/correctness run, and measured invocation. A timed-out run is force-removed; cleanup errors are surfaced and invalidate the comparison. The Docker socket is never mounted. Each workload container has a fresh writable `/tmp` tmpfs, working directory, `HOME`, and `TMPDIR`, so its process-created files cannot persist into later repetitions. This does not reset host page caches, kernel state, or external machine state. The timer helper measures the workload child and excludes lifecycle/dispatch from sample durations; full invocation wall time is measured and residual overhead is reported as an estimate. Container mode is intended for code the user has reviewed, not as a security sandbox.
