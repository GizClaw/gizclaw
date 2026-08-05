import assert from "node:assert/strict";
import test from "node:test";

import { matchRoutes } from "react-router-dom";

import {
  decodeRouteParam,
  encodeGoPathSegment,
  encodeRouteParam,
  membershipResourceID,
} from "../views/admin/full/lib/route-param.ts";

for (const id of ["plain", "space id", "a/b", "a%2Fb", "100%25"]) {
  test(`opaque Admin route parameter round trips ${id}`, () => {
    const path = `/resources/${encodeRouteParam(id)}`;
    const parameter = matchRoutes([{ path: "/resources/:id" }], path)?.[0]
      ?.params.id;

    assert.equal(decodeRouteParam(parameter ?? ""), id);
  });
}

test("Go path segment encoding matches net/url.PathEscape", () => {
  assert.equal(
    encodeGoPathSegment("a+b@c&d=e$f:g/h,;?!'()*"),
    "a+b@c&d=e$f:g%2Fh%2C%3B%3F%21%27%28%29%2A",
  );
});

test("membership resource IDs escape component separators", () => {
  assert.equal(
    membershipResourceID("family:blue/team", "Peer:Key/One"),
    "family%3Ablue%2Fteam:Peer%3AKey%2FOne",
  );
});
