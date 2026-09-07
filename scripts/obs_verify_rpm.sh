#!/bin/sh
# Run only inside the disposable Tumbleweed verification container.
set -eu

gswitch_rpm=$1
gswitch_key=${2-}
test -f "$gswitch_rpm"
if [ -n "$gswitch_key" ]; then
    rpm --import "$gswitch_key"
    rpm --checksig --verbose "$gswitch_rpm" > /tmp/gswitch-signature
    cat /tmp/gswitch-signature
    grep -Eq 'Signature.*: OK' /tmp/gswitch-signature
    if grep -Eq 'NOKEY|NOT OK' /tmp/gswitch-signature; then exit 1; fi
fi
test "$(rpm -qp --qf '%{NAME}' "$gswitch_rpm")" = gswitch
test "$(rpm -qp --qf '%{ARCH}' "$gswitch_rpm")" = x86_64
if [ -n "${GSWITCH_EXPECTED_VERSION-}" ]; then
    test "$(rpm -qp --qf '%{VERSION}' "$gswitch_rpm")" = "$GSWITCH_EXPECTED_VERSION"
    printf 'Verified RPM version: %s\n' "$GSWITCH_EXPECTED_VERSION"
fi
if [ -n "${GSWITCH_EXPECTED_DISTURL-}" ]; then
    test "$(rpm -qp --qf '%{DISTURL}' "$gswitch_rpm")" = "$GSWITCH_EXPECTED_DISTURL"
    printf 'Verified RPM source: %s\n' "$GSWITCH_EXPECTED_DISTURL"
fi
if [ -n "$gswitch_key" ]; then
    zypper --non-interactive install --no-recommends "$gswitch_rpm"
else
    # The local source-build workflow also verifies explicitly unsigned RPMs.
    zypper --non-interactive install --allow-unsigned-rpm --no-recommends "$gswitch_rpm"
fi
rpm -q gswitch
rpm -ql gswitch
rpm -qR gswitch
rpm -V gswitch
test -x /usr/bin/gswitch
test -x /usr/bin/gswitch-tray
test -f /usr/lib/systemd/user/gswitch.service
test ! -e /usr/lib/systemd/system/gswitch.service
test -f /usr/lib/udev/rules.d/70-gswitch.rules
ldd /usr/bin/gswitch > /tmp/daemon-ldd
ldd /usr/bin/gswitch-tray > /tmp/tray-ldd
cat /tmp/daemon-ldd /tmp/tray-ldd
if grep 'not found' /tmp/daemon-ldd /tmp/tray-ldd; then exit 1; fi
/usr/bin/gswitch --version
sha256sum /usr/bin/gswitch /usr/bin/gswitch-tray
printf '\n# OBS config preservation probe\n' >> /etc/gswitch/default.conf
gswitch_config_digest=$(sha256sum < /etc/gswitch/default.conf)
rpm -U --replacepkgs "$gswitch_rpm"
test "$gswitch_config_digest" = "$(sha256sum < /etc/gswitch/default.conf)"
rpm -e gswitch
test ! -e /usr/bin/gswitch
test ! -e /usr/bin/gswitch-tray
test ! -e /usr/lib/systemd/user/gswitch.service
test "$gswitch_config_digest" = "$(sha256sum < /etc/gswitch/default.conf.rpmsave)"
