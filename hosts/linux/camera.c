#include "camera.h"

#include <glib/gstdio.h>
#include <gst/gst.h>
#include <gst/video/video.h>
#include <math.h>
#include <stdlib.h>
#include <string.h>

enum {
    GRID_W = 64, GRID_H = 36, /* one brightness sample per block of the picture */
    STILL_BELOW = 3,          /* mean change per sample under which a frame is not drawn */
    STOP_WAIT_S = 10,         /* longest wait for a recording to be finished */
    FIRST_FRAME_WAIT_S = 6,   /* longest wait for the camera's first frame */
};
#define SILENCE_DB (-160.0)

typedef struct Recording {
    guint generation;
    char *file;
    GstElement *bin, *filesink;
    GstPad *video_tee_pad, *audio_tee_pad;
    GstClockTime start;       /* running time when asked; earlier frames are not part of it */
    GstClockTime first, last; /* video times written, for the length */
    gboolean stopping;
    guint timeout;
} Recording;

struct Camera {
    PoiesisPicture *picture;
    CameraSend send;
    gpointer send_data;
    GstElement *pipeline, *video_tee, *audio_tee;
    GdkPaintable *paintable;
    guint bus_watch, first_frame_check;
    gint frames_seen;
    gboolean picture_on, broken;
    Recording *recording;
    guint generation;
    /* shared with the picture branch's streaming thread */
    GMutex lock;
    gboolean gate_open, have_shown;
    int fps;
    GstClockTime last_shown;
    GstVideoInfo info;
    guint8 shown[GRID_W * GRID_H], next[GRID_W * GRID_H];
};

typedef struct {
    Camera *camera;
    guint generation;
} Started;

static void apply(Camera *c);

/* MARK: messages for the core */

static void send_error(Camera *c, const char *what, const char *text) {
    JsonObject *o = json_object_new();
    json_object_set_string_member(o, "t", "error");
    json_object_set_string_member(o, "what", what);
    json_object_set_string_member(o, "text", text);
    c->send(o, c->send_data);
}

static void send_recording(Camera *c, const char *state, const char *file, double seconds) {
    JsonObject *o = json_object_new();
    json_object_set_string_member(o, "t", "recording");
    json_object_set_string_member(o, "state", state);
    if (file) {
        json_object_set_string_member(o, "file", file);
        json_object_set_double_member(o, "seconds", seconds);
    }
    c->send(o, c->send_data);
}

static gboolean send_started(gpointer data) {
    Started *s = data;
    if (s->camera->recording && s->camera->recording->generation == s->generation) {
        send_recording(s->camera, "started", NULL, 0);
    }
    g_free(s);
    return G_SOURCE_REMOVE;
}

/* MARK: the picture */

/* Lets a frame through to the screen only when the picture has moved since the last one
 * shown, and at most at the picture's rate: a sparse sample of the brightness plane against
 * the last one, as the core does in the terminal (frameMoved in tui_record.go). */
static GstPadProbeReturn picture_gate(GstPad *pad, GstPadProbeInfo *info, gpointer data) {
    (void)pad;
    Camera *c = data;
    GstBuffer *buffer = GST_PAD_PROBE_INFO_BUFFER(info);
    GstClockTime t = GST_BUFFER_PTS(buffer);
    g_mutex_lock(&c->lock);
    gboolean too_soon = c->have_shown && GST_CLOCK_TIME_IS_VALID(t) && GST_CLOCK_TIME_IS_VALID(c->last_shown) &&
                        t < c->last_shown + GST_SECOND / (GstClockTime)c->fps - 5 * GST_MSECOND;
    gboolean open = c->gate_open;
    g_mutex_unlock(&c->lock);
    if (!open || too_soon) return GST_PAD_PROBE_DROP;

    GstVideoFrame frame;
    if (!gst_video_frame_map(&frame, &c->info, buffer, GST_MAP_READ)) return GST_PAD_PROBE_OK;
    const guint8 *luma = GST_VIDEO_FRAME_PLANE_DATA(&frame, 0);
    int stride = GST_VIDEO_FRAME_PLANE_STRIDE(&frame, 0);
    int width = GST_VIDEO_FRAME_WIDTH(&frame), height = GST_VIDEO_FRAME_HEIGHT(&frame);
    g_mutex_lock(&c->lock);
    long sum = 0;
    for (int gy = 0, k = 0; gy < GRID_H; gy++) {
        const guint8 *line = luma + (gy * height / GRID_H + height / (2 * GRID_H)) * stride;
        for (int gx = 0; gx < GRID_W; gx++, k++) {
            guint8 v = line[gx * width / GRID_W + width / (2 * GRID_W)];
            c->next[k] = v;
            sum += abs((int)v - (int)c->shown[k]);
        }
    }
    gboolean still = c->have_shown && sum / (GRID_W * GRID_H) < STILL_BELOW;
    if (!still) {
        memcpy(c->shown, c->next, sizeof c->shown);
        c->have_shown = TRUE;
        c->last_shown = t;
    }
    g_mutex_unlock(&c->lock);
    gst_video_frame_unmap(&frame);
    return still ? GST_PAD_PROBE_DROP : GST_PAD_PROBE_OK;
}

void camera_set_picture(Camera *c, gboolean on, const double *matrix12, int fps) {
    poiesis_picture_set_look(c->picture, on ? matrix12 : NULL);
    g_mutex_lock(&c->lock);
    c->gate_open = on;
    c->fps = fps > 0 ? fps : 15;
    c->have_shown = FALSE; /* the first frame after this is always shown */
    g_mutex_unlock(&c->lock);
    c->picture_on = on;
    if (on) c->broken = FALSE; /* asked again: try again */
    apply(c);
}

/* MARK: the pipeline */

static gboolean test_sources(void) {
    return g_strcmp0(g_getenv("POIESIS_HOST_SOURCES"), "test") == 0;
}

static gboolean have(const char *element) {
    GstElementFactory *factory = gst_element_factory_find(element);
    if (!factory) return FALSE;
    gst_object_unref(factory);
    return TRUE;
}

/* The camera through PipeWire, as desktops share it now, else straight from the device; any
 * size and format it gives becomes 1280x720 NV12, cropped to the shape, never stretched.
 * POIESIS_HOST_SOURCES=test puts a moving test picture and a tone in their place, stamped
 * with the clock as a camera and a microphone stamp theirs. */
static char *description(void) {
    const char *video = test_sources()
        ? "videotestsrc name=camera is-live=true do-timestamp=true pattern=ball"
          " ! video/x-raw,format=NV12,width=1280,height=720,framerate=30/1"
        : have("pipewiresrc") ? "pipewiresrc name=camera ! capsfilter name=prefer ! decodebin"
                              : "v4l2src name=camera ! capsfilter name=prefer ! decodebin";
    const char *audio = test_sources() ? "audiotestsrc name=microphone is-live=true do-timestamp=true wave=sine freq=220 volume=0.3"
                                       : "autoaudiosrc name=microphone";
    return g_strdup_printf(
        "%s ! videoconvert ! aspectratiocrop aspect-ratio=16/9 ! videoscale"
        " ! video/x-raw,format=NV12,width=1280,height=720 ! tee name=vtee allow-not-linked=true"
        " vtee. ! queue name=shown leaky=downstream max-size-buffers=1 max-size-bytes=0 max-size-time=0"
        " ! videoconvert ! gtk4paintablesink name=screen sync=false"
        " %s ! audioconvert ! audioresample ! audio/x-raw,format=S16LE,rate=48000,channels=1"
        " ! level name=meter interval=100000000 post-messages=true ! tee name=atee allow-not-linked=true"
        " atee. ! queue leaky=downstream ! fakesink sync=false async=false",
        video, audio);
}

static GstClockTime running_time(Camera *c) {
    GstClock *clock = gst_element_get_clock(c->pipeline);
    if (!clock) return 0;
    GstClockTime now = gst_clock_get_time(clock);
    GstClockTime base = gst_element_get_base_time(c->pipeline);
    gst_object_unref(clock);
    return now > base ? now - base : 0;
}

static GstPadProbeReturn on_first_frame(GstPad *pad, GstPadProbeInfo *info, gpointer data) {
    (void)pad;
    (void)info;
    g_atomic_int_set(&((Camera *)data)->frames_seen, 1);
    return GST_PAD_PROBE_REMOVE;
}

static gboolean check_first_frame(gpointer data) {
    Camera *c = data;
    c->first_frame_check = 0;
    if (c->pipeline && !g_atomic_int_get(&c->frames_seen)) send_error(c, "camera", "no picture came from the camera");
    return G_SOURCE_REMOVE;
}

static void finish_recording(Camera *c, gboolean finished);

static gboolean inside(GstObject *object, GstElement *bin) {
    for (GstObject *o = object; o; o = GST_OBJECT_PARENT(o)) {
        if (o == GST_OBJECT(bin)) return TRUE;
    }
    return FALSE;
}

static const char *what_failed(Camera *c, GstObject *source) {
    if (c->recording && c->recording->bin && inside(source, c->recording->bin)) return "record";
    for (GstObject *o = source; o; o = GST_OBJECT_PARENT(o)) {
        if (g_strcmp0(GST_OBJECT_NAME(o), "camera") == 0) return "camera";
        if (g_strcmp0(GST_OBJECT_NAME(o), "microphone") == 0) return "microphone";
    }
    return "picture";
}

static void on_error(Camera *c, GstMessage *message) {
    GError *error = NULL;
    gst_message_parse_error(message, &error, NULL);
    const char *what = what_failed(c, GST_MESSAGE_SRC(message));
    send_error(c, what, error->message);
    g_error_free(error);
    if (strcmp(what, "record") == 0) {
        finish_recording(c, FALSE);
        return;
    }
    c->broken = TRUE; /* the camera stays off until the core asks again */
    if (c->recording) finish_recording(c, FALSE);
    apply(c);
}

static void on_level(Camera *c, const GstStructure *s) {
    const GValue *rms = gst_structure_get_value(s, "rms");
    const GValue *first = NULL;
    if (!rms) return;
    if (GST_VALUE_HOLDS_ARRAY(rms) && gst_value_array_get_size(rms) > 0) {
        first = gst_value_array_get_value(rms, 0);
    } else if (g_strcmp0(G_VALUE_TYPE_NAME(rms), "GValueArray") == 0) { /* the older form of the same list */
        G_GNUC_BEGIN_IGNORE_DEPRECATIONS
        GValueArray *list = g_value_get_boxed(rms);
        if (list && list->n_values > 0) first = g_value_array_get_nth(list, 0);
        G_GNUC_END_IGNORE_DEPRECATIONS
    }
    if (!first || !G_VALUE_HOLDS_DOUBLE(first)) return;
    double db = g_value_get_double(first);
    if (!isfinite(db) || db < SILENCE_DB) db = SILENCE_DB; /* JSON has no minus infinity */
    JsonObject *o = json_object_new();
    json_object_set_string_member(o, "t", "level");
    json_object_set_double_member(o, "db", db);
    c->send(o, c->send_data);
}

static gboolean on_bus(GstBus *bus, GstMessage *message, gpointer data) {
    (void)bus;
    Camera *c = data;
    switch (GST_MESSAGE_TYPE(message)) {
    case GST_MESSAGE_ERROR:
        on_error(c, message);
        break;
    case GST_MESSAGE_ELEMENT: {
        const GstStructure *s = gst_message_get_structure(message);
        if (gst_structure_has_name(s, "level")) {
            on_level(c, s);
        } else if (gst_structure_has_name(s, "GstBinForwarded") && c->recording) {
            GstMessage *inner = NULL;
            gst_structure_get(s, "message", GST_TYPE_MESSAGE, &inner, NULL);
            if (inner && GST_MESSAGE_TYPE(inner) == GST_MESSAGE_EOS &&
                GST_MESSAGE_SRC(inner) == GST_OBJECT(c->recording->filesink)) {
                finish_recording(c, TRUE); /* the file is whole */
            }
            if (inner) gst_message_unref(inner);
        }
        break;
    }
    default:
        break;
    }
    return G_SOURCE_CONTINUE;
}

static void build(Camera *c) {
    char *text = description();
    GError *error = NULL;
    GstElement *pipeline = gst_parse_launch_full(text, NULL, GST_PARSE_FLAG_FATAL_ERRORS, &error);
    g_free(text);
    if (!pipeline) {
        send_error(c, "camera", error ? error->message : "the camera could not be set up");
        g_clear_error(&error);
        c->broken = TRUE;
        return;
    }
    c->pipeline = pipeline;
    c->video_tee = gst_bin_get_by_name(GST_BIN(pipeline), "vtee");
    c->audio_tee = gst_bin_get_by_name(GST_BIN(pipeline), "atee");

    GstElement *prefer = gst_bin_get_by_name(GST_BIN(pipeline), "prefer");
    if (prefer) { /* the camera's own 720p first, then anything it has */
        GstCaps *caps = gst_caps_from_string("image/jpeg,width=1280,height=720; video/x-raw,width=1280,height=720;"
                                             " image/jpeg; video/x-raw");
        g_object_set(prefer, "caps", caps, NULL);
        gst_caps_unref(caps);
        gst_object_unref(prefer);
    }

    GstCaps *caps = gst_caps_from_string("video/x-raw,format=NV12,width=1280,height=720,framerate=0/1");
    gst_video_info_from_caps(&c->info, caps);
    gst_caps_unref(caps);

    GstElement *shown = gst_bin_get_by_name(GST_BIN(pipeline), "shown");
    GstPad *pad = gst_element_get_static_pad(shown, "src");
    gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER, picture_gate, c, NULL);
    gst_object_unref(pad);
    gst_object_unref(shown);

    GstElement *screen = gst_bin_get_by_name(GST_BIN(pipeline), "screen");
    g_object_get(screen, "paintable", &c->paintable, NULL);
    gst_object_unref(screen);

    g_atomic_int_set(&c->frames_seen, 0);
    pad = gst_element_get_static_pad(c->video_tee, "sink");
    gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_BUFFER, on_first_frame, c, NULL);
    gst_object_unref(pad);
    c->first_frame_check = g_timeout_add_seconds(FIRST_FRAME_WAIT_S, check_first_frame, c);

    GstBus *bus = gst_element_get_bus(pipeline);
    c->bus_watch = gst_bus_add_watch(bus, on_bus, c);
    gst_object_unref(bus);
    if (gst_element_set_state(pipeline, GST_STATE_PLAYING) == GST_STATE_CHANGE_FAILURE) {
        /* the reason arrives on the bus as an error */
    }
}

static void teardown(Camera *c) {
    poiesis_picture_set_paintable(c->picture, NULL);
    g_clear_object(&c->paintable);
    if (c->first_frame_check) g_source_remove(c->first_frame_check);
    c->first_frame_check = 0;
    gst_element_set_state(c->pipeline, GST_STATE_NULL);
    if (c->bus_watch) g_source_remove(c->bus_watch);
    c->bus_watch = 0;
    gst_clear_object(&c->video_tee);
    gst_clear_object(&c->audio_tee);
    gst_clear_object(&c->pipeline);
}

/* Runs the camera while the picture shows or something records, and lets it go otherwise, so
 * its light is off whenever nothing needs it. */
static void apply(Camera *c) {
    gboolean needed = !c->broken && (c->picture_on || c->recording);
    if (needed && !c->pipeline) build(c);
    if (!needed && c->pipeline) teardown(c);
    if (c->pipeline) poiesis_picture_set_paintable(c->picture, c->picture_on ? c->paintable : NULL);
}

/* MARK: the recording */

static void set_if_there(GstElement *e, const char *property, const char *value) {
    if (g_object_class_find_property(G_OBJECT_GET_CLASS(e), property)) gst_util_set_object_arg(G_OBJECT(e), property, value);
}

/* The graphics chip's H.264 encoder when it has one (VA-API for Intel and AMD, NVENC), x264
 * in software otherwise: 1 Mbit/s, a key frame every two seconds. */
static GstElement *video_encoder(void) {
    static const char *const hardware[] = {"vah264lpenc", "vah264enc", "nvh264enc"};
    for (size_t i = 0; i < G_N_ELEMENTS(hardware); i++) {
        GstElement *e = gst_element_factory_make(hardware[i], NULL);
        if (!e) continue;
        set_if_there(e, "rate-control", "cbr");
        set_if_there(e, "rc-mode", "cbr");
        set_if_there(e, "bitrate", "1000");
        set_if_there(e, "key-int-max", "60");
        set_if_there(e, "gop-size", "60");
        return e;
    }
    GstElement *e = gst_element_factory_make("x264enc", NULL);
    if (e) {
        set_if_there(e, "tune", "zerolatency");
        set_if_there(e, "speed-preset", "veryfast");
        set_if_there(e, "bitrate", "1000");
        set_if_there(e, "key-int-max", "60");
    }
    return e;
}

static GstElement *audio_encoder(void) {
    static const char *const names[] = {"fdkaacenc", "avenc_aac", "voaacenc"};
    for (size_t i = 0; i < G_N_ELEMENTS(names); i++) {
        GstElement *e = gst_element_factory_make(names[i], NULL);
        if (!e) continue;
        set_if_there(e, "bitrate", "128000");
        return e;
    }
    return NULL;
}

/* Frames from before the start was asked for are not part of the entry. */
static GstPadProbeReturn drop_early(GstPad *pad, GstPadProbeInfo *info, gpointer data) {
    (void)pad;
    Recording *r = data;
    GstClockTime t = GST_BUFFER_PTS(GST_PAD_PROBE_INFO_BUFFER(info));
    return GST_CLOCK_TIME_IS_VALID(t) && t < r->start ? GST_PAD_PROBE_DROP : GST_PAD_PROBE_OK;
}

static GstPadProbeReturn on_written(GstPad *pad, GstPadProbeInfo *info, gpointer data) {
    (void)pad;
    Recording *r = data;
    GstClockTime t = GST_BUFFER_PTS(GST_PAD_PROBE_INFO_BUFFER(info));
    if (GST_CLOCK_TIME_IS_VALID(t)) {
        if (!GST_CLOCK_TIME_IS_VALID(r->first)) r->first = t;
        r->last = t;
    }
    return GST_PAD_PROBE_OK;
}

typedef struct {
    Camera *camera;
    Recording *recording;
} Watch;

static GstPadProbeReturn on_first_written(GstPad *pad, GstPadProbeInfo *info, gpointer data) {
    (void)pad;
    (void)info;
    Watch *w = data;
    Started *s = g_new0(Started, 1);
    s->camera = w->camera;
    s->generation = w->recording->generation;
    g_idle_add(send_started, s); /* the core's clock starts with the first frame written */
    return GST_PAD_PROBE_REMOVE;
}

static GstPad *ghost(GstElement *bin, GstElement *element, const char *name) {
    GstPad *target = gst_element_get_static_pad(element, "sink");
    GstPad *pad = gst_ghost_pad_new(name, target);
    gst_object_unref(target);
    gst_element_add_pad(bin, pad);
    return pad;
}

static GstElement *queue_for(GstClockTime most) {
    GstElement *q = gst_element_factory_make("queue", NULL);
    if (q) g_object_set(q, "max-size-time", most, "max-size-buffers", 0, "max-size-bytes", 0, NULL);
    return q;
}

/* The recording branch: both queues, the encoders, one mp4 with its index at the front. */
static gboolean attach(Camera *c, Recording *r) {
    GstElement *vq = queue_for(3 * GST_SECOND), *venc = video_encoder(), *vparse = gst_element_factory_make("h264parse", NULL);
    GstElement *aq = queue_for(3 * GST_SECOND), *aconv = gst_element_factory_make("audioconvert", NULL);
    GstElement *aenc = audio_encoder(), *aparse = gst_element_factory_make("aacparse", NULL);
    GstElement *mux = gst_element_factory_make("mp4mux", NULL), *sink = gst_element_factory_make("filesink", NULL);
    GstElement *all[] = {vq, venc, vparse, aq, aconv, aenc, aparse, mux, sink};
    gboolean complete = TRUE;
    for (size_t i = 0; i < G_N_ELEMENTS(all); i++) complete = complete && all[i];
    if (!complete) {
        for (size_t i = 0; i < G_N_ELEMENTS(all); i++) {
            if (all[i]) gst_object_unref(gst_object_ref_sink(all[i]));
        }
        send_error(c, "record", "a part of the recorder is missing: gst-plugins-good, -bad, -ugly and gst-libav are needed");
        return FALSE;
    }
    g_object_set(mux, "faststart", TRUE, NULL);
    g_object_set(sink, "location", r->file, NULL);
    GstElement *bin = gst_bin_new(NULL);
    g_object_set(bin, "message-forward", TRUE, NULL); /* the file's end reaches the bus */
    gst_bin_add_many(GST_BIN(bin), vq, venc, vparse, aq, aconv, aenc, aparse, mux, sink, NULL);
    if (!gst_element_link_many(vq, venc, vparse, mux, NULL) || !gst_element_link_many(aq, aconv, aenc, aparse, mux, NULL) ||
        !gst_element_link(mux, sink)) {
        gst_object_unref(gst_object_ref_sink(bin));
        send_error(c, "record", "the recorder's parts do not fit together");
        return FALSE;
    }
    GstPad *video = ghost(bin, vq, "video"), *audio = ghost(bin, aq, "audio");
    r->start = running_time(c);
    gst_pad_add_probe(video, GST_PAD_PROBE_TYPE_BUFFER, drop_early, r, NULL);
    gst_pad_add_probe(audio, GST_PAD_PROBE_TYPE_BUFFER, drop_early, r, NULL);
    gst_pad_set_offset(video, -(gint64)r->start); /* the file starts at zero */
    gst_pad_set_offset(audio, -(gint64)r->start);
    GstPad *written = gst_element_get_static_pad(vparse, "src");
    Watch *w = g_new0(Watch, 1);
    w->camera = c;
    w->recording = r;
    gst_pad_add_probe(written, GST_PAD_PROBE_TYPE_BUFFER, on_first_written, w, g_free);
    gst_pad_add_probe(written, GST_PAD_PROBE_TYPE_BUFFER, on_written, r, NULL);
    gst_object_unref(written);

    gst_bin_add(GST_BIN(c->pipeline), bin);
    gst_element_sync_state_with_parent(bin); /* ready before the first buffer comes */
    r->bin = bin;
    r->filesink = sink;
    r->video_tee_pad = gst_element_request_pad_simple(c->video_tee, "src_%u");
    r->audio_tee_pad = gst_element_request_pad_simple(c->audio_tee, "src_%u");
    gboolean linked = gst_pad_link(r->video_tee_pad, video) == GST_PAD_LINK_OK &&
                      gst_pad_link(r->audio_tee_pad, audio) == GST_PAD_LINK_OK;
    if (!linked) send_error(c, "record", "the recorder could not be joined to the camera");
    return linked;
}

void camera_start_recording(Camera *c, const char *file) {
    if (c->recording) return;
    char *dir = g_path_get_dirname(file);
    g_mkdir_with_parents(dir, 0755);
    g_free(dir);
    g_unlink(file);
    Recording *r = g_new0(Recording, 1);
    r->generation = ++c->generation;
    r->file = g_strdup(file);
    r->first = r->last = GST_CLOCK_TIME_NONE;
    c->recording = r;
    c->broken = FALSE; /* asked again: try again */
    apply(c);
    if (!c->pipeline || !attach(c, r)) finish_recording(c, FALSE);
}

/* Unlinks one branch from its tee when no buffer is passing, and ends its stream. */
static GstPadProbeReturn end_branch(GstPad *tee_pad, GstPadProbeInfo *info, gpointer data) {
    (void)info;
    (void)data;
    GstPad *peer = gst_pad_get_peer(tee_pad);
    if (peer) {
        gst_pad_unlink(tee_pad, peer);
        gst_pad_send_event(peer, gst_event_new_eos());
        gst_object_unref(peer);
    }
    return GST_PAD_PROBE_REMOVE;
}

static gboolean stop_timed_out(gpointer data) {
    Camera *c = data;
    if (c->recording) {
        c->recording->timeout = 0;
        finish_recording(c, FALSE);
    }
    return G_SOURCE_REMOVE;
}

void camera_stop_recording(Camera *c) {
    Recording *r = c->recording;
    if (!r || r->stopping) return;
    r->stopping = TRUE;
    gst_pad_add_probe(r->video_tee_pad, GST_PAD_PROBE_TYPE_IDLE, end_branch, NULL, NULL);
    gst_pad_add_probe(r->audio_tee_pad, GST_PAD_PROBE_TYPE_IDLE, end_branch, NULL, NULL);
    r->timeout = g_timeout_add_seconds(STOP_WAIT_S, stop_timed_out, c);
}

static void release(GstElement *tee, GstPad *pad) {
    if (!pad) return;
    GstPad *peer = gst_pad_get_peer(pad);
    if (peer) {
        gst_pad_unlink(pad, peer);
        gst_object_unref(peer);
    }
    if (tee) gst_element_release_request_pad(tee, pad);
    gst_object_unref(pad);
}

/* Takes the branch out and tells the core; `finished` says whether the file was closed whole. */
static void finish_recording(Camera *c, gboolean finished) {
    Recording *r = c->recording;
    if (!r) return;
    c->recording = NULL;
    if (r->timeout) g_source_remove(r->timeout);
    if (!finished && r->bin) send_error(c, "record", "the recording could not be finished");
    release(c->video_tee, r->video_tee_pad);
    release(c->audio_tee, r->audio_tee_pad);
    if (r->bin) {
        gst_element_set_state(r->bin, GST_STATE_NULL);
        if (c->pipeline) gst_bin_remove(GST_BIN(c->pipeline), r->bin);
    }
    double seconds = GST_CLOCK_TIME_IS_VALID(r->first) ? (double)(r->last - r->first) / GST_SECOND : 0;
    send_recording(c, "stopped", r->file, seconds);
    g_free(r->file);
    g_free(r);
    apply(c);
}

/* MARK: life */

Camera *camera_new(PoiesisPicture *picture, CameraSend send, gpointer data) {
    Camera *c = g_new0(Camera, 1);
    c->picture = g_object_ref(picture); /* kept until the camera is let go, even past the window */
    c->send = send;
    c->send_data = data;
    c->fps = 15;
    c->last_shown = GST_CLOCK_TIME_NONE;
    g_mutex_init(&c->lock);
    return c;
}

void camera_free(Camera *c) {
    if (c->recording) {
        camera_stop_recording(c);
        gint64 end = g_get_monotonic_time() + STOP_WAIT_S * G_USEC_PER_SEC;
        while (c->recording && g_get_monotonic_time() < end) g_main_context_iteration(NULL, TRUE);
    }
    c->picture_on = FALSE;
    if (c->recording) finish_recording(c, FALSE);
    apply(c);
    g_object_unref(c->picture);
    g_mutex_clear(&c->lock);
    g_free(c);
}
