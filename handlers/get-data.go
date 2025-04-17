package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/avast/retry-go/v4"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

func GetData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet) // Inform client which method is allowed
		w.WriteHeader(http.StatusMethodNotAllowed)
		// Consistently use JSON for error responses
		fmt.Fprint(w, `{"error": "Method Not Allowed. Please use GET."}`)
		return
	}

	// Use request context for cancellation propagation
	ctx := r.Context()

	logrus.Info("Received request to get data.")

	filters := &utils.Filters{}

	// Get filter parameters from the URL
	query := r.URL.Query()
	filters.Type = query.Get("type")
	filters.CompoundName = query.Get("compound")
	filters.FromDate = query.Get("fromDate") // Assuming format like 'YYYY-MM-DD'
	filters.ToDate = query.Get("toDate")     // Assuming format like 'YYYY-MM-DD'

	logrus.Debugf("Parsed filters: %+v", filters)

	// Validate the 'type' filter
	if filters.Type != "incoming" && filters.Type != "outgoing" && filters.Type != "both" && filters.Type != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."}`)
		logrus.Warnf("Invalid 'type' filter provided: %s", filters.Type)
		return
	}

	// Validate the date range (basic check)
	// Note: Further validation might be needed depending on expected date formats
	if filters.FromDate != "" && filters.ToDate != "" && filters.ToDate < filters.FromDate {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "Invalid date range: 'toDate' cannot be earlier than 'fromDate'."}`)
		logrus.Warn("Invalid date range provided.")
		return
	}

	// --- Query Building ---
	var queryBuilder, countQueryBuilder strings.Builder
	filterArgs := make([]any, 0)

	// Base SELECT for data
	// Assumes e.date stores Unix timestamps (as TEXT or INTEGER).
	// Outputting as localized time string.
	queryBuilder.WriteString(`
SELECT
	e.id, e.type, datetime(e.date, 'unixepoch', 'localtime') AS formatted_date,
	e.remark, e.voucher_no, e.net_stock,
	c.name, c.scale,
	q.num_of_units, q.quantity_per_unit
FROM entry as e
JOIN compound as c ON e.compound_id = c.id
JOIN quantity as q ON e.quantity_id = q.id
WHERE 1=1`)

	// Base SELECT for count
	countQueryBuilder.WriteString(`
SELECT COUNT(*)
FROM entry as e
JOIN compound as c ON e.compound_id = c.id
JOIN quantity as q ON e.quantity_id = q.id
WHERE 1=1`)

	// Apply filters dynamically
	if filters.Type == "incoming" || filters.Type == "outgoing" {
		queryBuilder.WriteString(" AND e.type = ?")
		countQueryBuilder.WriteString(" AND e.type = ?")
		filterArgs = append(filterArgs, filters.Type)
	}
	if filters.CompoundName != "" && filters.CompoundName != "all" {
		queryBuilder.WriteString(" AND c.name = ?")
		countQueryBuilder.WriteString(" AND c.name = ?")
		filterArgs = append(filterArgs, filters.CompoundName)
	}
	// *** Corrected Date Filtering ***
	// Assumes e.date stores Unix Timestamps (as TEXT or INTEGER).
	// Converts filter dates (YYYY-MM-DD strings) to Unix timestamps for comparison.
	if filters.FromDate != "" {
		queryBuilder.WriteString(" AND e.date >= unixepoch(?)")
		countQueryBuilder.WriteString(" AND e.date >= unixepoch(?)")
		filterArgs = append(filterArgs, filters.FromDate) // Pass YYYY-MM-DD string
	}
	if filters.ToDate != "" {
		// To include the entire 'toDate', we find the start of the *next* day
		// and check if e.date is less than that.
		// Alternatively, add ' 23:59:59' if precision is needed, but < next day start is safer.
		queryBuilder.WriteString(" AND e.date < unixepoch(?, '+1 day')")
		countQueryBuilder.WriteString(" AND e.date < unixepoch(?, '+1 day')")
		// Or, if you store dates accurately and want to include the whole day:
		// queryBuilder.WriteString(" AND e.date <= unixepoch(?, ' 23:59:59')") // Less common for unix timestamps
		// countQueryBuilder.WriteString(" AND e.date <= unixepoch(?, ' 23:59:59')")
		filterArgs = append(filterArgs, filters.ToDate) // Pass YYYY-MM-DD string
	}

	// Order results
	queryBuilder.WriteString(" ORDER BY e.date DESC") // Order by original timestamp column

	queryStr := queryBuilder.String()
	countQueryStr := countQueryBuilder.String()

	logrus.Debugf("Data Query: %s, Args: %v", queryStr, filterArgs)
	logrus.Debugf("Count Query: %s, Args: %v", countQueryStr, filterArgs)

	// --- Concurrent Data Fetching ---
	var wg sync.WaitGroup
	// Use buffered channels to avoid blocking if one goroutine finishes early
	countCh := make(chan int, 1)
	entriesCh := make(chan []utils.Entry, 1)
	// Channel to signal the first error encountered
	errCh := make(chan error, 2) // Buffer size 2, one for each goroutine

	// Fetch total count concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		var count int
		err := retry.Do(
			func() error {
				// Use QueryRowContext with request context
				return db.Db.QueryRowContext(ctx, countQueryStr, filterArgs...).Scan(&count)
			},
			retry.Attempts(utils.MaxRetries+1),
			retry.Delay(utils.RetryDelay),
			retry.Context(ctx), // Allow retry to be cancelled by context
		)
		if err != nil {
			// Use non-blocking send in case errCh is full or already closed
			select {
			case errCh <- fmt.Errorf("error executing count query: %w", err):
			default: // Don't block if error channel is full/closed
			}
			return
		}
		countCh <- count
	}()

	// Fetch data entries concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		var entries []utils.Entry
		var rows *sql.Rows // Declare rows outside retry loop

		err := retry.Do(
			func() error {
				var queryErr error
				// Use QueryContext with request context
				rows, queryErr = db.Db.QueryContext(ctx, queryStr, filterArgs...)
				if queryErr != nil {
					return queryErr // Propagate error for retry logic
				}
				// Important: Defer Close *after* successful query, but handle potential nil rows if error occurred before assignment
				// However, QueryContext should return error if it fails, so rows shouldn't be nil here on success.
				// A panic here would indicate an issue in the sql driver or QueryContext itself.
				defer rows.Close() // Ensure rows are closed even if scanning fails below

				for rows.Next() {
					var e utils.Entry
					// Scan into the Entry struct fields
					// Note the change: Scan into e.Date which should be string or time.Time
					// depending on utils.Entry definition. Assuming string for formatted_date.
					scanErr := rows.Scan(
						&e.ID, &e.Type, &e.Date, // Assuming e.Date is string to receive formatted_date
						&e.Remark, &e.VoucherNo, &e.NetStock,
						&e.CompoundName, &e.Scale,
						&e.NumOfUnits, &e.QuantityPerUnit,
					)
					if scanErr != nil {
						// Return error to potentially trigger a retry if it's recoverable,
						// though scan errors are usually not transient.
						return fmt.Errorf("error scanning data row: %w", scanErr)
					}
					entries = append(entries, e)
				}
				// Check for errors encountered during iteration
				if err := rows.Err(); err != nil {
					return fmt.Errorf("error during rows iteration: %w", err)
				}
				// If everything succeeded, return nil for retry.Do
				return nil
			},
			retry.Attempts(utils.MaxRetries+1),
			retry.Delay(utils.RetryDelay),
			retry.Context(ctx), // Allow retry to be cancelled by context
		)

		// After retry loop, check final error status
		if err != nil {
			select {
			case errCh <- fmt.Errorf("error executing data query: %w", err):
			default:
			}
			return // Don't send potentially partial results
		}

		// Send successfully retrieved entries
		entriesCh <- entries
	}()

	// Wait for both goroutines to complete
	wg.Wait()
	// Close channels to signal completion to receivers
	close(countCh)
	close(entriesCh)
	close(errCh) // Close errCh last

	// Check if any errors were sent
	// Read the first error encountered (if any)
	firstErr := <-errCh
	if firstErr != nil {
		logrus.Errorf("Error fetching data: %v", firstErr)
		// Check if context was cancelled
		if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
			http.Error(w, `{"error": "Request cancelled or timed out."}`, http.StatusServiceUnavailable) // Or 499 Client Closed Request if detectable
		} else {
			// Generic internal server error for database or other issues
			http.Error(w, `{"error": "Failed to retrieve data. Please try again later."}`, http.StatusInternalServerError)
		}
		return
	}

	// If no errors, retrieve results from channels
	totalCount := <-countCh
	entriesResult := <-entriesCh

	logrus.Infof("Successfully retrieved %d entries (total matching count: %d).", len(entriesResult), totalCount)

	// Prepare JSON response
	response := map[string]interface{}{
		"total":   totalCount,    // Total count matching filters
		"results": entriesResult, // The actual data entries fetched (could be a subset if pagination were added)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error, but response likely already partially sent
		logrus.Errorf("Error encoding JSON response: %v", err)
		// Avoid writing header again if already written or partially written
		// http.Error might panic if headers are already sent. Best effort logging.
	}
}
