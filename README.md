# Sinvo Go

Sinvo Go stands for Simple Invoice Go. It is a local invoice management app written in Go, with a browser-based interface, SQLite storage, invoice templates, exports, backups, and basic customer/product management.

This project is an independent Go rewrite inspired by [Invio](https://github.com/kittendevv/Invio). Invio is released under the Unlicense. Sinvo Go is not affiliated with or endorsed by the Invio project.

## Features

**1** Manage customers, products, product categories, units, and tax definitions.

**2** Create draft invoices and move them through sent, paid, overdue, and voided states.

**3** Generate invoice numbers with configurable prefixes, year segments, padding, or custom patterns.

**4** Export invoices as HTML, PDF, XML, JSON, and CSV.

**5** Render invoice templates with company branding, payment details, tax summaries, and optional logos.

**6** Support UBL 2.1, XRechnung, Factur-X/ZUGFeRD, and FatturaPA-style XML output.

**7** Store data locally in SQLite beside the application binary.

## Requirements

**1** Go 1.23 or newer.

**2** A modern browser.

**3** Optional for PDF export: Chrome/Chromium, wkhtmltopdf, WeasyPrint, or a renderer path provided through `SINVO_GO_PDF_RENDERER`.

## Run From Source

```sh
go run .
```

The app starts a server and opens the browser automatically.

Default address:

```text
http://0.0.0.0:8123
```

On the same machine, open:

```text
http://127.0.0.1:8123
```

From another device in the same LAN, open:

```text
http://<this-computer-lan-ip>:8123
```

## Authentication

Sinvo Go reads optional HTTP Basic Auth credentials from `config.json` beside `main.go`:

```json
{
  "username": "admin",
  "password": "admin"
}
```

Authentication is enabled only when `config.json` exists and both `username` and `password` are set to non-empty values. If the file is missing, or either value is missing or empty, the app runs without authentication.

Malformed `config.json` stops startup with an error.

## Build

Build all supported targets into `builds/`:

```sh
./build.sh
```

This creates:

```text
builds/sinvo-go-linux-amd64
builds/sinvo-go-linux-arm64
builds/sinvo-go-windows-amd64.exe
```

Single-target builds:

Linux amd64:

```sh
GOOS=linux GOARCH=amd64 go build -o sinvo-go-linux-amd64 .
```

Linux arm64:

```sh
GOOS=linux GOARCH=arm64 go build -o sinvo-go-linux-arm64 .
```

Windows amd64:

```sh
GOOS=windows GOARCH=amd64 go build -o sinvo-go-windows-amd64.exe .
```

## Data Storage

Sinvo Go stores runtime data next to the executable:

```text
data/sinvo-go.sqlite
backups/
exports/
```

These files are user data and generated output. They should not be committed to the source repository.

## PDF Export

PDF export uses an external HTML-to-PDF renderer. Sinvo Go checks these locations and commands:

```text
SINVO_GO_PDF_RENDERER
tools/<arch>-<os>/
google-chrome-stable
google-chrome
chromium
chromium-browser
msedge
wkhtmltopdf
weasyprint
```

For portable release archives, place `chrome-headless-shell` in a `tools` subfolder next to the Sinvo Go binary. The platform folder name is built from Go's runtime values: `<GOARCH>-<GOOS>`.

Expected `chrome-headless-shell` locations:

```text
tools/amd64-linux/chrome-headless-shell
tools/arm64-linux/chrome-headless-shell
tools/amd64-windows/chrome-headless-shell.exe
```

Linux builds need the renderer file to be executable.

Example release archive layout:

```text
sinvo-go-linux-amd64/
  sinvo-go-linux-amd64
  tools/
    amd64-linux/
      chrome-headless-shell
```

If no renderer is available, the app still runs, but PDF export fails with an error.

## Security Notes

Sinvo Go binds to `0.0.0.0:8123` so it can be reached from other devices in the same LAN. Enable HTTP Basic Auth through `config.json` before using it on a shared network.

Do not expose the app directly to a public network.

## License

MIT License. See [LICENSE](LICENSE).

The original Invio project is released under the Unlicense.
