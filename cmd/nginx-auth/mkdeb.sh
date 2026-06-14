#!/usr/bin/env bash

set -e

VERSION=0.1.3
for ARCH in amd64 arm64; do
    CGO_ENABLED=0 GOARCH=${ARCH} GOOS=linux go build -o scaletail.nginx-auth .

    mkpkg \
        --out=tailscale-nginx-auth-${VERSION}-${ARCH}.deb \
        --name=tailscale-nginx-auth \
        --version=${VERSION} \
        --type=deb \
        --arch=${ARCH} \
        --postinst=deb/postinst.sh \
        --postrm=deb/postrm.sh \
        --prerm=deb/prerm.sh \
        --description="Tailscale NGINX authentication protocol handler" \
        --files=./scaletail.nginx-auth:/usr/sbin/scaletail.nginx-auth,./scaletail.nginx-auth.socket:/lib/systemd/system/scaletail.nginx-auth.socket,./scaletail.nginx-auth.service:/lib/systemd/system/scaletail.nginx-auth.service,./README.md:/usr/share/scaletail/nginx-auth/README.md

    mkpkg \
        --out=tailscale-nginx-auth-${VERSION}-${ARCH}.rpm \
        --name=tailscale-nginx-auth \
        --version=${VERSION} \
        --type=rpm \
        --arch=${ARCH} \
        --postinst=rpm/postinst.sh \
        --postrm=rpm/postrm.sh \
        --prerm=rpm/prerm.sh \
        --description="Tailscale NGINX authentication protocol handler" \
        --files=./scaletail.nginx-auth:/usr/sbin/scaletail.nginx-auth,./scaletail.nginx-auth.socket:/lib/systemd/system/scaletail.nginx-auth.socket,./scaletail.nginx-auth.service:/lib/systemd/system/scaletail.nginx-auth.service,./README.md:/usr/share/scaletail/nginx-auth/README.md
done
