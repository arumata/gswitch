#!/bin/sh
# gswitch preremove script
#
# This script is executed before the gswitch package is removed.
# It stops and disables the service to ensure clean removal.
#
# Compatible with: DEB (prerm), RPM (%preun), nFPM (preremove)
#
# Note: This script uses graceful degradation - commands may fail
# (e.g., if service was never enabled) without causing removal failure.

# Detect whether this is a real uninstall vs an upgrade.
#
# RPM (%preun): $1 == 0 for erase, $1 == 1 for upgrade.
# DEB (prerm): $1 == remove|upgrade|deconfigure|...
is_real_removal=false
case "${1:-}" in
    0 | remove | purge)
        is_real_removal=true
        ;;
esac

# Stop/disable only on real removal to avoid changing enablement on upgrades.
if [ "$is_real_removal" = "true" ] && command -v systemctl >/dev/null 2>&1; then
    systemctl stop gswitch 2>/dev/null || true
    systemctl disable gswitch 2>/dev/null || true
fi

exit 0
