// Shared library step: fail the build when mesh config has errors, and keep the report.
// Usage: meshGate(path: 'deploy', strict: true)
def call(Map cfg = [:]) {
  String path = cfg.path ?: '.'
  String flags = cfg.strict ? '--strict --min-severity warn' : ''
  sh "bin/meshdoctor ${flags} --format json ${path} > meshdoctor-report.json || true"
  archiveArtifacts artifacts: 'meshdoctor-report.json', allowEmptyArchive: true
  sh "bin/meshdoctor ${flags} ${path}"
}
