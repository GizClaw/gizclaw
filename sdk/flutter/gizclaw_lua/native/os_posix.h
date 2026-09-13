#ifndef GIZCLAW_LUA_OS_POSIX_H
#define GIZCLAW_LUA_OS_POSIX_H
#include "h2_pal.h"

extern const h2_pal_mem_api_t gcl_mem;
extern const h2_pal_log_api_t gcl_log;
extern const h2_pal_time_api_t gcl_time;
extern const h2_pal_task_api_t gcl_task;
extern const h2_pal_queue_api_t gcl_queue;
extern const h2_pal_sync_api_t gcl_sync;
// Read-only, relative paths; each component is opened with O_NOFOLLOW.
// Caller keeps this API alive until Runtime/Host have stopped and joined.
int gcl_fs_create(const char *root, h2_pal_fs_api_t *out);
void gcl_fs_destroy(h2_pal_fs_api_t *api);
#endif
