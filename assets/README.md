# RustRush brand assets

These original PNGs are preserved in place. The website uses
`rustrush-logo-transparent.png` for its wordmark and references
`rustrush-github-banner.png` as its Open Graph image. The transparent logo has an
alpha channel and supplies the source for `favicon.png` and `favicon.ico`. The
favicon is tightly cropped to the logo mark; the PNG is 32×32, and the ICO embeds
16×16 and 32×32 images. The square-logo and banner variants are opaque RGB images.
The opaque square variant is retained but not displayed.

Vite serves this directory directly as static files during development and copies
it to the website build output. Every file here is public; do not place secrets or
private files in this directory. No duplicate copies are maintained.
