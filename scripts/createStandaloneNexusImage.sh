#!/bin/bash

source helpers.sh

./buildNexus.sh
./buildNxctl.sh

#
# Docker stuff
#

s Generate Docker image

which docker &>/dev/null
if [[ $? != 0 ]]; then
        e Missing docker binary in \$PATH
        exit
fi

docker version &>/dev/null
if [[ $? != 0 ]]; then
        e Couldnt find a docker daemon running
        exit
fi

ni New docker image name:\ 
read -a iname 

IMAGENAME=${iname[0]}
i Generating $IMAGENAME docker image
sleep 1

DCTX="$(pwd)/$IMAGENAME-docker"
i Docker context for building the image will be stored at $DCTX

mkdir $DCTX &>/dev/null
if [[ ! -e $DCTX ]]; then
	e Failed
	exit
fi

i Creating Dockerfile

cat > $DCTX/Dockerfile <<EOF
FROM rethinkdb

VOLUME /data

# Expose ports.
#   - 8080:  web UI
#   - 28015: process
#   - 29015: cluster

#   - 80:    nexus http/ws
#   - 443:   https/wss
#   - 1717:  tcp
#   - 1718:  tcp+ssl

EXPOSE 8080
EXPOSE 28015
EXPOSE 29015

EXPOSE 80 
EXPOSE 443
EXPOSE 1717
EXPOSE 1718

COPY start.sh /start.sh
COPY nxctl /usr/bin/nxctl
COPY nexus /nexus

ENTRYPOINT ["/start.sh"]
EOF


i Creating entrypoint script
cat > $DCTX/start.sh <<EOF
#!/bin/bash

echo Starting rethinkdb
rethinkdb --bind all -d /data &

echo Waiting for rethink...
while [[ \$(ss -lnt | grep 28015| wc -l) != 1 ]]; do
        sleep 1
done

echo Starting Nexus
sleep 3
/nexus \$*
EOF

chmod +x $DCTX/start.sh
cp nexus $DCTX/ &>/dev/null
cp nxctl $DCTX/ &>/dev/null

i Building \'$IMAGENAME\' image...

docker build -t $IMAGENAME $DCTX/

i Docker image built
s Done
i Run with: docker run -ti $IMAGENAME -l http://0.0.0.0:80 -l tcp://0.0.0.0:1717
i "Use -v <volume>:/data to preserve the rethinkdb data in a docker volume"
