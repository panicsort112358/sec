# sec

## Build and install

Build the executable in the repository directory:

```sh
go build -o sec ./cmd/sec
```

Install it into your Go binary directory (usually `$(go env GOPATH)/bin`):

```sh
go install ./cmd/sec
```

Make sure that directory is on your `PATH` to run `sec` from anywhere.

Set `SEC_USER_AGENT` to a descriptive agent string with a contact email before
running commands, or pass the same value with `-ua`.

Read a filing as prose:

```sh
./sec -ticker AAPL -mode document -form 10-K
```

Choose a filing by accession number:

```sh
./sec -ticker AAPL -mode document -accession 0000320193-24-000123
```

Read extracted XBRL facts as JSON:

```sh
./sec -ticker AAPL -mode facts -form 10-K
```

Set `SEC_USER_AGENT` to a descriptive agent string with a contact email, or
pass `-ua 'Research App contact@example.com'`.
