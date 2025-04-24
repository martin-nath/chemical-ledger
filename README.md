# Chemical Ledger Backend

This is the backend for the Chemical Ledger application. It is written in Go and uses the [http](https://pkg.go.dev/net/http) package to handle HTTP requests and responses.

## Features

- Insert data
- Retrieve data
- Update data
- Retrieve compound names

## Run locally

To run the backend, you need to have Go installed on your machine. You can then run the following command to build and run the backend:

```bash
go run backend.go
```

## Data Storage

The backend uses a SQLite database to store data. The database file is located at `chemical_ledger.db` in the same directory as the backend executable.