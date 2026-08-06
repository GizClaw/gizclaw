export function encodeRouteParam(value: string): string {
  return encodeURIComponent(encodeURIComponent(value));
}

export function decodeRouteParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export function encodeGoPathSegment(value: string): string {
  return encodeURIComponent(value)
    .replace(
      /[!'()*]/g,
      (character) => `%${character.charCodeAt(0).toString(16).toUpperCase()}`,
    )
    .replace(/%(24|26|2B|3A|3D|40)/g, (escape) =>
      String.fromCharCode(Number.parseInt(escape.slice(1), 16)),
    );
}

export function membershipResourceID(
  groupID: string,
  peerPublicKey: string,
): string {
  const escapeComponent = (value: string): string =>
    encodeGoPathSegment(value).replaceAll(":", "%3A");
  return `${escapeComponent(groupID)}:${escapeComponent(peerPublicKey)}`;
}
