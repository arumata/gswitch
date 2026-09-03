#include "xembed_linux.h"

#include <stdlib.h>

void gswitch_xembed_free(void *pointer) {
    free(pointer);
}

void gswitch_xembed_free_string(char *value) {
    free(value);
}

struct GSwitchXEmbedTray {
    GtkStatusIcon *icon;
    GtkWidget *menu;
    uintptr_t backend_handle;
};

typedef struct {
    uintptr_t backend_handle;
    int item_id;
} GSwitchXEmbedMenuSignal;

static void gswitch_xembed_popup_menu(
    GtkStatusIcon *icon,
    guint button,
    guint activate_time,
    gpointer user_data
) {
    GSwitchXEmbedTray *tray = user_data;
    gtk_menu_popup(
        GTK_MENU(tray->menu),
        NULL,
        NULL,
        gtk_status_icon_position_menu,
        icon,
        button,
        activate_time
    );
}

static void gswitch_xembed_activate(GtkStatusIcon *icon, gpointer user_data) {
    gswitch_xembed_popup_menu(icon, 1, gtk_get_current_event_time(), user_data);
}

static void gswitch_xembed_item_activate(GtkMenuItem *item, gpointer user_data) {
    (void)item;
    GSwitchXEmbedMenuSignal *signal = user_data;
    goXEmbedMenuActivated(signal->backend_handle, signal->item_id);
}

static void gswitch_xembed_signal_free(gpointer data, GClosure *closure) {
    (void)closure;
    g_free(data);
}

GSwitchXEmbedTray *gswitch_xembed_tray_new(uintptr_t backend_handle) {
    GSwitchXEmbedTray *tray = g_new0(GSwitchXEmbedTray, 1);
    tray->backend_handle = backend_handle;
    tray->menu = gtk_menu_new();
    if (tray->menu == NULL) {
        gswitch_xembed_tray_free(tray);
        return NULL;
    }
    return tray;
}

void gswitch_xembed_tray_free(GSwitchXEmbedTray *tray) {
    if (tray == NULL) {
        return;
    }
    if (tray->icon != NULL) {
        gtk_status_icon_set_visible(tray->icon, FALSE);
        g_object_unref(tray->icon);
    }
    if (tray->menu != NULL) {
        gtk_widget_destroy(tray->menu);
    }
    g_free(tray);
}

gboolean gswitch_xembed_tray_set_icon(
    GSwitchXEmbedTray *tray,
    const unsigned char *data,
    size_t length
) {
    if (tray == NULL || data == NULL || length == 0) {
        return FALSE;
    }

    GError *error = NULL;
    GdkPixbufLoader *loader = gdk_pixbuf_loader_new();
    gboolean written = gdk_pixbuf_loader_write(loader, data, length, &error);
    gboolean closed = written && gdk_pixbuf_loader_close(loader, &error);
    GdkPixbuf *pixbuf = closed ? gdk_pixbuf_loader_get_pixbuf(loader) : NULL;
    if (pixbuf != NULL) {
        if (tray->icon == NULL) {
            tray->icon = gtk_status_icon_new_from_pixbuf(pixbuf);
            g_signal_connect(
                tray->icon,
                "popup-menu",
                G_CALLBACK(gswitch_xembed_popup_menu),
                tray
            );
            g_signal_connect(
                tray->icon,
                "activate",
                G_CALLBACK(gswitch_xembed_activate),
                tray
            );
        } else {
            gtk_status_icon_set_from_pixbuf(tray->icon, pixbuf);
        }
    }
    if (error != NULL) {
        g_error_free(error);
    }
    g_object_unref(loader);
    return pixbuf != NULL;
}

void gswitch_xembed_tray_set_title(GSwitchXEmbedTray *tray, const char *title) {
    if (tray != NULL && tray->icon != NULL) {
        gtk_status_icon_set_title(tray->icon, title);
    }
}

void gswitch_xembed_tray_set_tooltip(GSwitchXEmbedTray *tray, const char *tooltip) {
    if (tray != NULL && tray->icon != NULL) {
        gtk_status_icon_set_tooltip_text(tray->icon, tooltip);
    }
}

GtkWidget *gswitch_xembed_tray_add_item(
    GSwitchXEmbedTray *tray,
    const char *title,
    const char *tooltip,
    int item_id
) {
    if (tray == NULL || tray->menu == NULL) {
        return NULL;
    }

    GtkWidget *item = gtk_menu_item_new_with_label(title);
    if (tooltip != NULL && tooltip[0] != '\0') {
        gtk_widget_set_tooltip_text(item, tooltip);
    }
    GSwitchXEmbedMenuSignal *signal = g_new(GSwitchXEmbedMenuSignal, 1);
    signal->backend_handle = tray->backend_handle;
    signal->item_id = item_id;
    g_signal_connect_data(
        item,
        "activate",
        G_CALLBACK(gswitch_xembed_item_activate),
        signal,
        gswitch_xembed_signal_free,
        0
    );
    gtk_menu_shell_append(GTK_MENU_SHELL(tray->menu), item);
    gtk_widget_show(item);
    return item;
}

void gswitch_xembed_tray_add_separator(GSwitchXEmbedTray *tray) {
    if (tray == NULL || tray->menu == NULL) {
        return;
    }
    GtkWidget *separator = gtk_separator_menu_item_new();
    gtk_menu_shell_append(GTK_MENU_SHELL(tray->menu), separator);
    gtk_widget_show(separator);
}

void gswitch_xembed_menu_item_set_title(GtkWidget *item, const char *title) {
    if (item != NULL) {
        gtk_menu_item_set_label(GTK_MENU_ITEM(item), title);
    }
}

void gswitch_xembed_menu_item_set_enabled(GtkWidget *item, gboolean enabled) {
    if (item != NULL) {
        gtk_widget_set_sensitive(item, enabled);
    }
}
