#pragma once
#include <gtk/gtk.h>

/* The camera behind the text: a paintable drawn to cover the whole widget, cropped, never
 * stretched, through the look's colour matrix. The graphics chip does the drawing. */
#define POIESIS_TYPE_PICTURE (poiesis_picture_get_type())
G_DECLARE_FINAL_TYPE(PoiesisPicture, poiesis_picture, POIESIS, PICTURE, GtkWidget)

GtkWidget *poiesis_picture_new(void);
/* The camera's frames; NULL shows nothing. */
void poiesis_picture_set_paintable(PoiesisPicture *self, GdkPaintable *paintable);
/* The core's look (docs/HOST.md): twelve numbers, rows for red, green and blue, each the
 * weights of red, green and blue and an offset. NULL shows the camera as it is. */
void poiesis_picture_set_look(PoiesisPicture *self, const double *matrix12);

/* The same twelve numbers as GTK takes them, which computes transpose(matrix) * pixel + offset. */
void look_to_gtk(const double *matrix12, graphene_matrix_t *matrix, graphene_vec4_t *offset);
