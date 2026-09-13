// Firmware host adapter. All Lua entry occurs on GizOS workers. Dart only
// polls copied data; this library never calls Dart from a native thread.
#include "host.h"
#include "h2_desktop_platform.h"
#include "h2_pal.h"
#include "h2_posix_pal_core.h"
#include "runtime/h2_lua_internal.h"
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define CAP_MAX 32
#define EVENT_MAX 64
#define JOB_MAX 64
#define TEXT_MAX 4096
_Static_assert(offsetof(bridge_event, input) == 16, "Dart event ABI input offset");
_Static_assert(offsetof(bridge_event, options) == 4112, "Dart event ABI options offset");

typedef struct {
  bridge *owner;
  int index;
} capability;
typedef struct {
  uint32_t id;
  char *result;
} result_slot;
struct bridge {
  pthread_mutex_t lock;
  h2_runtime_t *runtime;
  h2_lua_host_t *host;
  h2_posix_host_fs_t *fs;
  h2_pal_display_api_t display;
  h2_pal_fs_api_t confined_fs;
  h2_pal_audio_api_t audio;
  int audio_in, audio_out, mic_started, speaker_started, track_count;
  uint8_t mic[5120], speaker[5120];
  size_t mic_bytes, speaker_bytes;

  h2_pal_touch_api_t touch;
  h2_pal_periph_api_t periph;
  h2_runtime_component_mapper_t mapper;
  int width, height, buttons, touch_enabled, display_open, touch_open;
  uint16_t *pixels, *presented;
  uint64_t frame;
  h2_pal_touch_event_t touches[EVENT_MAX];
  size_t touch_head, touch_count;
  capability capabilities[CAP_MAX];
  int cap_count;
  bridge_event events[EVENT_MAX];
  size_t head, count;
  result_slot results[JOB_MAX];
  size_t output_limit;
};
static int display_open(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  int rc = b->display_open ? H2_PAL_ERR_BUSY : 0;
  if (!rc)
    b->display_open = 1;
  pthread_mutex_unlock(&b->lock);
  return rc;
}
static int display_close(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  b->display_open = 0;
  pthread_mutex_unlock(&b->lock);
  return 0;
}
static int display_info(void *u, h2_display_info_t *i) {
  bridge *b = u;
  if (!i)
    return H2_PAL_ERR_INVALID_ARG;
  *i = (h2_display_info_t){.width = b->width, .height = b->height, .native_format = H2_DISPLAY_PIXEL_RGB565};
  return 0;
}
static int display_draw(void *u, const h2_display_rect_t *r, const void *p, size_t stride, h2_display_pixel_format_t f) {
  bridge *b = u;
  if (!r || !p || f != H2_DISPLAY_PIXEL_RGB565 || r->x < 0 || r->y < 0 || r->width <= 0 || r->height <= 0 || r->x > b->width - r->width || r->y > b->height - r->height || stride < (size_t)r->width * 2)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&b->lock);
  for (int y = 0; y < r->height; y++)
    memcpy(b->pixels + (r->y + y) * b->width + r->x, (const char *)p + y * stride, r->width * 2);
  pthread_mutex_unlock(&b->lock);
  return 0;
}
static int display_present(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  memcpy(b->presented, b->pixels, (size_t)b->width * b->height * 2);
  b->frame++;
  pthread_mutex_unlock(&b->lock);
  return 0;
}
static const h2_pal_display_vtable_t display_vtable = {.open = display_open, .close = display_close, .get_info = display_info, .draw_bitmap = display_draw, .present = display_present};
static int touch_open(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  int rc = !b->touch_enabled ? H2_PAL_ERR_UNSUPPORTED : b->touch_open ? H2_PAL_ERR_BUSY
                                                                      : 0;
  if (!rc)
    b->touch_open = 1;
  pthread_mutex_unlock(&b->lock);
  return rc;
}
static int touch_close(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  b->touch_open = 0;
  b->touch_count = 0;
  pthread_mutex_unlock(&b->lock);
  return 0;
}
static int touch_info(void *u, h2_pal_touch_info_t *i) {
  bridge *b = u;
  if (!i)
    return H2_PAL_ERR_INVALID_ARG;
  *i = (h2_pal_touch_info_t){b->width, b->height};
  return 0;
}
static int touch_poll(void *u, h2_pal_touch_event_t *e) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  int rc = H2_PAL_ERR_WOULD_BLOCK;
  if (b->touch_count) {
    *e = b->touches[b->touch_head];
    b->touch_head = (b->touch_head + 1) % EVENT_MAX;
    b->touch_count--;
    rc = 0;
  }
  pthread_mutex_unlock(&b->lock);
  return rc;
}
static const h2_pal_touch_vtable_t touch_vtable = {.open = touch_open, .close = touch_close, .get_info = touch_info, .poll_event = touch_poll};
static const h2_pal_periph_single_button_payload_t button_payload = {.delivery = H2_PAL_BUTTON_DELIVERY_PUSH_EDGE};
static int periph_get(void *u, h2_pal_periph_id_t id, h2_pal_periph_info_t *i) {
  bridge *b = u;
  if (!i || id < 1 || id > (unsigned)b->buttons)
    return H2_PAL_ERR_NOT_FOUND;
  *i = (h2_pal_periph_info_t){.id = id, .type = H2_PAL_PERIPH_TYPE_SINGLE_BUTTON, .payload = &button_payload, .payload_size = sizeof(button_payload)};
  snprintf(i->name, sizeof(i->name), "button%u", id);
  return 0;
}
static int periph_list(void *u, h2_pal_periph_type_t f, h2_pal_periph_cb_t cb, void *cu) {
  bridge *b = u;
  if (!cb)
    return H2_PAL_ERR_INVALID_ARG;
  if (f != H2_PAL_PERIPH_TYPE_ANY && f != H2_PAL_PERIPH_TYPE_SINGLE_BUTTON)
    return 0;
  for (int n = 1; n <= b->buttons; n++) {
    h2_pal_periph_info_t i;
    periph_get(u, n, &i);
    int rc = cb(cu, &i);
    if (rc)
      return rc;
  }
  return 0;
}
static int map_get(void *u, h2_runtime_component_id_t id, h2_pal_periph_id_t *out) {
  bridge *b = u;
  if (!out || id < 1 || id > (unsigned)b->buttons)
    return H2_PAL_ERR_NOT_FOUND;
  *out = id;
  return 0;
}
static int map_list(void *u, h2_runtime_component_t f, h2_runtime_component_mapping_cb_t cb, void *cu) {
  bridge *b = u;
  if (!cb)
    return H2_PAL_ERR_INVALID_ARG;
  if (f != H2_RUNTIME_COMPONENT_BUTTON)
    return 0;
  for (int n = 1; n <= b->buttons; n++) {
    h2_runtime_component_mapping_entry_t e = {n, n};
    int rc = cb(cu, &e);
    if (rc)
      return rc;
  }
  return 0;
}
static const h2_pal_periph_vtable_t periph_vtable = {.get = periph_get, .list = periph_list};
static const h2_runtime_component_mapper_vtable_t mapper_vtable = {.get_periph_id = map_get, .list = map_list};
static int cap_call(void *u, uint64_t id, const char *input, const char *options, char *out, size_t capacity, const char **error) {
  (void)out;
  (void)capacity;
  (void)error;
  capability *c = u;
  bridge *b = c->owner;
  if ((input && strlen(input) >= TEXT_MAX) || (options && strlen(options) >= TEXT_MAX))
    return H2_PAL_ERR_NO_SPACE;
  pthread_mutex_lock(&b->lock);
  if (b->count == EVENT_MAX) {
    pthread_mutex_unlock(&b->lock);
    return H2_PAL_ERR_NO_SPACE;
  }
  bridge_event *e = &b->events[(b->head + b->count++) % EVENT_MAX];
  memset(e, 0, sizeof(*e));
  e->id = id;
  e->index = c->index;
  snprintf(e->input, TEXT_MAX, "%s", input ? input : "");
  snprintf(e->options, TEXT_MAX, "%s", options ? options : "");
  pthread_mutex_unlock(&b->lock);
  return H2_PAL_ERR_WOULD_BLOCK;
}
static void cap_cancel(void *u, uint64_t id) {
  capability *c = u;
  bridge *b = c->owner;
  pthread_mutex_lock(&b->lock);
  if (b->count < EVENT_MAX) {
    bridge_event *e = &b->events[(b->head + b->count++) % EVENT_MAX];
    memset(e, 0, sizeof(*e));
    e->id = id;
    e->index = c->index;
    e->cancel = 1;
  }
  pthread_mutex_unlock(&b->lock);
}
// Current GizOS exposes job state but not root return values. This module
// writes only the calling job's slot, identified by firmware execution context.
static int result_write(lua_State *L) {
  bridge *b = lua_touserdata(L, lua_upvalueindex(1));
  h2_lua_execution_context_t *c = *(h2_lua_execution_context_t **)lua_getextraspace(L);
  size_t size;
  const char *s = luaL_checklstring(L, 1, &size);
  if (!c || !c->job || size > b->output_limit)
    return luaL_error(L, "result limit exceeded");
  char *copy = malloc(size + 1);
  if (!copy)
    return luaL_error(L, "result allocation failed");
  memcpy(copy, s, size);
  copy[size] = 0;
  pthread_mutex_lock(&b->lock);
  for (int n = 0; n < JOB_MAX; n++) {
    if (b->results[n].id == c->job->id) {
      free(b->results[n].result);
      b->results[n].result = copy;
      pthread_mutex_unlock(&b->lock);
      return 0;
    }
  }
  pthread_mutex_unlock(&b->lock);
  free(copy);
  return luaL_error(L, "unknown result job");
}
static int result_open(void *state, void *u) {
  lua_State *L = state;
  lua_pushlightuserdata(L, u);
  lua_pushcclosure(L, result_write, 1);
  return 1;
}

static const h2_audio_pcm_format_t audio_format = {.sample_rate_hz = 16000, .frame_samples_per_channel = 320, .channels = 1, .sample_format = H2_AUDIO_SAMPLE_S16LE};
static int audio_info(void *u, h2_audio_info_t *i) {
  bridge *b = u;
  if (!i)
    return H2_PAL_ERR_INVALID_ARG;
  *i = (h2_audio_info_t){.available = b->audio_in || b->audio_out, .mic_supported = b->audio_in, .playback_supported = b->audio_out, .mic_format = audio_format, .playback_format = audio_format, .mic_queue_frames = 8, .track_queue_frames = 8, .max_tracks = 4};
  return 0;
}
static int mic_start(void *u) {
  bridge *b = u;
  if (!b->audio_in)
    return H2_PAL_ERR_UNSUPPORTED;
  b->mic_started = 1;
  return 0;
}
static int mic_stop(void *u) {
  bridge *b = u;
  pthread_mutex_lock(&b->lock);
  b->mic_started = 0;
  b->mic_bytes = 0;
  pthread_mutex_unlock(&b->lock);
  return 0;
}
static int speaker_start(void *u) {
  bridge *b = u;
  if (!b->audio_out)
    return H2_PAL_ERR_UNSUPPORTED;
  b->speaker_started = 1;
  return 0;
}
static int speaker_stop(void *u) {
  bridge *b = u;
  b->speaker_started = 0;
  return 0;
}
static int mic_read(void *u, h2_audio_frame_t *f, uint32_t timeout) {
  (void)timeout;
  bridge *b = u;
  if (!f || !f->data || f->capacity < 640)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&b->lock);
  int rc = H2_PAL_ERR_WOULD_BLOCK;
  if (b->mic_started && b->mic_bytes >= 640) {
    memcpy(f->data, b->mic, 640);
    b->mic_bytes -= 640;
    memmove(b->mic, b->mic + 640, b->mic_bytes);
    f->bytes = 640;
    f->samples_per_channel = 320;
    f->sample_rate_hz = 16000;
    f->channels = 1;
    f->sample_format = H2_AUDIO_SAMPLE_S16LE;
    rc = 0;
  }
  pthread_mutex_unlock(&b->lock);
  return rc;
}
static int track_write(h2_pal_audio_track_t *t, const h2_audio_frame_t *f, uint32_t timeout) {
  (void)timeout;
  bridge *b = t->user;
  if (!f || !f->data || f->bytes != 640 || f->channels != 1 || f->sample_rate_hz != 16000 || f->sample_format != H2_AUDIO_SAMPLE_S16LE)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&b->lock);
  int rc = H2_PAL_ERR_WOULD_BLOCK;
  if (b->speaker_bytes + 640 <= sizeof(b->speaker)) {
    memcpy(b->speaker + b->speaker_bytes, f->data, 640);
    b->speaker_bytes += 640;
    rc = 0;
  }
  pthread_mutex_unlock(&b->lock);
  return rc;
}
static int track_close(h2_pal_audio_track_t *t) {
  bridge *b = t->user;
  b->track_count--;
  free(t);
  return 0;
}
static int track_create(void *u, const h2_audio_track_config_t *c, h2_pal_audio_track_t **out) {
  bridge *b = u;
  if (!c || !out)
    return H2_PAL_ERR_INVALID_ARG;
  if (!b->audio_out || c->format.sample_rate_hz != 16000 || c->format.channels != 1 || c->format.sample_format != H2_AUDIO_SAMPLE_S16LE || c->format.frame_samples_per_channel != 320)
    return H2_PAL_ERR_UNSUPPORTED;
  if (b->track_count == 4)
    return H2_PAL_ERR_NO_SPACE;
  h2_pal_audio_track_t *t = calloc(1, sizeof(*t));
  if (!t)
    return H2_PAL_ERR_NO_MEMORY;
  *t = (h2_pal_audio_track_t){.user = b, .audio = &b->audio, .write = track_write, .close = track_close};
  b->track_count++;
  *out = t;
  return 0;
}
static const h2_pal_audio_vtable_t audio_vtable = {.get_info = audio_info, .start_mic = mic_start, .stop_mic = mic_stop, .start_speaker = speaker_start, .stop_speaker = speaker_stop, .mic_read = mic_read, .create_track = track_create};
void gcl_audio_config(bridge *b, int input, int output) {
  b->audio_in = input;
  b->audio_out = output;
}
int gcl_audio_push(bridge *b, const uint8_t *p, size_t size) {
  if (!b->audio_in || !p || size == 0 || size % 640)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&b->lock);
  int rc = H2_PAL_ERR_NO_SPACE;
  if (size <= sizeof(b->mic) - b->mic_bytes) {
    memcpy(b->mic + b->mic_bytes, p, size);
    b->mic_bytes += size;
    rc = 0;
  }
  pthread_mutex_unlock(&b->lock);
  return rc;
}
size_t gcl_audio_read(bridge *b, uint8_t *p, size_t size) {
  pthread_mutex_lock(&b->lock);
  size_t n = b->speaker_bytes < size ? b->speaker_bytes : size;
  memcpy(p, b->speaker, n);
  b->speaker_bytes -= n;
  memmove(b->speaker, b->speaker + n, b->speaker_bytes);
  pthread_mutex_unlock(&b->lock);
  return n;
}
static int fs_path(const char *path, char *out, size_t capacity) {
  if (!path || !*path || *path == '/' || strstr(path, "..") || strchr(path, '\\'))
    return H2_PAL_ERR_INVALID_ARG;
  return snprintf(out, capacity, "/apps/%s", path) >= (int)capacity ? H2_PAL_ERR_INVALID_ARG : 0;
}
static int fs_open(void *u, const char *p, h2_pal_fs_open_mode_t mode, h2_pal_fs_file_t **out) {
  bridge *b = u;
  char path[2048];
  int rc = fs_path(p, path, sizeof(path));
  if (rc)
    return rc;
  if (mode != H2_PAL_FS_OPEN_READ)
    return H2_PAL_ERR_UNSUPPORTED;
  return h2_pal_fs_open(h2_posix_host_fs_api(b->fs), path, mode, out);
}
static int fs_read(void *u, h2_pal_fs_file_t *f, void *p, size_t n, size_t *out) {
  bridge *b = u;
  return h2_pal_fs_read(h2_posix_host_fs_api(b->fs), f, p, n, out);
}
static int fs_close(void *u, h2_pal_fs_file_t *f) {
  bridge *b = u;
  return h2_pal_fs_close(h2_posix_host_fs_api(b->fs), f);
}
static int fs_stat(void *u, const char *p, h2_pal_fs_stat_t *out) {
  bridge *b = u;
  char path[2048];
  int rc = fs_path(p, path, sizeof(path));
  return rc ? rc : h2_pal_fs_stat(h2_posix_host_fs_api(b->fs), path, out);
}
static const h2_pal_fs_vtable_t fs_vtable = {.open = fs_open, .read = fs_read, .close = fs_close, .stat = fs_stat};
void gcl_destroy(bridge *b) {
  if (!b)
    return;
  if (b->host) {
    h2_lua_host_stop(b->host);
    h2_lua_host_join(b->host);
    h2_lua_host_destroy(b->host);
  }
  if (b->runtime)
    h2_runtime_deinit(b->runtime);
  if (b->fs)
    h2_posix_host_fs_destroy(b->fs);
  for (int n = 0; n < JOB_MAX; n++)
    free(b->results[n].result);
  free(b->pixels);
  free(b->presented);
  pthread_mutex_destroy(&b->lock);
  free(b);
}
bridge *gcl_create(const char *root, int width, int height, int buttons, int touch, size_t memory, size_t source, size_t output, int timeout, int jobs) {
  if (!root || width < 1 || height < 1 || width > 4096 || height > 4096 || buttons < 0 || buttons > 8 || jobs < 1 || jobs > JOB_MAX)
    return NULL;
  bridge *b = calloc(1, sizeof(*b));
  if (!b)
    return NULL;
  pthread_mutex_init(&b->lock, NULL);
  b->width = width;
  b->height = height;
  b->buttons = buttons;
  b->touch_enabled = touch;
  b->output_limit = output;
  b->pixels = calloc((size_t)width * height, 2);
  b->presented = calloc((size_t)width * height, 2);
  if (!b->pixels || !b->presented)
    goto fail;
  const char *targets[] = {"/apps"};
  const char *sources[] = {root};
  if (h2_posix_host_fs_create(sources, targets, 1, &b->fs))
    goto fail;
  b->audio = (h2_pal_audio_api_t){b, &audio_vtable};
  b->confined_fs = (h2_pal_fs_api_t){b, &fs_vtable};
  b->display = (h2_pal_display_api_t){b, &display_vtable};
  b->touch = (h2_pal_touch_api_t){b, &touch_vtable};
  b->periph = (h2_pal_periph_api_t){b, &periph_vtable};
  b->mapper = (h2_runtime_component_mapper_t){b, &mapper_vtable};
  h2_runtime_config_t config = {
      .board = "test",
      .target = "desktop",
      .chip = "host",
      .firmware_info = h2_pal_unsupported_firmware_info_api(),
      .mem = h2_desktop_platform_default_allocator(),
      .log = h2_desktop_platform_log_api(),
      .time = h2_desktop_platform_time_api(),
      .timer = h2_pal_unsupported_timer_api(),
      .task = h2_desktop_platform_task_api(),
      .queue = h2_desktop_platform_queue_api(),
      .sync = h2_desktop_platform_sync_api(),
      .fs = &b->confined_fs,
      .disk = h2_pal_unsupported_disk_api(),
      .pref = h2_pal_unsupported_pref_api(),
      .crypto = h2_pal_unsupported_crypto_api(),
      .http = h2_pal_unsupported_http_api(),
      .net = h2_pal_unsupported_net_api(),
      .netif = h2_pal_unsupported_netif_api(),
      .mqtt = h2_pal_unsupported_mqtt_api(),
      .webrtc = h2_pal_unsupported_webrtc_api(),
      .wifi_sta = h2_pal_unsupported_wifi_sta_api(),
      .wifi_ap = h2_pal_unsupported_wifi_ap_api(),
      .wifi_csi = h2_pal_unsupported_wifi_csi_api(),
      .wifi_settings = h2_pal_unsupported_wifi_settings_api(),
      .ble_host = h2_pal_unsupported_ble_host_api(),
      .modem = h2_pal_unsupported_modem_api(),
      .power = h2_pal_unsupported_power_api(),
      .display = &b->display,
      .audio = &b->audio,
      .audio_decoder = h2_pal_unsupported_audio_decoder_api(),
      .periph = &b->periph,
      .button = h2_pal_unsupported_button_api(),
      .touch = &b->touch,
      .buzzer = h2_pal_unsupported_buzzer_api(),
      .nfc = h2_pal_unsupported_nfc_api(),
      .nfc_card_emulation = h2_pal_unsupported_nfc_card_emulation_api(),
      .imu = h2_pal_unsupported_imu_api(),
      .gpio_irq = h2_pal_unsupported_gpio_irq_api(),
      .led = h2_pal_unsupported_led_api(),
      .switch_api = h2_pal_unsupported_switch_api(),
      .pwm_switch = h2_pal_unsupported_pwm_switch_api(),
      .input = h2_pal_unsupported_input_api(),
      .system_event = h2_pal_unsupported_system_event_api(),
      .video_decoder = h2_pal_unsupported_video_decoder_api(),
      .component_mapper = &b->mapper,
  };
  if (h2_runtime_init(&config, &b->runtime))
    goto fail;
  if (buttons && h2_runtime_input_start(b->runtime, NULL))
    goto fail;
  h2_lua_host_config_t hc = {.runtime = b->runtime, .worker_count = 1, .max_jobs = jobs, .vm_memory_limit_bytes = memory, .source_limit_bytes = source, .output_limit_bytes = output, .execution_timeout_ms = timeout, .pending_capability_capacity = 32};
  if (h2_lua_host_create(&hc, &b->host))
    goto fail;
  if (h2_lua_register_module(b->host, "_gizclaw_result", result_open, b))
    goto fail;
  return b;
fail:
  gcl_destroy(b);
  return NULL;
}
int gcl_register(bridge *b, const char *name) {
  if (!b || b->cap_count == CAP_MAX)
    return H2_PAL_ERR_NO_SPACE;
  int n = b->cap_count;
  b->capabilities[n] = (capability){b, n};
  int rc = h2_lua_register_capability(b->host, name, cap_call, cap_cancel, &b->capabilities[n]);
  if (!rc)
    b->cap_count++;
  return rc;
}
int gcl_start(bridge *b) { return h2_lua_host_start(b->host); }
int gcl_submit(bridge *b, const char *name, const char *source) {
  pthread_mutex_lock(&b->lock);
  int slot = -1;
  for (int n = 0; n < JOB_MAX; n++)
    if (!b->results[n].id) {
      slot = n;
      break;
    }
  if (slot < 0) {
    pthread_mutex_unlock(&b->lock);
    return -1;
  }
  uint32_t id = 0;
  int rc = h2_lua_job_submit_text(b->host, name, (const uint8_t *)source, strlen(source), NULL, 0, &id);
  if (!rc)
    b->results[slot].id = id;
  pthread_mutex_unlock(&b->lock);
  return rc ? rc : (int)id;
}
int gcl_status(bridge *b, int id, char *out, size_t capacity) {
  if (!out || capacity == 0)
    return H2_PAL_ERR_INVALID_ARG;
  out[0] = 0;
  h2_lua_job_status_t s;
  int rc = h2_lua_job_get_status(b->host, id, &s);
  if (rc)
    return rc;
  snprintf(out, capacity, "%s", s.message);
  if (s.state == H2_LUA_JOB_SUCCEEDED) {
    pthread_mutex_lock(&b->lock);
    for (int n = 0; n < JOB_MAX; n++)
      if (b->results[n].id == (unsigned)id)
        snprintf(out, capacity, "%s", b->results[n].result ? b->results[n].result : "null");
    pthread_mutex_unlock(&b->lock);
  }
  return s.state;
}
void gcl_release(bridge *b, int id) {
  if (h2_lua_job_release(b->host, id))
    return;
  pthread_mutex_lock(&b->lock);
  for (int n = 0; n < JOB_MAX; n++)
    if (b->results[n].id == (unsigned)id) {
      free(b->results[n].result);
      b->results[n] = (result_slot){0};
    }
  pthread_mutex_unlock(&b->lock);
}
int gcl_cancel(bridge *b, int id) { return h2_lua_job_cancel(b->host, id); }
int gcl_poll(bridge *b, bridge_event *out) {
  pthread_mutex_lock(&b->lock);
  int ok = b->count != 0;
  if (ok) {
    *out = b->events[b->head];
    b->head = (b->head + 1) % EVENT_MAX;
    b->count--;
  }
  pthread_mutex_unlock(&b->lock);
  return ok;
}
int gcl_complete(bridge *b, uint64_t id, int rc, const char *output, const char *error) { return h2_lua_capability_complete(b->host, id, rc, output, error); }
void gcl_pump(bridge *b) {
  uint8_t payload[H2_RUNTIME_EVENT_PAYLOAD_MAX];
  h2_runtime_event_t e = {.payload = payload, .payload_capacity = sizeof(payload)};
  for (int i = 0; i < 128 && !h2_runtime_poll_event(b->runtime, &e); i++) {
    for (int n = 0; n < JOB_MAX; n++)
      if (b->results[n].id)
        h2_lua_dispatch_runtime_event(b->host, b->results[n].id, &e);
  }
}
int gcl_button(bridge *b, int id, int down) { return h2_runtime_button_push_edge(b->runtime, id, down ? H2_RUNTIME_BUTTON_EDGE_DOWN : H2_RUNTIME_BUTTON_EDGE_UP); }
int gcl_touch(bridge *b, int kind, int x, int y) {
  if (!b->touch_enabled || kind < 1 || kind > 3 || x < 0 || y < 0 || x >= b->width || y >= b->height)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&b->lock);
  int rc = H2_PAL_ERR_NO_SPACE;
  if (b->touch_count < EVENT_MAX) {
    b->touches[(b->touch_head + b->touch_count++) % EVENT_MAX] = (h2_pal_touch_event_t){kind, x, y};
    rc = 0;
  }
  pthread_mutex_unlock(&b->lock);
  return rc;
}
uint64_t gcl_frame(bridge *b, uint16_t *out) {
  pthread_mutex_lock(&b->lock);
  if (out)
    memcpy(out, b->presented, (size_t)b->width * b->height * 2);
  uint64_t frame = b->frame;
  pthread_mutex_unlock(&b->lock);
  return frame;
}
