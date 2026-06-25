module main

// version and commit are injectable at build time with V compile-time defines,
// e.g. `v -d looksy_version=v1.0 -d looksy_commit=abc123 .`
const version = $d('looksy_version', 'dev')
const commit = $d('looksy_commit', 'none')
