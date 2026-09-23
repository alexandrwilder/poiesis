/* The Linux host (docs/ARCHITECTURE.md): one window with the camera drawn by the graphics chip
 * behind a VTE text view whose ground is see-through, and the core running in that view. It
 * owns the camera and the recording and speaks docs/HOST.md with the core over a socket. It
 * holds no log logic. */
#include <gst/gst.h>
#include <gtk/gtk.h>
#include <string.h>
#include <vte/vte.h>

#include "camera.h"
#include "link.h"
#include "picture.h"

#define FOREGROUND "#E6E4DC"

typedef struct {
    GtkApplication *app;
    GtkWindow *window;
    VteTerminal *text;
    Camera *camera;
    Link *link;
    char *socket;
    char **core; /* the program and its arguments */
} Host;

/* MARK: the contract */

static void send_to_core(JsonObject *message, gpointer data) {
    link_send(((Host *)data)->link, message);
}

static gboolean speaks_version_one(JsonObject *m) {
    JsonNode *node = json_object_get_member(m, "versions");
    if (!node || !JSON_NODE_HOLDS_ARRAY(node)) return FALSE;
    JsonArray *versions = json_node_get_array(node);
    for (guint i = 0; i < json_array_get_length(versions); i++) {
        JsonNode *v = json_array_get_element(versions, i);
        if (JSON_NODE_HOLDS_VALUE(v) && json_node_get_int(v) == 1) return TRUE;
    }
    return FALSE;
}

/* The look's twelve numbers, when the message has all of them. */
static gboolean read_matrix(JsonObject *m, double out[12]) {
    JsonNode *node = json_object_get_member(m, "matrix");
    if (!node || !JSON_NODE_HOLDS_ARRAY(node)) return FALSE;
    JsonArray *values = json_node_get_array(node);
    if (json_array_get_length(values) != 12) return FALSE;
    for (guint i = 0; i < 12; i++) {
        JsonNode *v = json_array_get_element(values, i);
        if (!JSON_NODE_HOLDS_VALUE(v)) return FALSE;
        out[i] = json_node_get_double(v);
    }
    return TRUE;
}

static void on_message(JsonObject *m, gpointer data) {
    Host *h = data;
    const char *type = json_object_get_string_member_with_default(m, "t", "");
    if (g_strcmp0(type, "hello") == 0) {
        JsonObject *o = json_object_new();
        json_object_set_string_member(o, "t", "hello");
        json_object_set_int_member(o, "version", speaks_version_one(m) ? 1 : 0);
        json_object_set_string_member(o, "host", "poiesis-linux 0.1");
        JsonArray *can = json_array_new();
        const char *abilities[] = {"picture", "look", "record", "level"};
        for (size_t i = 0; i < G_N_ELEMENTS(abilities); i++) json_array_add_string_element(can, abilities[i]);
        json_object_set_array_member(o, "can", can);
        link_send(h->link, o);
    } else if (g_strcmp0(type, "picture") == 0) {
        double matrix[12];
        gboolean has_matrix = read_matrix(m, matrix);
        camera_set_picture(h->camera, json_object_get_boolean_member_with_default(m, "on", FALSE),
                           has_matrix ? matrix : NULL, (int)json_object_get_int_member_with_default(m, "fps", 15));
    } else if (g_strcmp0(type, "record") == 0) {
        const char *action = json_object_get_string_member_with_default(m, "action", "");
        const char *file = json_object_get_string_member_with_default(m, "file", NULL);
        if (g_strcmp0(action, "start") == 0 && file) camera_start_recording(h->camera, file);
        if (g_strcmp0(action, "stop") == 0) camera_stop_recording(h->camera);
    }
    /* anything else is from a later version: ignored */
}

/* MARK: the core */

static char *here(void) {
    char *self = g_file_read_link("/proc/self/exe", NULL);
    char *dir = self ? g_path_get_dirname(self) : g_get_current_dir();
    g_free(self);
    return dir;
}

static void on_core_exited(VteTerminal *text, int status, Host *h) {
    (void)text;
    (void)status;
    g_application_quit(G_APPLICATION(h->app)); /* the core ended: so does the window */
}

static void on_spawned(VteTerminal *text, GPid pid, GError *error, gpointer data) {
    (void)pid;
    (void)data;
    if (!error) return;
    char *message = g_strdup_printf("\r\n  The core could not start: %s\r\n", error->message);
    vte_terminal_feed(text, message, -1);
    g_free(message);
}

/* The core runs in the text view with the socket's path and the app's own tools on PATH
 * (docs/HOST.md); VTE adds these to the desktop's environment and sets TERM itself. */
static void start_core(Host *h) {
    char *dir = here();
    char *tools = g_build_filename(dir, "tools", NULL);
    const char *path = g_getenv("PATH");
    char **env = NULL;
    if (g_file_test(tools, G_FILE_TEST_IS_DIR)) {
        char *joined = g_strconcat(tools, ":", path ? path : "/usr/bin:/bin", NULL);
        env = g_environ_setenv(env, "PATH", joined, TRUE);
        g_free(joined);
    }
    env = g_environ_setenv(env, "COLORTERM", "truecolor", TRUE);
    env = g_environ_setenv(env, "TERM_PROGRAM", "PoiesisHost", TRUE);
    if (h->link) env = g_environ_setenv(env, "POIESIS_HOST", h->socket, TRUE);
    if (!h->core) {
        char *core = g_build_filename(dir, "poiesis", NULL);
        h->core = g_new0(char *, 3);
        h->core[0] = core;
        h->core[1] = g_strdup("ui");
    }
    vte_terminal_spawn_async(h->text, VTE_PTY_DEFAULT, g_get_home_dir(), h->core, env, G_SPAWN_DEFAULT, NULL, NULL,
                             NULL, -1, NULL, on_spawned, h);
    g_strfreev(env);
    g_free(tools);
    g_free(dir);
}

/* MARK: the window */

/* A tiling desktop (Omarchy's Hyprland, sway, niri) frames windows itself; elsewhere the window
 * keeps the desktop's own title bar. */
static gboolean tiling_desktop(void) {
    const char *desktop = g_getenv("XDG_CURRENT_DESKTOP");
    if (!desktop) return FALSE;
    char *lower = g_ascii_strdown(desktop, -1);
    gboolean tiling = strstr(lower, "hyprland") || strstr(lower, "sway") || strstr(lower, "niri") || strstr(lower, "river");
    g_free(lower);
    return tiling;
}

static void make_window(Host *h) {
    GtkCssProvider *css = gtk_css_provider_new();
    /* black where nothing is drawn; the text view drops the theme's own ground, so the camera
     * shows through wherever the core draws no background */
    gtk_css_provider_load_from_string(css, "window.poiesis { background: black; }"
                                           " window.poiesis vte-terminal { background: none; }");
    gtk_style_context_add_provider_for_display(gdk_display_get_default(), GTK_STYLE_PROVIDER(css),
                                               GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    g_object_unref(css);

    h->window = GTK_WINDOW(gtk_application_window_new(h->app));
    gtk_window_set_title(h->window, "Poiesis");
    gtk_window_set_default_size(h->window, 1100, 720);
    gtk_widget_add_css_class(GTK_WIDGET(h->window), "poiesis");
    if (tiling_desktop()) gtk_window_set_decorated(h->window, FALSE);

    GtkWidget *picture = poiesis_picture_new();
    h->text = VTE_TERMINAL(vte_terminal_new());
    PangoFontDescription *font = pango_font_description_from_string("Monospace 12");
    vte_terminal_set_font(h->text, font);
    pango_font_description_free(font);
    GdkRGBA foreground, ground = {0, 0, 0, 0}; /* the camera shows wherever the core draws no background */
    gdk_rgba_parse(&foreground, FOREGROUND);
    vte_terminal_set_colors(h->text, &foreground, &ground, NULL, 0);
    vte_terminal_set_scrollback_lines(h->text, 0); /* the core owns the whole screen */
    g_signal_connect(h->text, "child-exited", G_CALLBACK(on_core_exited), h);

    GtkWidget *overlay = gtk_overlay_new();
    gtk_overlay_set_child(GTK_OVERLAY(overlay), picture);
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay), GTK_WIDGET(h->text));
    gtk_window_set_child(h->window, overlay);
    h->camera = camera_new(POIESIS_PICTURE(picture), send_to_core, h);
    gtk_window_present(h->window);
    gtk_widget_grab_focus(GTK_WIDGET(h->text));
}

static void on_activate(GtkApplication *app, Host *h) {
    (void)app;
    if (h->window) { /* opened again: the one window comes forward */
        gtk_window_present(h->window);
        return;
    }
    make_window(h);
    GError *error = NULL;
    h->link = link_start(h->socket, on_message, h, &error);
    if (!h->link) {
        g_printerr("Poiesis host: no socket (%s); the core will run on its own\n", error ? error->message : "?");
        g_clear_error(&error);
    }
    start_core(h);
}

static void on_shutdown(GApplication *app, Host *h) {
    (void)app;
    if (h->camera) camera_free(h->camera); /* a recording in progress is finished first */
    h->camera = NULL;
    link_stop(h->link);
    h->link = NULL;
}

int main(int argc, char **argv) {
    Host h = {0};
    gst_init(NULL, NULL);
    GApplicationFlags flags = G_APPLICATION_DEFAULT_FLAGS;
    /* --core <program> [-- arguments]: the contract test runs its fake core in place of the
     * real one, as its own instance beside any running app */
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--core") != 0 || i + 1 >= argc) continue;
        GPtrArray *core = g_ptr_array_new();
        g_ptr_array_add(core, g_strdup(argv[i + 1]));
        for (int j = i + 2; j < argc; j++) {
            if (strcmp(argv[j], "--") != 0) continue;
            for (int k = j + 1; k < argc; k++) g_ptr_array_add(core, g_strdup(argv[k]));
            break;
        }
        g_ptr_array_add(core, NULL);
        h.core = (char **)g_ptr_array_free(core, FALSE);
        flags = G_APPLICATION_NON_UNIQUE;
        break;
    }
    h.socket = g_build_filename(g_get_user_runtime_dir(), "poiesis", "host.sock", NULL);
    h.app = gtk_application_new("app.poiesis", flags);
    g_signal_connect(h.app, "activate", G_CALLBACK(on_activate), &h);
    g_signal_connect(h.app, "shutdown", G_CALLBACK(on_shutdown), &h);
    int status = g_application_run(G_APPLICATION(h.app), 1, argv); /* our own options are read above */
    g_object_unref(h.app);
    g_strfreev(h.core);
    g_free(h.socket);
    return status;
}
