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

s Building Nexus from parent folder...
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

cd $DIR/..

i GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' 
GO111MODULE=on go build
if [[ $? != 0 ]]; then
        e Failed
        exit
fi

cd $DIR
cp $DIR/../nexus .

s Nexus built.

if [[ ! -x nexus ]]; then
        e Uh. Its not here..
        exit
fi
i $(file nexus)
i $(ls --color=always -l nexus)
