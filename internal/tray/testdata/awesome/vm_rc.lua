-- Keep the distribution's real Awesome 4.3 configuration and add the two
-- startup commands documented for gswitch. The E2E helper installs this file
-- only for the Awesome matrix row and restores the previous user config.
dofile("/etc/xdg/awesome/rc.lua")

local awful = require("awful")

awful.spawn.with_shell(
    "systemctl --user import-environment DISPLAY XAUTHORITY "
        .. "XDG_CURRENT_DESKTOP XDG_SESSION_DESKTOP DESKTOP_SESSION "
        .. "&& systemctl --user is-enabled --quiet gswitch.service "
        .. "&& systemctl --user start gswitch.service"
)
awful.spawn.with_shell(
    'pgrep -u "$(id -u)" -x gswitch-tray >/dev/null || exec gswitch-tray'
)
