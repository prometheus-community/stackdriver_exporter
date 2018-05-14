def label = "stackdriver-exporter-${UUID.randomUUID().toString().substring(0, 5)}"
podTemplate(label: label,
            containers: [containerTemplate(name: 'golang',
                                           image: 'golang:1.10.1-alpine3.7',
                                           resourceRequestCpu: '500m',
                                           resourceLimitCpu: '2000m',
                                           resourceRequestMemory: '500Mi',
                                           resourceLimitMemory: '500Mi',
                                           ttyEnabled: true,
                                           command: '/bin/cat -'),
                         containerTemplate(name: 'docker',
                                           image: 'docker:18.03.0-ce',
                                           command: '/bin/cat -',
                                           resourceRequestCpu: '500m',
                                           resourceLimitCpu: '2000m',
                                           resourceRequestMemory: '500Mi',
                                           resourceLimitMemory: '500Mi',
                                           ttyEnabled: true)],
            volumes: [secretVolume(secretName: 'jenkins-docker-builder',
                                   mountPath: '/jenkins-docker-builder',
                                   readOnly: true),
                      hostPathVolume(hostPath: '/var/run/docker.sock', mountPath: '/var/run/docker.sock')]) {
    node(label) {
        def gitCommit
        def imageName
        container('jnlp') {
            stage('Checkout') {
                checkout(scm)
                gitCommit = sh(returnStdout: true, script: 'git rev-parse --short HEAD').trim()
            }
        }
        container('golang') {
            stage('Build') {
                withEnv(["CGO_ENABLED=0", "GOOS=linux"]) {
                    sh('mkdir -p $GOPATH/src/github.com/frodenas')
                    sh('ln -s $WORKSPACE $GOPATH/src/github.com/frodenas/stackdriver_exporter')
                    sh('cd $GOPATH/src/github.com/frodenas/stackdriver_exporter && go build')
                }
            }
        }
        container('docker') {
            stage('Build container') {
                imageName = "eu.gcr.io/cognitedata/frodenas/stackdriver-exporter:${gitCommit}"
                sh("docker build -t ${imageName} .")
            }
            if (env.BRANCH_NAME == 'master') {
                stage('Push container') {
                    sh('#!/bin/sh -e\n' + 'docker login -u _json_key -p "$(cat /jenkins-docker-builder/credentials.json)" https://eu.gcr.io')
                    sh("docker push ${imageName}")
                }
            }
        }
    }
}
