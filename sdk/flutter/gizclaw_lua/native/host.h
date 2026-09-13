#ifndef GIZCLAW_LUA_HOST_H
#define GIZCLAW_LUA_HOST_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif

typedef struct bridge bridge;
typedef struct {
  uint64_t id;
  int32_t index, cancel;
  char input[4096], options[4096];
} bridge_event;

// Returns an owned host, or NULL. All strings are borrowed during calls only.
// Calls other than native PAL callbacks are serialized on the Dart isolate.
bridge *gcl_create(const char *root, int width, int height, int buttons, int touch,
                   size_t memory, size_t source, size_t output, int timeout,
                   int jobs);
// Stops and joins workers before freeing all borrowed PAL backing storage.
void gcl_destroy(bridge *host);
int gcl_register(bridge *host, const char *name);
int gcl_start(bridge *host);
// Positive job ID on success; negative PAL result on failure.
int gcl_submit(bridge *host, const char *name, const char *source);
// Copies status/error into the caller-owned output buffer.
int gcl_status(bridge *host, int id, char *output, size_t capacity);
// Public job result semantics: NULL/0 queries byte length, including embedded NUL.
int gcl_result(bridge *host, int id, char *output, size_t capacity,
               size_t *size, int *has_result);
void gcl_release(bridge *host, int id);
int gcl_cancel(bridge *host, int id);
int gcl_poll(bridge *host, bridge_event *event);
int gcl_complete(bridge *host, uint64_t id, int result, const char *output,
                 const char *error);
void gcl_pump(bridge *host);
int gcl_button(bridge *host, int id, int down);
int gcl_touch(bridge *host, int kind, int x, int y);
// pixels is NULL for sequence-only polling, otherwise holds width*height
// RGB565 values. Returns presentation sequence.
uint64_t gcl_frame(bridge *host, uint16_t *pixels);
void gcl_audio_config(bridge *host, int input, int output);
// Whole 640-byte S16LE frames only. At most eight frames are retained.
int gcl_audio_push(bridge *host, const uint8_t *bytes, size_t size);
size_t gcl_audio_read(bridge *host, uint8_t *bytes, size_t capacity);
#ifdef __cplusplus
}
#endif
#endif
