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

# Stop and disable the legacy system-scope root service during migration, then
# reload systemd so the removed unit is forgotten.
if command -v systemctl >/dev/null 2>&1; then
    # Keep stop and disable separate: `disable --now` can return early when an
    # older package already removed the unit file but systemd still has the
    # root service loaded and running.
    systemctl stop gswitch.service 2>/dev/null || true
    systemctl disable gswitch.service 2>/dev/null || true
    rm -f /etc/systemd/system/gswitch.service \
        /usr/lib/systemd/system/gswitch.service \
        /lib/systemd/system/gswitch.service
    systemctl daemon-reload || true
fi

# Reload udev rules for /dev/uinput access
if command -v udevadm >/dev/null 2>&1; then
    # Remove the old late-sorting rule left by pre-non-root packages or
    # manual installations. uaccess tags must exist before 73-seat-late.rules.
    rm -f /usr/lib/udev/rules.d/90-gswitch.rules \
        /etc/udev/rules.d/90-gswitch.rules
    udevadm control --reload-rules || true
    udevadm trigger --subsystem-match=misc --action=change || true
    udevadm trigger --subsystem-match=input --action=change || true
fi

# Reload active user managers and restart gswitch only where it was already
# running. Do not enable a service the user explicitly disabled.
if command -v loginctl >/dev/null 2>&1 && command -v runuser >/dev/null 2>&1; then
    loginctl list-users --no-legend 2>/dev/null | while read -r uid user _; do
        runtime_dir="/run/user/$uid"
        if [ -S "$runtime_dir/bus" ]; then
            runuser -u "$user" -- env \
                XDG_RUNTIME_DIR="$runtime_dir" \
                DBUS_SESSION_BUS_ADDRESS="unix:path=$runtime_dir/bus" \
                systemctl --user daemon-reload 2>/dev/null || true
            runuser -u "$user" -- env \
                XDG_RUNTIME_DIR="$runtime_dir" \
                DBUS_SESSION_BUS_ADDRESS="unix:path=$runtime_dir/bus" \
                systemctl --user try-restart gswitch.service 2>/dev/null || true
        fi
    done
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
echo "Enable the service once from the graphical user session:"
echo "  systemctl --user enable --now gswitch.service"
echo ""
echo "The tray application will start automatically on next login."
echo "To start it now, run: gswitch-tray"
echo ""

exit 0
