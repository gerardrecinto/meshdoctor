// Pipeline for this repo. The mesh gate is a shared library step kept in jenkins/vars.
library identifier: 'meshdoctor-lib@main', retriever: legacySCM([$class: 'GitSCMSource', remote: 'https://github.com/gerardrecinto/meshdoctor.git'])

pipeline {
  agent any
  options { timestamps(); timeout(time: 40, unit: 'MINUTES'); disableConcurrentBuilds(abortPrevious: true) }
  stages {
    stage('Test') {
      parallel {
        stage('Go') { steps { sh 'go vet ./... && go test -race ./...' } }
        stage('Python') { steps { sh 'python3 -m venv .venv && .venv/bin/pip install -q -r python/requirements.txt && cd python && ../.venv/bin/python -m pytest -q' } }
        stage('Java') { steps { sh 'mvn -q -B -f java/checkout/pom.xml test' } }
      }
    }
    stage('Mesh gate') {
      steps {
        sh 'go build -o bin/meshdoctor ./cmd/meshdoctor'
        meshGate(path: 'deploy', strict: true)
      }
    }
    stage('Terraform') {
      steps { sh 'cd terraform/eks && terraform init -backend=false -input=false && terraform validate && terraform fmt -check' }
    }
    stage('Mesh e2e') {
      when { branch 'main' }
      steps { sh 'bash scripts/e2e.sh' }
    }
  }
  post { always { archiveArtifacts artifacts: 'meshdoctor-report.json', allowEmptyArchive: true } }
}
