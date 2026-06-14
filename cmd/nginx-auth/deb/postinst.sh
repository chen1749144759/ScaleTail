if [ "$1" = "configure" ] || [ "$1" = "abort-upgrade" ] || [ "$1" = "abort-deconfigure" ] || [ "$1" = "abort-remove" ] ; then
	  deb-systemd-helper unmask 'scaletail.nginx-auth.socket' >/dev/null || true
	  if deb-systemd-helper --quiet was-enabled 'scaletail.nginx-auth.socket'; then
		    deb-systemd-helper enable 'scaletail.nginx-auth.socket' >/dev/null || true
	  else
		    deb-systemd-helper update-state 'scaletail.nginx-auth.socket' >/dev/null || true
	  fi

    if systemctl is-active scaletail.nginx-auth.socket >/dev/null; then
        systemctl --system daemon-reload >/dev/null || true
        deb-systemd-invoke stop 'scaletail.nginx-auth.service' >/dev/null || true
        deb-systemd-invoke restart 'scaletail.nginx-auth.socket' >/dev/null || true
    fi
fi
