#!/bin/sh
# gswitch postremove script
#
# This script is executed after the gswitch package is removed.
# It reloads systemd, udev, and updates icon/desktop caches.
#
# Compatible with: DEB (postrm), RPM (%postun), nFPM (postremove)
#
# Note: Configuration directory /etc/gswitch/ is intentionally preserved
# to allow reinstallation without losing settings. Users can manually
# remove it or use 'dpkg --purge' (DEB) for complete cleanup.

# Reload systemd after unit file removal
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

if command -v loginctl >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
    loginctl list-users --no-legend 2>/dev/null | while read -r uid user _; do
        runtime_dir="/run/user/$uid"
        if [ -S "$runtime_dir/bus" ]; then
            runuser -u "$user" -- env \
                XDG_RUNTIME_DIR="$runtime_dir" \
                DBUS_SESSION_BUS_ADDRESS="unix:path=$runtime_dir/bus" \
                systemctl --user daemon-reload 2>/dev/null || true
        fi
    done
fi

# Reload udev rules after rules file removal
if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules || true
    udevadm trigger --subsystem-match=misc --action=change || true
    udevadm trigger --subsystem-match=input --action=change || true
fi

# Update GTK icon cache after icon removal
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
fi

# Update desktop database after desktop file removal
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications 2>/dev/null || true
fi

exit 0
