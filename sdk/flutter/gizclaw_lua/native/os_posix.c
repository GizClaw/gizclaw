// Host-owned POSIX PAL services. No GizOS provider sources are required.
#include "os_posix.h"
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

static int result(int rc) {
  if (!rc)
    return H2_PAL_OK;
  if (rc == ENOMEM)
    return H2_PAL_ERR_NO_MEMORY;
  if (rc == ETIMEDOUT)
    return H2_PAL_ERR_TIMEOUT;
  if (rc == EBUSY)
    return H2_PAL_ERR_BUSY;
  if (rc == ENOENT)
    return H2_PAL_ERR_NOT_FOUND;
  if (rc == EINVAL || rc == ELOOP || rc == ENOTDIR)
    return H2_PAL_ERR_INVALID_ARG;
  return H2_PAL_ERR_IO;
}
static void *mem_alloc(void *u, size_t n) {
  (void)u;
  return malloc(n);
}
static void *mem_resize(void *u, void *p, size_t n) {
  (void)u;
  return realloc(p, n);
}
static void mem_free(void *u, void *p) {
  (void)u;
  free(p);
}
static const h2_pal_mem_vtable_t mem_vtable = {mem_alloc, mem_resize, mem_free};
const h2_pal_mem_api_t gcl_mem = {NULL, &mem_vtable};
static int log_write(void *u, h2_pal_log_level_t level, const char *scope, const char *message) {
  (void)u;
  if (!message)
    return H2_PAL_ERR_INVALID_ARG;
  return fprintf(stderr, "[gizclaw_lua:%d] %s: %s\n", level, scope ? scope : "", message) < 0 ? H2_PAL_ERR_IO : 0;
}
static const h2_pal_log_vtable_t log_vtable = {log_write};
const h2_pal_log_api_t gcl_log = {NULL, &log_vtable};
static int monotonic_us(void *u, uint64_t *out) {
  (void)u;
  if (!out)
    return H2_PAL_ERR_INVALID_ARG;
  struct timespec now;
  if (clock_gettime(CLOCK_MONOTONIC, &now))
    return result(errno);
  *out = (uint64_t)now.tv_sec * 1000000u + (uint64_t)now.tv_nsec / 1000u;
  return 0;
}
static int monotonic_ms(void *u, uint64_t *out) {
  int rc = monotonic_us(u, out);
  if (!rc)
    *out /= 1000u;
  return rc;
}
static int sleep_ms(void *u, uint32_t ms) {
  (void)u;
  struct timespec delay = {ms / 1000u, (long)(ms % 1000u) * 1000000L};
  while (nanosleep(&delay, &delay))
    if (errno != EINTR)
      return result(errno);
  return 0;
}
static const h2_pal_time_vtable_t time_vtable = {.get_monotonic_ms = monotonic_ms, .get_monotonic_us = monotonic_us, .sleep_ms = sleep_ms};
const h2_pal_time_api_t gcl_time = {NULL, &time_vtable};

struct h2_pal_task {
  pthread_t thread;
  h2_pal_task_entry_t entry;
  void *ctx;
};
static void *task_entry(void *p) {
  h2_pal_task_t *t = p;
  t->entry(t->ctx);
  return NULL;
}
static int task_start(void *u, const h2_pal_task_options_t *options, h2_pal_task_entry_t entry, void *ctx, h2_pal_task_t **out) {
  (void)u;
  if (!entry || !out)
    return H2_PAL_ERR_INVALID_ARG;
  *out = NULL;
  h2_pal_task_t *t = calloc(1, sizeof(*t));
  if (!t)
    return H2_PAL_ERR_NO_MEMORY;
  pthread_attr_t attr;
  int rc = pthread_attr_init(&attr);
  if (rc) {
    free(t);
    return result(rc);
  }
  size_t size;
  rc = pthread_attr_getstacksize(&attr, &size);
  if (!rc && options && options->min_stack_size > size)
    rc = pthread_attr_setstacksize(&attr, options->min_stack_size);
  t->entry = entry;
  t->ctx = ctx;
  if (!rc)
    rc = pthread_create(&t->thread, &attr, task_entry, t);
  pthread_attr_destroy(&attr);
  if (rc) {
    free(t);
    return result(rc);
  }
  *out = t;
  return 0;
}
static int task_join(void *u, h2_pal_task_t *t) {
  (void)u;
  if (!t)
    return H2_PAL_ERR_INVALID_ARG;
  int rc = pthread_join(t->thread, NULL);
  if (!rc)
    free(t);
  return result(rc);
}
static const h2_pal_task_vtable_t task_vtable = {task_start, task_join};
const h2_pal_task_api_t gcl_task = {NULL, &task_vtable};

struct h2_pal_mutex {
  pthread_mutex_t mutex;
  const h2_pal_mem_api_t *mem;
};
static int mutex_create(void *u, const h2_pal_mutex_config_t *cfg, h2_pal_mutex_t **out) {
  (void)u;
  if (!cfg || !out || (cfg->flags & ~H2_PAL_MUTEX_FLAG_RECURSIVE))
    return H2_PAL_ERR_INVALID_ARG;
  *out = NULL;
  const h2_pal_mem_api_t *mem = cfg->allocator ? cfg->allocator : &gcl_mem;
  h2_pal_mutex_t *m = h2_pal_mem_alloc(mem, sizeof(*m));
  if (!m)
    return H2_PAL_ERR_NO_MEMORY;
  m->mem = mem;
  pthread_mutexattr_t attr;
  int rc = pthread_mutexattr_init(&attr);
  if (rc) {
    h2_pal_mem_free(mem, m);
    return result(rc);
  }
  if (cfg->flags & H2_PAL_MUTEX_FLAG_RECURSIVE)
    rc = pthread_mutexattr_settype(&attr, PTHREAD_MUTEX_RECURSIVE);
  if (!rc)
    rc = pthread_mutex_init(&m->mutex, &attr);
  pthread_mutexattr_destroy(&attr);
  if (rc) {
    h2_pal_mem_free(mem, m);
    return result(rc);
  }
  *out = m;
  return 0;
}
static int mutex_destroy(void *u, h2_pal_mutex_t *m) {
  (void)u;
  if (!m)
    return H2_PAL_ERR_INVALID_ARG;
  int rc = pthread_mutex_destroy(&m->mutex);
  if (!rc)
    h2_pal_mem_free(m->mem, m);
  return result(rc);
}
static int mutex_lock(void *u, h2_pal_mutex_t *m) {
  (void)u;
  return m ? result(pthread_mutex_lock(&m->mutex)) : H2_PAL_ERR_INVALID_ARG;
}
static int mutex_try(void *u, h2_pal_mutex_t *m) {
  (void)u;
  return m ? result(pthread_mutex_trylock(&m->mutex)) : H2_PAL_ERR_INVALID_ARG;
}
static int mutex_unlock(void *u, h2_pal_mutex_t *m) {
  (void)u;
  return m ? result(pthread_mutex_unlock(&m->mutex)) : H2_PAL_ERR_INVALID_ARG;
}
// Runtime/Lua need mutexes only. Missing semaphore/condition callbacks use the
// public PAL wrappers' UNSUPPORTED result, rather than partial implementations.
static const h2_pal_sync_vtable_t sync_vtable = {.create_mutex = mutex_create, .destroy_mutex = mutex_destroy, .lock_mutex = mutex_lock, .try_lock_mutex = mutex_try, .unlock_mutex = mutex_unlock};
const h2_pal_sync_api_t gcl_sync = {NULL, &sync_vtable};

struct h2_pal_queue {
  pthread_mutex_t mutex;
  pthread_cond_t changed;
  const h2_pal_mem_api_t *mem;
  size_t size, capacity, count, head;
  int closed;
  unsigned char data[];
};
static int queue_create(void *u, const h2_pal_queue_config_t *cfg, h2_pal_queue_t **out) {
  (void)u;
  if (!cfg || !out || !cfg->item_size || !cfg->item_count || cfg->item_count > (SIZE_MAX - sizeof(h2_pal_queue_t)) / cfg->item_size)
    return H2_PAL_ERR_INVALID_ARG;
  *out = NULL;
  const h2_pal_mem_api_t *mem = cfg->allocator ? cfg->allocator : &gcl_mem;
  h2_pal_queue_t *q = h2_pal_mem_alloc(mem, sizeof(*q) + cfg->item_size * cfg->item_count);
  if (!q)
    return H2_PAL_ERR_NO_MEMORY;
  memset(q, 0, sizeof(*q));
  q->mem = mem;
  q->size = cfg->item_size;
  q->capacity = cfg->item_count;
  int rc = pthread_mutex_init(&q->mutex, NULL);
  if (rc) {
    h2_pal_mem_free(mem, q);
    return result(rc);
  }
  pthread_condattr_t attr;
  rc = pthread_condattr_init(&attr);
  if (!rc) {
#ifndef __APPLE__
    rc = pthread_condattr_setclock(&attr, CLOCK_MONOTONIC);
#endif
    if (!rc)
      rc = pthread_cond_init(&q->changed, &attr);
    pthread_condattr_destroy(&attr);
  }
  if (rc) {
    pthread_mutex_destroy(&q->mutex);
    h2_pal_mem_free(mem, q);
    return result(rc);
  }
  *out = q;
  return 0;
}
static void queue_destroy(void *u, h2_pal_queue_t *q) {
  (void)u;
  if (!q)
    return;
  // Owner has closed the queue and joined all producers/consumers.
  pthread_cond_destroy(&q->changed);
  pthread_mutex_destroy(&q->mutex);
  h2_pal_mem_free(q->mem, q);
}
static int queue_transfer(h2_pal_queue_t *q, void *item, uint32_t ms, int send, int latest) {
  if (!q || !item)
    return H2_PAL_ERR_INVALID_ARG;
  uint64_t now;
  int rc = monotonic_us(NULL, &now);
  if (rc)
    return rc;
  uint64_t end = now + (uint64_t)ms * 1000;
  rc = pthread_mutex_lock(&q->mutex);
  if (rc)
    return result(rc);
  if (!q->closed && latest && q->count == q->capacity) {
    q->head = (q->head + 1) % q->capacity;
    q->count--;
  }
  while (!q->closed && (send ? q->count == q->capacity : q->count == 0)) {
    if (!ms) {
      rc = H2_PAL_ERR_TIMEOUT;
      break;
    }
    int wait;
    if (ms == UINT32_MAX)
      wait = pthread_cond_wait(&q->changed, &q->mutex);
    else {
#ifdef __APPLE__
      rc = monotonic_us(NULL, &now);
      if (rc)
        break;
      if (now >= end) {
        rc = H2_PAL_ERR_TIMEOUT;
        break;
      }
      uint64_t left = end - now;
      struct timespec delay = {left / 1000000, (long)(left % 1000000) * 1000};
      wait = pthread_cond_timedwait_relative_np(&q->changed, &q->mutex, &delay);
#else
      struct timespec deadline = {end / 1000000, (long)(end % 1000000) * 1000};
      wait = pthread_cond_timedwait(&q->changed, &q->mutex, &deadline);
#endif
    }
    if (wait) {
      rc = result(wait);
      break;
    }
  }
  // Closed queues reject sends but allow consumers to drain retained items.
  if (q->closed && (send || !q->count))
    rc = H2_PAL_ERR_CLOSED;
  if (!rc) {
    if (send) {
      memcpy(q->data + ((q->head + q->count) % q->capacity) * q->size, item, q->size);
      q->count++;
    } else {
      memcpy(item, q->data + q->head * q->size, q->size);
      q->head = (q->head + 1) % q->capacity;
      q->count--;
    }
    pthread_cond_broadcast(&q->changed);
  }
  pthread_mutex_unlock(&q->mutex);
  return rc;
}
static int queue_send(void *u, h2_pal_queue_t *q, const void *p, uint32_t ms) {
  (void)u;
  return queue_transfer(q, (void *)p, ms, 1, 0);
}
static int queue_latest(void *u, h2_pal_queue_t *q, const void *p) {
  (void)u;
  return queue_transfer(q, (void *)p, 0, 1, 1);
}
static int queue_recv(void *u, h2_pal_queue_t *q, void *p, uint32_t ms) {
  (void)u;
  return queue_transfer(q, p, ms, 0, 0);
}
static int queue_reset(void *u, h2_pal_queue_t *q) {
  (void)u;
  if (!q)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&q->mutex);
  int rc = q->closed ? H2_PAL_ERR_CLOSED : 0;
  if (!rc)
    q->head = q->count = 0;
  pthread_cond_broadcast(&q->changed);
  pthread_mutex_unlock(&q->mutex);
  return rc;
}
static int queue_close(void *u, h2_pal_queue_t *q) {
  (void)u;
  if (!q)
    return H2_PAL_ERR_INVALID_ARG;
  pthread_mutex_lock(&q->mutex);
  q->closed = 1;
  pthread_cond_broadcast(&q->changed);
  pthread_mutex_unlock(&q->mutex);
  return 0;
}
static const h2_pal_queue_vtable_t queue_vtable = {.create = queue_create, .destroy = queue_destroy, .send = queue_send, .send_latest = queue_latest, .recv = queue_recv, .reset = queue_reset, .close = queue_close};
const h2_pal_queue_api_t gcl_queue = {NULL, &queue_vtable};

struct h2_pal_fs_file {
  int fd;
};
typedef struct {
  int root;
} filesystem;
// Walk directory descriptors to avoid both symlink escapes and realpath/open
// races. Each returned fd is owned by the caller.
static int fs_open_relative(filesystem *fs, const char *path) {
  if (!path || !*path || *path == '/' || strchr(path, '\\')) {
    errno = EINVAL;
    return -1;
  }
  int fd = dup(fs->root);
  if (fd < 0)
    return -1;
  const char *p = path;
  while (*p) {
    const char *slash = strchr(p, '/');
    size_t n = slash ? (size_t)(slash - p) : strlen(p);
    if (!n || n > NAME_MAX || (n == 1 && *p == '.') || (n == 2 && !memcmp(p, "..", 2))) {
      close(fd);
      errno = EINVAL;
      return -1;
    }
    char name[NAME_MAX + 1];
    memcpy(name, p, n);
    name[n] = 0;
    int next = openat(fd, name, O_RDONLY | O_CLOEXEC | O_NOFOLLOW | (slash ? O_DIRECTORY : O_NONBLOCK));
    int error = errno;
    close(fd);
    if (next < 0) {
      errno = error;
      return -1;
    }
    fd = next;
    if (!slash)
      return fd;
    p = slash + 1;
  }
  close(fd);
  errno = EINVAL;
  return -1;
}
static int fs_open(void *u, const char *path, h2_pal_fs_open_mode_t mode, h2_pal_fs_file_t **out) {
  if (!out)
    return H2_PAL_ERR_INVALID_ARG;
  *out = NULL;
  if (mode != H2_PAL_FS_OPEN_READ)
    return H2_PAL_ERR_UNSUPPORTED;
  int fd = fs_open_relative(u, path);
  if (fd < 0)
    return result(errno);
  struct stat st;
  if (fstat(fd, &st)) {
    int rc = result(errno);
    close(fd);
    return rc;
  }
  if (!S_ISREG(st.st_mode)) {
    close(fd);
    return H2_PAL_ERR_INVALID_ARG;
  }
  h2_pal_fs_file_t *f = malloc(sizeof(*f));
  if (!f) {
    close(fd);
    return H2_PAL_ERR_NO_MEMORY;
  }
  f->fd = fd;
  *out = f;
  return 0;
}
static int fs_read(void *u, h2_pal_fs_file_t *f, void *p, size_t n, size_t *out) {
  (void)u;
  if (!f || (!p && n) || !out || n > SSIZE_MAX)
    return H2_PAL_ERR_INVALID_ARG;
  *out = 0;
  ssize_t read_size;
  do {
    read_size = read(f->fd, p, n);
  } while (read_size < 0 && errno == EINTR);
  if (read_size < 0)
    return result(errno);
  *out = (size_t)read_size;
  return 0;
}
static int fs_close(void *u, h2_pal_fs_file_t *f) {
  (void)u;
  if (!f)
    return H2_PAL_ERR_INVALID_ARG;
  int rc = close(f->fd) ? result(errno) : 0;
  free(f);
  return rc;
}
static int fs_stat(void *u, const char *path, h2_pal_fs_stat_t *out) {
  if (!out)
    return H2_PAL_ERR_INVALID_ARG;
  int fd = fs_open_relative(u, path);
  if (fd < 0)
    return result(errno);
  struct stat st;
  int rc = fstat(fd, &st) ? result(errno) : 0;
  close(fd);
  if (!rc)
    *out = (h2_pal_fs_stat_t){.size = (uint64_t)st.st_size, .is_dir = S_ISDIR(st.st_mode)};
  return rc;
}
static const h2_pal_fs_vtable_t fs_vtable = {.open = fs_open, .read = fs_read, .close = fs_close, .stat = fs_stat};
int gcl_fs_create(const char *root, h2_pal_fs_api_t *out) {
  if (!root || !out)
    return H2_PAL_ERR_INVALID_ARG;
  *out = (h2_pal_fs_api_t){0};
  int fd = open(root, O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
  if (fd < 0)
    return result(errno);
  filesystem *fs = malloc(sizeof(*fs));
  if (!fs) {
    close(fd);
    return H2_PAL_ERR_NO_MEMORY;
  }
  fs->root = fd;
  *out = (h2_pal_fs_api_t){fs, &fs_vtable};
  return 0;
}
void gcl_fs_destroy(h2_pal_fs_api_t *api) {
  if (!api || !api->user)
    return;
  filesystem *fs = api->user;
  close(fs->root);
  free(fs);
  *api = (h2_pal_fs_api_t){0};
}
