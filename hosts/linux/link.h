#pragma once
#include <json-glib/json-glib.h>

/* The host's side of the socket in docs/HOST.md: one client, one JSON object per line.
 * Everything runs on the main loop: messages arrive there, and are sent from there. */
typedef struct Link Link;
typedef void (*LinkHandler)(JsonObject *message, gpointer data);

Link *link_start(const char *path, LinkHandler handler, gpointer data, GError **error);
/* Sends one message and takes it over. Without a connected core it is dropped. */
void link_send(Link *link, JsonObject *message);
void link_stop(Link *link);
