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

# Reload udev rules after rules file removal
if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules || true
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
