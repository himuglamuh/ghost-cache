# Ghost Cache Presentation

This directory contains the self-contained Marp deck for Ghost Cache. The HTML build supports local demo video; the PDF is a static fallback with descriptive poster frames.

## Build

```bash
cd presentation
npm install
npm run build
npm run pdf
```

Outputs:

- `ghost-cache.html`: primary local presentation
- `ghost-cache.pdf`: static fallback

Serve locally:

```bash
npm run serve
```

Open the local Marp URL in a Chromium-family browser. The deck uses only local assets and system-safe font fallbacks; no Internet connection is required to present it.

Presentation dependencies are development-only and are not used by `ghost-node` or application CI. Build the deck from trusted local Markdown and assets.

## Presenter Notes

Speaker notes are point-form HTML comments at the end of each Markdown slide. Marp embeds them in the HTML. In the browser presentation, press `P` to open Presenter View.

Slide 4's file-transfer estimates are generated from the current protocol/scheduler model:

```bash
python ./transfer-model.py
```

They are clean-link transfer-phase estimates, not measured end-to-end throughput.

## Video

The optimized MP4 files are versioned with the deck so the complete presentation can move between computers without a separate media-copy step.

Each demo slide contains:

- a local `<video controls>` element for HTML;
- a local SVG poster used before playback and when video is unavailable;
- a descriptive fallback caption;
- a PDF print rule that removes the video element and retains the poster.

The videos use standard H.264/AAC MP4 encoding for broad browser compatibility. Rendered HTML and PDF are versioned with their Markdown, CSS, SVG assets, and dependency lock.
