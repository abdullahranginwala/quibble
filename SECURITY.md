# Security Policy

Quibble's core runs entirely locally: `quibble serve` binds 127.0.0.1 only,
writes only inside your repo's `.quibble/`, and never mutates your markdown.
Anything that violates those properties is a security bug.

## Reporting

Please **do not** open a public issue for vulnerabilities. Use GitHub's
private vulnerability reporting ("Report a vulnerability" under this repo's
Security tab). You'll get an acknowledgment within a week; fixes ship as a
patch release with credit unless you prefer otherwise.

## Supported versions

Only the latest minor release receives security fixes.
