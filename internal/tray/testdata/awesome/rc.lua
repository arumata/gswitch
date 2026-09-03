local awful = require("awful")
local wibox = require("wibox")

awful.screen.connect_for_each_screen(function(s)
    s.gswitch_test_systray = wibox.widget.systray()
    s.gswitch_test_systray:set_base_size(20)
    s.gswitch_test_bar = awful.wibar({ position = "top", screen = s })

    local right_layout = wibox.layout.fixed.horizontal()
    right_layout:add(wibox.widget.textclock())
    right_layout:add(s.gswitch_test_systray)

    s.gswitch_test_bar:setup({
        layout = wibox.layout.align.horizontal,
        nil,
        nil,
        right_layout,
    })
end)
