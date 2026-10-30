# RustRush brand assets

The website uses `rustrush-logo-transparent.png` for its wordmark and favicon
source. `rustrush-github-banner.png` is the 2172×724 GitHub README and Open Graph
banner; it has genuine alpha transparency and is composited cleanly over dark or
light backgrounds. The favicon is tightly cropped to the logo mark; the PNG is
32×32, and the ICO embeds 16×16 and 32×32 images. The opaque square-logo variant
is retained but not displayed.

Vite serves this directory directly as static files during development and copies
it to the website build output. Every file here is public; do not place secrets or
private files in this directory. No duplicate copies are maintained.
