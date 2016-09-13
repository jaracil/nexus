#!/bin/bash

source helpers.sh

s Building 'nexus' docker image

cat <<EOF | docker build -t nexus --no-cache -
FROM alpine

ENV GOPATH /go/
RUN apk update &&\
 apk add go git mercurial &&\
 mkdir /go/src/github.com/jaracil/ -p &&\
 git clone https://github.com/jaracil/nexus.git /go/src/github.com/jaracil/nexus &&\
 cd /go/src/github.com/jaracil/nexus &&\
 go get &&\
 go build -o /nexus &&\
 apk del go git mercurial &&\
 rm -fr /go

ENTRYPOINT ["/nexus"]
EOF

s Done
i Run as: docker run --rm -ti nexus -l http://0.0.0.0:80 -r rethinkdb.host:28015

