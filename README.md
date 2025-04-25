# Chemical Ledger Backend

This is the backend for the Chemical Ledger application. It is written in Go and uses the [Chi](https://github.com/go-chi/chi) web framework.

## Api Endpoints

### POST /insert-compound

Inserts a new compound into the database.

### GET /get-compound

Retrieves all compounds from the database.

### PUT /update-compound

Updates an existing compound in the database.

### POST /insert-entry

Inserts a new entry into the database.

### GET /get-entry

Retrieves all entries from the database.

### PUT /update-entry

Updates an existing entry in the database.

## Database Schema

The database schema is defined in the `db/create-tables.sql` file. It includes tables for compounds and entries, as well as a table for quantities.
