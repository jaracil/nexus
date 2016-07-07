#!/bin/bash

# Get script directory
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ]; do
  DIR="$( cd -P "$( dirname "$SOURCE" )" && pwd )"
  SOURCE="$(readlink "$SOURCE")"
  [[ $SOURCE != /* ]] && SOURCE="$DIR/$SOURCE" 
located
done
DIR="$( cd -P "$( dirname "$SOURCE" )" && pwd )"

# Log and exit on command error
function x {
	XOUTPUT="$($@)"
	local status=$?
	if [ $status -ne 0 ]; then
		echo "error with $1" >&2
		exit 1
	fi
}

# Pull up-to-date docker image
x docker pull nayarsystems/nexus

# Build nexus to use the binary
x go build ..

# Run nexus in a docker container with the built binary
x docker run -d -p 1717:1717 -p 8888:80 -v $DIR/nexus:/nexus nayarsystems/nexus -l http://0.0.0.0:80 -l tcp://0.0.0.0:1717
CONTAINER_ID=$XOUTPUT

# Wait until nexus responds on http interface (or timeout)
i="0"
while ! curl -s -H "Content-Type: application/json" -X POST -d '{"jsonrpc":"2.0"}' http://localhost:8888 > /dev/null && [ $i -lt 15 ]; do
	sleep 1
	i=$[$i+1]
done

# Exit if timed out
if [ $i -ge 15 ]; then
	echo "timeout waiting nexus to start" >&2
	exit 1
fi

# Execute the tests
go test

# Stop and remove the nexus container
x docker stop $CONTAINER_ID
x docker rm $CONTAINER_ID
