#pragma once
#include <json-glib/json-glib.h>

#include "picture.h"

/* The camera, the microphone and the recording from one GStreamer pipeline: one owner for the
 * camera (docs/HOST.md). The picture branch draws a frame only when the picture has moved, at
 * most at the picture's rate; a still scene draws nothing. For each part of a recording a
 * branch with the encoders is added to the running pipeline and ends in its own file. */
typedef struct Camera Camera;
/* Messages for the core, always called on the main loop; takes the message over. */
typedef void (*CameraSend)(JsonObject *message, gpointer data);

Camera *camera_new(PoiesisPicture *picture, CameraSend send, gpointer data);
void camera_set_picture(Camera *camera, gboolean on, const double *matrix12, int fps);
void camera_start_recording(Camera *camera, const char *file);
void camera_stop_recording(Camera *camera);
/* Finishes a recording in progress, waiting a few seconds at most, then lets the camera go. */
void camera_free(Camera *camera);
