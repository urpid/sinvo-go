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

The app starts a local server and opens the browser automatically.

Default local address:

```text
http://127.0.0.1:8123
```

## Build

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

## Releases

Compiled files should be published through GitHub Releases, not committed directly into the repository.

Suggested release assets:

```text
sinvo-go-v0.1.0-linux-amd64.zip
sinvo-go-v0.1.0-linux-arm64.zip
sinvo-go-v0.1.0-windows-amd64.zip
```

Do not include local runtime data such as `data/sinvo-go.sqlite`, `backups/`, or `exports/` in release archives.

## Security Notes

Sinvo Go is designed as a local desktop-style app. It binds to `127.0.0.1:8123` and does not include user accounts or API authentication.

Do not expose the app directly to a public network.

## License

Add your chosen license here before publishing.

The original Invio project is released under the Unlicense.
