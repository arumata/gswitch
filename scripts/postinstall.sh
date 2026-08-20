#!/bin/sh
# gswitch postinstall script
#
# This script is executed after the gswitch package is installed.
# It reloads systemd, udev, and updates icon/desktop caches.
#
# Compatible with: DEB (postinst), RPM (%post), nFPM (postinstall)
#
# Note: This script uses graceful degradation - commands may fail
# (e.g., on systems without systemd) without causing installation failure.

# Reload systemd to pick up the new unit file
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

# Reload udev rules for /dev/uinput access
if command -v udevadm >/dev/null 2>&1; then
    udevadm control --reload-rules || true
    udevadm trigger --subsystem-match=misc --action=change || true
fi

# Update GTK icon cache for tray application
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
fi

# Update desktop database for tray application
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database /usr/share/applications 2>/dev/null || true
fi

# Print post-installation message
echo ""
echo "gswitch installed successfully."
echo ""
echo "To start the service:"
echo "  sudo systemctl enable --now gswitch"
echo ""
echo "The tray application will start automatically on next login."
echo "To start it now, run: gswitch-tray"
echo ""

exit 0
