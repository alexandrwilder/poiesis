/* Checks that the look's colour matrix reaches GTK the right way round: a known colour goes
 * through a matrix whose rows all differ, each of GTK's renderers draws it, and the pixel read
 * back must be the sum the core means (docs/HOST.md). Needs a display; prints PASS or FAIL. */
#include <gtk/gtk.h>
#include <math.h>
#include <stdio.h>

#include "picture.h"

enum { SIDE = 8 };
static const double look[12] = {0.6, 0.3, 0.0, 0.0, 0.0, 0.5, 0.4, 0.0, 0.2, 0.0, 0.7, 0.05};
static const double colour[3] = {0.8, 0.4, 0.2};

/* Returns the number of channels off by more than a rounding step and a half. */
static int check(const char *name, GskRenderer *renderer, GdkDisplay *display) {
    GError *error = NULL;
    if (!gsk_renderer_realize_for_display(renderer, display, &error)) {
        printf("%s: not on this machine (%s)\n", name, error->message);
        g_error_free(error);
        return 0;
    }
    graphene_matrix_t matrix;
    graphene_vec4_t offset;
    look_to_gtk(look, &matrix, &offset);
    GtkSnapshot *snapshot = gtk_snapshot_new();
    gtk_snapshot_push_color_matrix(snapshot, &matrix, &offset);
    GdkRGBA rgba = {(float)colour[0], (float)colour[1], (float)colour[2], 1};
    gtk_snapshot_append_color(snapshot, &rgba, &GRAPHENE_RECT_INIT(0, 0, SIDE, SIDE));
    gtk_snapshot_pop(snapshot);
    GskRenderNode *node = gtk_snapshot_free_to_node(snapshot);
    GdkTexture *texture = gsk_renderer_render_texture(renderer, node, &GRAPHENE_RECT_INIT(0, 0, SIDE, SIDE));
    GdkTextureDownloader *downloader = gdk_texture_downloader_new(texture);
    gdk_texture_downloader_set_format(downloader, GDK_MEMORY_R8G8B8A8);
    guchar pixels[SIDE * SIDE * 4];
    gdk_texture_downloader_download_into(downloader, pixels, SIDE * 4);
    gdk_texture_downloader_free(downloader);
    const guchar *middle = pixels + ((SIDE / 2) * SIDE + SIDE / 2) * 4;
    int off = 0;
    for (int ch = 0; ch < 3; ch++) {
        const double *row = look + ch * 4;
        double want = row[0] * colour[0] + row[1] * colour[1] + row[2] * colour[2] + row[3];
        double got = middle[ch] / 255.0;
        gboolean ok = fabs(got - want) <= 1.5 / 255;
        off += !ok;
        printf("%s %c: want %.3f got %.3f%s\n", name, "RGB"[ch], want, got, ok ? "" : "  <- off");
    }
    g_object_unref(texture);
    gsk_render_node_unref(node);
    gsk_renderer_unrealize(renderer);
    return off;
}

int main(void) {
    gtk_init();
    GdkDisplay *display = gdk_display_get_default();
    struct {
        const char *name;
        GskRenderer *renderer;
    } renderers[] = {
        {"cairo", gsk_cairo_renderer_new()},
        {"gl", gsk_gl_renderer_new()},
        {"vulkan", gsk_vulkan_renderer_new()},
    };
    int off = 0;
    for (size_t i = 0; i < G_N_ELEMENTS(renderers); i++) {
        off += check(renderers[i].name, renderers[i].renderer, display);
        g_object_unref(renderers[i].renderer);
    }
    printf(off ? "FAIL: the look is not drawn as the core means it\n" : "PASS: the look reaches GTK the right way round\n");
    return off ? 1 : 0;
}
