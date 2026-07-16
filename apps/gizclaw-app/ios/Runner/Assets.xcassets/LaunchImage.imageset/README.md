# GizClaw Launch Screen Assets

The launch screen uses an original, text-free abstract image generated for
GizClaw. Large asymmetric gradient fields and precise mathematical curves form
the complete composition. The artwork must remain free of text, logos, icons,
mascots, animals, people, and other recognizable subjects.

`LaunchImage-iPad@2x.png` is the canonical committed raster master. It is an
RGB PNG with a `2048x2732` pixel, approximately 3:4 canvas. iPad launch screens
show the complete composition. iPhone assets use a centered narrow crop so the
curves and focal intersection remain visible on tall phone displays.

Generate the iPhone assets from this directory on macOS:

```sh
sips -c 2732 1261 LaunchImage-iPad@2x.png \
  --out /tmp/LaunchImage-iPhone-crop.png
sips -z 1864 860 /tmp/LaunchImage-iPhone-crop.png \
  --out LaunchImage-iPhone@2x.png
sips -z 2796 1290 /tmp/LaunchImage-iPhone-crop.png \
  --out LaunchImage-iPhone@3x.png
```

The committed files must remain RGB PNGs:

- `LaunchImage-iPhone@2x.png`: `860x1864` pixels for 2x iPhones.
- `LaunchImage-iPhone@3x.png`: `1290x2796` pixels for 3x iPhones.
- `LaunchImage-iPad@2x.png`: `2048x2732` pixels for 2x iPads.

`LaunchScreen.storyboard` pins the image to every viewport edge and renders it
with aspect fill. The `#F5F6F2` view background is only a fallback behind the
opaque artwork and is not visible during a correctly rendered launch screen.
