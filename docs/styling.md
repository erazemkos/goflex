# Styling

GoFlex defaults to Tailwind CSS without requiring `npm install` in application projects.

## Default Tailwind flow

New apps include `tailwind.config.css`:

```css
@import "tailwindcss";
@source "./**/*.go";
```

`goflex build` and `goflex dev` run the CSS pipeline when a `tailwind.config.css` or `tailwind.config.js` file is present. The framework scans Go source for literals passed to `ui.Class`, `ui.ClassIf`, `ui.ClassMap`, and `ui.Tw`, writes a deterministic safelist, and emits `dist/app.css`.

The Tailwind standalone binary is downloaded once into `~/.cache/goflex/` and reused on later builds. If the binary is unavailable, GoFlex still emits a deterministic fallback stylesheet for common utilities so builds keep working.

## Class helpers

```go
ui.Div(
    ui.Class("p-4 text-red-500"),
    ui.ClassIf(user.Admin, "bg-blue-500"),
    ui.ClassMap(map[string]bool{"opacity-50": disabled}),
)
```

Use `ui.Tw` when caller-provided classes should override defaults:

```go
ui.Button(ui.Class(ui.Tw("px-2 py-1 bg-red-500", props.Class)))
```

Examples:

- `ui.Tw("px-2", "px-4")` -> `px-4`
- `ui.Tw("p-2", "px-4")` -> `p-2 px-4`
- `ui.Tw("text-red-500", "text-blue-500")` -> `text-blue-500`

## Disabling Tailwind

Delete `tailwind.config.css` / `tailwind.config.js` to disable the Tailwind step. `goflex build` still compiles the app and simply skips CSS generation.

## Custom CSS and assets

Put plain CSS or other static files in `assets/`. Build tooling fingerprints them into `dist/assets/`, for example:

```text
assets/app.css -> dist/assets/app-<hash>.css
```

Hashed assets are served with a one-year immutable cache header. You can link custom CSS from your HTML or generated components as needed.
