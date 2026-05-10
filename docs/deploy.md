# Deploy

`goflex build` produces a production binary that embeds the compiled frontend and serves the API from the same process.

```sh
goflex build --out ./bin/app
PORT=8080 DATABASE_URL='sqlite://app.db' ./bin/app
```

The binary serves:

- `/` and SPA fallback routes from embedded `dist/index.html` with `Cache-Control: no-cache`.
- `/dist/app.<hash>.js`, `/dist/app.<hash>.css`, and copied `assets/*` with `Cache-Control: public, max-age=31536000, immutable`.
- precompressed `.br` / `.gz` assets when the browser sends `Accept-Encoding`.
- `/api/healthz` for health checks.

## Build flags

```sh
goflex build --out ./bin/app --minify --target linux/amd64
```

- `--out`: output binary path, default `bin/app`.
- `--minify`: minify frontend assets, default `true`.
- `--target`: optional `GOOS/GOARCH` or comma-separated list such as `linux/amd64,linux/arm64,darwin/arm64`.

Builds use `-trimpath`, content-hashed asset filenames, and honor `SOURCE_DATE_EPOCH` for reproducible timestamps.

## Environment variables

- `PORT`: listen port, default `8080`.
- `DATABASE_URL`: application database DSN when your app uses `pkg/db`.
- `GOFLEX_ENV=prod`: recommended for app code that checks environment.

## systemd example

```ini
[Unit]
Description=GoFlex app
After=network.target

[Service]
User=goflex
WorkingDirectory=/opt/goflex-app
Environment=PORT=8080
Environment=GOFLEX_ENV=prod
Environment=DATABASE_URL=postgres://user:pass@localhost/app?sslmode=disable
ExecStart=/opt/goflex-app/app
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

## Docker example

```dockerfile
FROM golang:1.23 AS build
WORKDIR /src
COPY . .
RUN go install ./cmd/goflex && goflex build --out /out/app

FROM gcr.io/distroless/static-debian12
ENV PORT=8080 GOFLEX_ENV=prod
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
```

## Troubleshooting

- If GopherJS reports a Go version mismatch, use the Go toolchain version required by your installed GopherJS release for the frontend build lane.
- If assets appear stale, verify the HTML references `/dist/app.<hash>.js` and `/dist/app.<hash>.css`; fingerprints change when content changes.
- For port conflicts, set `PORT` to an available port or use your process manager's socket activation/reverse proxy.
