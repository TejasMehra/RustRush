# RustRush website

The website is a standalone React, TypeScript, and Vite frontend. It is intentionally
static: it does not run benchmarks in the browser, call a backend, collect analytics,
or require API keys. The CLI remains a separate Go application at the repository root.

## Requirements

- Node.js 20.15 or newer
- npm 10 or newer

## Development

From this directory:

```powershell
npm ci
npm run dev
```

Vite prints a local development URL. Production output is generated with:

```powershell
npm run typecheck
npm run build
npm run preview
```

The build is portable under a relative base path. Existing brand images are served
from the repository's `assets/` directory; the alpha-transparent square logo is used
in the navigation and footer, while the existing wide banner is the Open Graph image.

The primary website content lives in `src/App.tsx` and `src/styles.css`. Update the
root README and methodology documents when CLI behavior changes; do not describe
unimplemented browser execution as a website feature.
