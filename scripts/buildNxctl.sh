#!/bin/bash

source helpers.sh

which go &>/dev/null
if [[ $? != 0 ]]; then
        e Missing go binary in \$PATH
        exit
fi

which git &>/dev/null
if [[ $? != 0 ]]; then
        e Missing git binary in \$PATH
        exit
fi


#
# Nexus stuff
#

s Setting local go environment...

if [[ -e go/ ]]; then
        i Go environment found.
else 
        w Go environment not found. Creating...
        /usr/bin/mkdir -p go/src/github.com/nxctl
fi

export GOPATH=$ABSPATH/go/
i GOPATH=$GOPATH

s Cloning nxctl...

if [[ -e go/src/github.com/nayarsystems/nxctl ]]; then
        pushd . &>/dev/null
        cd go/src/github.com/nayarsystems/nxctl
        i git pull
        git pull
        popd &>/dev/null
else 
        git clone https://github.com/nayarsystems/nxctl.git go/src/github.com/nayarsystems/nxctl/
fi


s Building NXCTL...

pushd . &>/dev/null
cd go/src/github.com/nayarsystems/nxctl

i go get
go get -v -u
if [[ $? != 0 ]]; then
        e Failed
        exit
fi

i go build
go build
if [[ $? != 0 ]]; then
        e Failed
        exit
fi
popd &>/dev/null

cp go/src/github.com/nayarsystems/nxctl/nxctl nxctl

s nxctl built.

if [[ ! -x nxctl ]]; then
        e Uh. Its not here..
        exit
fi
i $(file nxctl)
i $(ls --color=always -l nxctl)
