#!/bin/sh
# Stop the legacy system-scope daemon before an upgrade removes its unit file.
# DEB and RPM both run this hook before unpacking the new package.

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop gswitch.service 2>/dev/null || true
    systemctl disable gswitch.service 2>/dev/null || true
fi

exit 0
