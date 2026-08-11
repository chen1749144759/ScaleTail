#!/bin/sh
set -eu

usage() {
  cat >&2 <<'EOF'
Usage: scaletail-configure-account --server HTTP_URL_OR_HTTPS_URL --username USERNAME [--accept-routes true|false] [--accept-dns true|false]
EOF
  exit 2
}

fail() {
  echo "scaletail-configure-account: $*" >&2
  exit 1
}

server=""
username=""
accept_routes="false"
accept_dns="true"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --server)
      [ "$#" -ge 2 ] || usage
      server=$2
      shift 2
      ;;
    --username)
      [ "$#" -ge 2 ] || usage
      username=$2
      shift 2
      ;;
    --accept-routes)
      [ "$#" -ge 2 ] || usage
      accept_routes=$2
      shift 2
      ;;
    --accept-dns)
      [ "$#" -ge 2 ] || usage
      accept_dns=$2
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      usage
      ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "must run as root"
case "$server" in
  http://*) authority=${server#http://} ;;
  https://*) authority=${server#https://} ;;
  *) fail "--server must use http:// or https://" ;;
esac
case "$authority" in
  */) authority=${authority%/} ;;
esac
[ -n "$authority" ] || fail "--server must include a host"
case "$authority" in
  *[/?#@]*|*[[:space:]]*) fail "--server must be an origin without credentials, path, query, or fragment" ;;
esac
case "$authority" in
  \[*\]*)
    host=${authority#\[}
    host=${host%%\]*}
    suffix=${authority#*\]}
    [ -n "$host" ] || fail "--server must include a host"
    case "$suffix" in ""|:[0-9]*) ;; *) fail "invalid --server port" ;; esac
    ;;
  *:*:*) fail "IPv6 control hosts must use brackets" ;;
  *:*)
    host=${authority%%:*}
    suffix=${authority#*:}
    [ -n "$host" ] || fail "--server must include a host"
    printf '%s' "$suffix" | grep -Eq '^[0-9]+$' || fail "invalid --server port"
    ;;
  *) host=$authority; suffix="" ;;
esac
case "$suffix" in
  :*) port=${suffix#:} ;;
  "") port="" ;;
  *) port=$suffix ;;
esac
if [ -n "$port" ]; then
  printf '%s' "$port" | grep -Eq '^[0-9]+$' || fail "invalid --server port"
  [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || fail "--server port must be between 1 and 65535"
fi
server=${server%/}
printf '%s' "$username" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9_.@+-]{0,253}$' || fail "invalid --username value"
case "$accept_routes" in true|false) ;; *) fail "--accept-routes must be true or false" ;; esac
case "$accept_dns" in true|false) ;; *) fail "--accept-dns must be true or false" ;; esac
[ -t 0 ] || fail "password input requires an interactive terminal"

password=""
password_tmp=""
config_tmp=""
cleanup() {
  stty echo 2>/dev/null || true
  password=""
  [ -z "$password_tmp" ] || rm -f "$password_tmp"
  [ -z "$config_tmp" ] || rm -f "$config_tmp"
}
trap cleanup EXIT HUP INT TERM
printf 'ScaleForge account password: ' >&2
stty -echo
IFS= read -r password
stty echo
printf '\n' >&2

password_bytes=$(LC_ALL=C printf '%s' "$password" | wc -c | tr -d ' ')
[ "$password_bytes" -ge 1 ] && [ "$password_bytes" -le 72 ] || fail "password must contain between 1 and 72 bytes"

install -d -o root -g root -m 0700 /etc/scaletail
password_tmp=$(mktemp /etc/scaletail/.account-password.XXXXXX)
config_tmp=$(mktemp /etc/scaletail/.account-conf.XXXXXX)
chmod 0600 "$password_tmp" "$config_tmp"
printf '%s' "$password" >"$password_tmp"
cat >"$config_tmp" <<EOF
SCALETAIL_LOGIN_SERVER=$server
SCALETAIL_ACCOUNT_USERNAME=$username
SCALETAIL_ACCOUNT_PASSWORD_FILE=/etc/scaletail/account-password
EOF
chown root:root "$password_tmp" "$config_tmp"

systemctl daemon-reload
systemctl enable --now scaletaild.service
/usr/bin/scaletail up \
  --login-server="$server" \
  --username="$username" \
  --password-file="$password_tmp" \
  --timeout=60s

mv -f "$password_tmp" /etc/scaletail/account-password
password_tmp=""
mv -f "$config_tmp" /etc/scaletail/account.conf
config_tmp=""
systemctl enable scaletail-account-login.service
/usr/bin/scaletail set --accept-routes="$accept_routes" --accept-dns="$accept_dns"

echo "ScaleTail account login configured. Automatic reauthentication will run after scaletaild restarts."
