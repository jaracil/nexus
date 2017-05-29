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
        /usr/bin/mkdir -p go/src/github.com/jaracil/
fi

export GOPATH=$ABSPATH/go/
i GOPATH=$GOPATH

s Cloning Nexus...

if [[ -e go/src/github.com/jaracil/nexus/ ]]; then
        pushd . &>/dev/null
        cd go/src/github.com/jaracil/nexus/
        i git pull
        git pull
        popd &>/dev/null
else 
        git clone https://github.com/jaracil/nexus.git go/src/github.com/jaracil/nexus/
fi


s Building Nexus...

pushd . &>/dev/null
cd go/src/github.com/jaracil/nexus/

i glide install
glide install
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

cp go/src/github.com/jaracil/nexus/nexus .

s Nexus built.

if [[ ! -x nexus ]]; then
        e Uh. Its not here..
        exit
fi
i $(file nexus)
i $(ls --color=always -l nexus)
