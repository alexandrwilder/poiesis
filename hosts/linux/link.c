#include "link.h"

#include <gio/gio.h>
#include <gio/gunixsocketaddress.h>
#include <glib/gstdio.h>
#include <string.h>

struct Link {
    char *path;
    GSocketService *service;
    GSocketConnection *client; /* the one core, or NULL */
    GDataInputStream *in;
    GCancellable *reading;
    LinkHandler handler;
    gpointer data;
};

static void read_next(Link *link);

static void drop_client(Link *link) {
    if (link->reading) {
        g_cancellable_cancel(link->reading);
        g_clear_object(&link->reading);
    }
    g_clear_object(&link->in);
    if (link->client) {
        g_io_stream_close(G_IO_STREAM(link->client), NULL, NULL);
        g_clear_object(&link->client);
    }
}

static void on_line(GObject *source, GAsyncResult *result, gpointer user_data) {
    GError *error = NULL;
    gsize length = 0;
    char *line = g_data_input_stream_read_line_finish_utf8(G_DATA_INPUT_STREAM(source), result, &length, &error);
    if (g_error_matches(error, G_IO_ERROR, G_IO_ERROR_CANCELLED)) {
        g_error_free(error); /* the link is gone or has a new client: touch nothing */
        return;
    }
    Link *link = user_data;
    if (!line) { /* the core closed its end, or the socket failed */
        g_clear_error(&error);
        drop_client(link);
        return;
    }
    /* unknown or garbled lines are ignored, as the contract says */
    JsonParser *parser = json_parser_new();
    if (json_parser_load_from_data(parser, line, (gssize)length, NULL)) {
        JsonNode *root = json_parser_get_root(parser);
        if (root && JSON_NODE_HOLDS_OBJECT(root)) link->handler(json_node_get_object(root), link->data);
    }
    g_object_unref(parser);
    g_free(line);
    if (link->in) read_next(link);
}

static void read_next(Link *link) {
    g_data_input_stream_read_line_async(link->in, G_PRIORITY_DEFAULT, link->reading, on_line, link);
}

static gboolean on_incoming(GSocketService *service, GSocketConnection *connection, GObject *source, gpointer user_data) {
    (void)service;
    (void)source;
    Link *link = user_data;
    drop_client(link); /* one core at a time: a new one replaces the old */
    link->client = g_object_ref(connection);
    link->in = g_data_input_stream_new(g_io_stream_get_input_stream(G_IO_STREAM(connection)));
    link->reading = g_cancellable_new();
    read_next(link);
    return TRUE;
}

Link *link_start(const char *path, LinkHandler handler, gpointer data, GError **error) {
    char *dir = g_path_get_dirname(path);
    g_mkdir_with_parents(dir, 0700);
    g_free(dir);
    g_unlink(path);
    GSocketService *service = g_socket_service_new();
    GSocketAddress *address = g_unix_socket_address_new(path);
    gboolean added = g_socket_listener_add_address(G_SOCKET_LISTENER(service), address, G_SOCKET_TYPE_STREAM,
                                                   G_SOCKET_PROTOCOL_DEFAULT, NULL, NULL, error);
    g_object_unref(address);
    if (!added) {
        g_object_unref(service);
        return NULL;
    }
    g_chmod(path, 0600); /* readable and writable only by this user */
    Link *link = g_new0(Link, 1);
    link->path = g_strdup(path);
    link->service = service;
    link->handler = handler;
    link->data = data;
    g_signal_connect(service, "incoming", G_CALLBACK(on_incoming), link);
    g_socket_service_start(service);
    return link;
}

void link_send(Link *link, JsonObject *message) {
    JsonNode *node = json_node_alloc();
    json_node_init_object(node, message);
    json_object_unref(message);
    if (link && link->client) {
        char *text = json_to_string(node, FALSE);
        char *line = g_strconcat(text, "\n", NULL);
        GOutputStream *out = g_io_stream_get_output_stream(G_IO_STREAM(link->client));
        if (!g_output_stream_write_all(out, line, strlen(line), NULL, NULL, NULL)) drop_client(link);
        g_free(line);
        g_free(text);
    }
    json_node_unref(node);
}

void link_stop(Link *link) {
    if (!link) return;
    drop_client(link);
    g_socket_service_stop(link->service);
    g_socket_listener_close(G_SOCKET_LISTENER(link->service));
    g_object_unref(link->service);
    g_unlink(link->path);
    g_free(link->path);
    g_free(link);
}
