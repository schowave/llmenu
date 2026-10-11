# Security policy

## Reporting a vulnerability

Please do not open a public issue for security problems. Report them through
GitHub's private vulnerability reporting instead: **Security → Report a
vulnerability** in this repository.

Include what you found, how to reproduce it and which version or commit is
affected. You will get a response as soon as possible.

## Scope

llmenu handles API keys for LLM gateways. Keys are stored in the operating
system's keyring and passed only to the child process it starts. Anything that
leaks a key (into files, process arguments, logs or other tools' configs) is in
scope.
