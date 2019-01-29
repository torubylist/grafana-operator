node('tos-builder') {
    properties([buildDiscarder(
            logRotator(artifactDaysToKeepStr: '', artifactNumToKeepStr: '', daysToKeepStr: '60', numToKeepStr: '100')),
                gitLabConnection('gitlab-172.16.1.41'),
                parameters([string(defaultValue: '', description: '', name: 'RELEASE_TAG')]),
                pipelineTriggers([])
    ])


    currentBuild.result = "SUCCESS"
    @Library('jenkins-library') _
    waitDocker {}

    def tag_name = ''
    stage('scm checkout') {
        checkout(scm)
    }

    withEnv([
            'DOCKER_HOST=unix:///var/run/docker.sock',
            'DOCKER_REPO=172.16.1.99',
            'COMPONENT_NAME=grafana-dashboard-operator',
            'DOCKER_PROD_NS=gold',
    ]) {

        try {
            withCredentials([
                    usernamePassword(
                            credentialsId: 'harbor',
                            passwordVariable: 'DOCKER_PASSWD',
                            usernameVariable: 'DOCKER_USER')
            ]) {
                stage('build binary') {
                    sh """#!/bin/bash -ex
                        wget ftp://172.16.1.32/pub/golang/go1.10.4.linux-amd64.tar.gz
                        mkdir -p /root/go1.10.4/
                        tar -xvzf go1.10.4.linux-amd64.tar.gz -C /root/go1.10.4/
                        export GOROOT=/root/go1.10.4/go
                        export PATH=/root/go1.10.4/go/bin:$PATH
                        which go
                        go version
                        mkdir -p $GOPATH/src/github.com/tsloughter/grafana-operator
                        cp -r . $GOPATH/src/github.com/tsloughter/grafana-operator
                        cd $GOPATH/src/github.com/tsloughter/grafana-operator
                        CGO_ENABLED=0  go build -v -i -o ./bin/grafana-dashboard-operator ./cmd
                    """
                }
                stage('ci build') {
                    sh """#!/bin/bash -ex
                      cd $GOPATH/src/github.com/tsloughter/grafana-operator
                      docker login -u \$DOCKER_USER -p \$DOCKER_PASSWD \$DOCKER_REPO
                      REV=\$(git rev-parse HEAD)
                      export DOCKER_IMG_NAME=\$DOCKER_REPO/\$DOCKER_PROD_NS/\$COMPONENT_NAME:$env.BRANCH_NAME
                      docker build --label CODE_REVISION=\${REV} \
                        --label BRANCH=$env.BRANCH_NAME \
                        --label COMPILE_DATE=\$(date +%Y%m%d-%H%M%S) \
                        -t \$DOCKER_IMG_NAME -f Dockerfile .
                      docker push \$DOCKER_IMG_NAME
                    """
                }
            }
        } catch (e) {
            currentBuild.result = "FAILED"
            echo 'Err: Incremental Build failed with Error: ' + e.toString()
            throw e
        } finally {
            sendMail {
                emailRecipients = "tosdev@transwarp.io"
                attachLog = false
            }
        }
    }
}
