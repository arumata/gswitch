#ifndef GSWITCH_XEMBED_LINUX_H
#define GSWITCH_XEMBED_LINUX_H

#include <gtk/gtk.h>
#include <stdint.h>

typedef struct GSwitchXEmbedTray GSwitchXEmbedTray;

extern void goXEmbedMenuActivated(uintptr_t backend_handle, int item_id);

void gswitch_xembed_free(void *pointer);
void gswitch_xembed_free_string(char *value);
GSwitchXEmbedTray *gswitch_xembed_tray_new(uintptr_t backend_handle);
void gswitch_xembed_tray_free(GSwitchXEmbedTray *tray);
gboolean gswitch_xembed_tray_set_icon(
    GSwitchXEmbedTray *tray,
    const unsigned char *data,
    size_t length
);
void gswitch_xembed_tray_set_title(GSwitchXEmbedTray *tray, const char *title);
void gswitch_xembed_tray_set_tooltip(GSwitchXEmbedTray *tray, const char *tooltip);
GtkWidget *gswitch_xembed_tray_add_item(
    GSwitchXEmbedTray *tray,
    const char *title,
    const char *tooltip,
    int item_id
);
void gswitch_xembed_tray_add_separator(GSwitchXEmbedTray *tray);
void gswitch_xembed_menu_item_set_title(GtkWidget *item, const char *title);
void gswitch_xembed_menu_item_set_enabled(GtkWidget *item, gboolean enabled);

#endif
