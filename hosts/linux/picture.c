#include "picture.h"

#include <math.h>

struct _PoiesisPicture {
    GtkWidget parent_instance;
    GdkPaintable *paintable;
    gboolean tinted;
    graphene_matrix_t matrix;
    graphene_vec4_t offset;
};

G_DEFINE_FINAL_TYPE(PoiesisPicture, poiesis_picture, GTK_TYPE_WIDGET)

void look_to_gtk(const double *m, graphene_matrix_t *matrix, graphene_vec4_t *offset) {
    /* GTK multiplies the pixel by the transposed matrix, so each of the core's rows becomes a
     * column here: column 0 makes red, column 1 green, column 2 blue. Alpha stays as it is.
     * graphene takes the sixteen values row by row. */
    const float values[16] = {
        (float)m[0], (float)m[4], (float)m[8], 0,
        (float)m[1], (float)m[5], (float)m[9], 0,
        (float)m[2], (float)m[6], (float)m[10], 0,
        0, 0, 0, 1,
    };
    graphene_matrix_init_from_float(matrix, values);
    graphene_vec4_init(offset, (float)m[3], (float)m[7], (float)m[11], 0);
}

static void on_invalidate(GdkPaintable *paintable, PoiesisPicture *self) {
    (void)paintable;
    gtk_widget_queue_draw(GTK_WIDGET(self));
}

static void poiesis_picture_snapshot(GtkWidget *widget, GtkSnapshot *snapshot) {
    PoiesisPicture *self = POIESIS_PICTURE(widget);
    if (!self->paintable) return;
    float width = (float)gtk_widget_get_width(widget), height = (float)gtk_widget_get_height(widget);
    float pw = (float)gdk_paintable_get_intrinsic_width(self->paintable);
    float ph = (float)gdk_paintable_get_intrinsic_height(self->paintable);
    if (pw <= 0 || ph <= 0) {
        pw = width;
        ph = height;
    }
    /* cover: the smallest scale that fills the widget, centred, the rest cut off */
    float scale = fmaxf(width / pw, height / ph);
    float dw = pw * scale, dh = ph * scale;
    gtk_snapshot_push_clip(snapshot, &GRAPHENE_RECT_INIT(0, 0, width, height));
    if (self->tinted) gtk_snapshot_push_color_matrix(snapshot, &self->matrix, &self->offset);
    gtk_snapshot_save(snapshot);
    gtk_snapshot_translate(snapshot, &GRAPHENE_POINT_INIT((width - dw) / 2, (height - dh) / 2));
    gdk_paintable_snapshot(self->paintable, snapshot, dw, dh);
    gtk_snapshot_restore(snapshot);
    if (self->tinted) gtk_snapshot_pop(snapshot);
    gtk_snapshot_pop(snapshot);
}

static void drop_paintable(PoiesisPicture *self) {
    if (!self->paintable) return;
    g_signal_handlers_disconnect_by_func(self->paintable, on_invalidate, self);
    g_clear_object(&self->paintable);
}

static void poiesis_picture_dispose(GObject *object) {
    drop_paintable(POIESIS_PICTURE(object));
    G_OBJECT_CLASS(poiesis_picture_parent_class)->dispose(object);
}

static void poiesis_picture_class_init(PoiesisPictureClass *klass) {
    G_OBJECT_CLASS(klass)->dispose = poiesis_picture_dispose;
    GTK_WIDGET_CLASS(klass)->snapshot = poiesis_picture_snapshot;
}

static void poiesis_picture_init(PoiesisPicture *self) {
    gtk_widget_set_hexpand(GTK_WIDGET(self), TRUE);
    gtk_widget_set_vexpand(GTK_WIDGET(self), TRUE);
}

GtkWidget *poiesis_picture_new(void) {
    return g_object_new(POIESIS_TYPE_PICTURE, NULL);
}

void poiesis_picture_set_paintable(PoiesisPicture *self, GdkPaintable *paintable) {
    if (paintable == self->paintable) return;
    drop_paintable(self);
    if (paintable) {
        self->paintable = g_object_ref(paintable);
        g_signal_connect(paintable, "invalidate-contents", G_CALLBACK(on_invalidate), self);
        g_signal_connect(paintable, "invalidate-size", G_CALLBACK(on_invalidate), self);
    }
    gtk_widget_queue_draw(GTK_WIDGET(self));
}

void poiesis_picture_set_look(PoiesisPicture *self, const double *m) {
    static const double identity[12] = {1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0};
    self->tinted = FALSE;
    if (m) {
        for (int i = 0; i < 12; i++) {
            if (fabs(m[i] - identity[i]) > 0.001) self->tinted = TRUE;
        }
    }
    if (self->tinted) look_to_gtk(m, &self->matrix, &self->offset);
    gtk_widget_queue_draw(GTK_WIDGET(self));
}
