#!/bin/bash

function s {
        echo
        echo -e ">> $*"
}

function i {
        echo -e " \e[36m::\e[39m $*"
}
function ni {
        echo -e -n " \e[36m::\e[39m $*"
}
function e {
        echo -e " \e[31mEE\e[39m $*"
}
function w {
        echo -e " \e[33m!!\e[39m $*"
}

LOCALPATH=$(dirname $BASH_SOURCE)
cd $LOCALPATH
ABSPATH=$(pwd)
