// Adapter for cooperative cancellation of the GizOS VM Core. The VM stays on
// its owning isolate; only the atomic cancellation flag crosses threads.
#define _POSIX_C_SOURCE 200809L
#include "h2_lua_internal.h"
#include "lauxlib.h"
#include <stdatomic.h>
#include <stdlib.h>
#include <time.h>

typedef struct {
  atomic_int cancelled;
  struct timespec started;
  int timeout_ms;
} control;
static void check(lua_State *state, lua_Debug *ar) {
  (void)ar;
  control *c = *(control **)lua_getextraspace(state);
  struct timespec now;
  clock_gettime(CLOCK_MONOTONIC, &now);
  double elapsed = (now.tv_sec - c->started.tv_sec) * 1000.0 + (now.tv_nsec - c->started.tv_nsec) / 1000000.0;
  if (atomic_load(&c->cancelled) || elapsed >= c->timeout_ms) {
    lua_sethook(state, check, LUA_MASKCOUNT, 1);
    luaL_error(state, "execution cancelled or timed out");
  }
}
control *gizclaw_lua_control_create(int timeout_ms) {
  control *c = calloc(1, sizeof(*c));
  if (c) {
    atomic_init(&c->cancelled, 0);
    c->timeout_ms = timeout_ms;
    clock_gettime(CLOCK_MONOTONIC, &c->started);
  }
  return c;
}
void gizclaw_lua_control_attach(h2_lua_vm_t *vm, control *c) {
  *(control **)lua_getextraspace(vm->state) = c;
  lua_sethook(vm->state, check, LUA_MASKCOUNT, 1000);
}
void gizclaw_lua_control_cancel(control *c) { atomic_store(&c->cancelled, 1); }
void gizclaw_lua_control_free(control *c) { free(c); }
