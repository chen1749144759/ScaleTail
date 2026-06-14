#!/bin/bash

set -eu

# Clean up folders and files created during build.
function cleanup() {
	rm -rf /ScaleTail/$ARCH
	rm -f /ScaleTail/sed*
	rm -f /ScaleTail/qpkg.cfg
}
trap cleanup EXIT

mkdir -p /ScaleTail/$ARCH
cp /scaletaild /ScaleTail/$ARCH/scaletaild
cp /scaletail /ScaleTail/$ARCH/scaletail

sed "s/\$QPKG_VER/$TSTAG-$QNAPTAG/g" /ScaleTail/qpkg.cfg.in >/ScaleTail/qpkg.cfg

qbuild --root /ScaleTail --build-arch $ARCH --build-dir /out
