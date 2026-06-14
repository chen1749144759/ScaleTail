#!/bin/sh
set -e
if [ -d /run/systemd/system ] ; then
	  systemctl --system daemon-reload >/dev/null || true
fi

if [ -x "/usr/bin/deb-systemd-helper" ]; then
    if [ "$1" = "remove" ]; then
		    deb-systemd-helper mask 'scaletail.nginx-auth.socket' >/dev/null || true
		    deb-systemd-helper mask 'scaletail.nginx-auth.service' >/dev/null || true
	  fi

    if [ "$1" = "purge" ]; then
		    deb-systemd-helper purge 'scaletail.nginx-auth.socket' >/dev/null || true
		    deb-systemd-helper unmask 'scaletail.nginx-auth.socket' >/dev/null || true
		    deb-systemd-helper purge 'scaletail.nginx-auth.service' >/dev/null || true
		    deb-systemd-helper unmask 'scaletail.nginx-auth.service' >/dev/null || true
	  fi
fi
