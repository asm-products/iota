#!/bin/bash

set -e

# Expects arguments containing
# 1. the path to the user directory
# 2. the package name
# 3. the filename of the endpointmain.go file

USER_DIR="$1"
PACKAGE_NAME="$2"
ENDPOINTMAIN="$3"

echo "USER_DIR: $USER_DIR"
echo "PACKAGE_NAME: $PACKAGE_NAME"
echo "ENDPOINTMAIN: $ENDPOINTMAIN"

# Make our temporary directory (to act as part of GOPATH)
TMP_DIR=`mktemp -d /tmp/iota.build.XXXXXXXXXX`
EPM_BASENAME=`basename $ENDPOINTMAIN`
mkdir -p "$TMP_DIR/src/$EPM_BASENAME"
mv $ENDPOINTMAIN "$TMP_DIR/src/$EPM_BASENAME/$EPM_BASENAME.go"
unset ENDPOINTMAIN

# Set up the target (where the build output will go)

TARGET_DIR="$USER_DIR/f/$PACKAGE_NAME"
if [ ! -d "$TARGET_DIR" ]; then
	mkdir -p $TARGET_DIR
fi

# Do the build
OUTPUT_FLAG="$TARGET_DIR/endpoint"
export GOPATH="$USER_DIR:$TMP_DIR"
cd "$TMP_DIR/src/$EPM_BASENAME"
go build -o $OUTPUT_FLAG -x
