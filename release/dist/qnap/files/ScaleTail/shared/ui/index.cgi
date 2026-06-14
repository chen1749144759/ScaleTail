#!/bin/sh
CONF=/etc/config/qpkg.conf
QPKG_NAME="ScaleTail"
QPKG_ROOT=$(/sbin/getcfg ${QPKG_NAME} Install_Path -f ${CONF} -d"")
exec "${QPKG_ROOT}/scaletail" --socket=/tmp/scaletail/scaletaild.sock web --cgi --prefix="/cgi-bin/qpkg/ScaleTail/index.cgi/"
