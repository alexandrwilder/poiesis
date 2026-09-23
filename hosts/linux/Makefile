# The Linux host: one C program on GTK 4, VTE and GStreamer, the parts Omarchy is built with.
#   make            the host, poiesis-host
#   make look-test  checks that the look's colour matrix reaches GTK the right way round
PKGS := gtk4 vte-2.91-gtk4 gstreamer-1.0 gstreamer-video-1.0 json-glib-1.0
CFLAGS ?= -O2
CFLAGS += -std=c11 -Wall -Wextra -Werror $(shell pkg-config --cflags $(PKGS))
LDLIBS += $(shell pkg-config --libs $(PKGS)) -lm

poiesis-host: main.c camera.c link.c picture.c camera.h link.h picture.h
	$(CC) $(CFLAGS) -o $@ main.c camera.c link.c picture.c $(LDLIBS)

look-test: look_test.c picture.c picture.h
	$(CC) $(CFLAGS) -o $@ look_test.c picture.c $(LDLIBS)

clean:
	rm -f poiesis-host look-test

.PHONY: clean
