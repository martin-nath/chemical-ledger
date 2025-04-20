package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/avast/retry-go/v4"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// TODO: fix the code structure of this file, make it similar to the insert-data.go file
// TODO: Reuse functions, if it can be
// TODO: Check all the responses and make sure they are correct in its situations

func GetData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.JsonRes(w, http.StatusMethodNotAllowed, &utils.Resp{Error: utils.InvalidMethod})
		return
	}
	ctx := r.Context()

	logrus.Info("Received request to get data.")

	filters := &utils.Filters{}
	query := r.URL.Query()
	filters.Type = query.Get("type")
	filters.CompoundName = query.Get("compound")
	filters.FromDate = query.Get("fromDate")
	filters.ToDate = query.Get("toDate")

	logrus.Debugf("Parsed filters: %+v", filters)

	if filters.Type != utils.TypeIncoming && filters.Type != utils.TypeOutgoing && filters.Type != utils.TypeBoth && filters.Type != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."}`)
		logrus.Warnf("Invalid 'type' filter provided: %s", filters.Type)
		return
	}

	if filters.FromDate != "" && filters.ToDate != "" && filters.ToDate < filters.FromDate {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "Invalid date range: 'toDate' cannot be earlier than 'fromDate'."}`)
		logrus.Warn("Invalid date range provided.")
		return
	}

	var fromDate int64
	var toDate int64
	var err error
	if filters.FromDate != "" {
		fromDate, err = utils.UnixTimestamp(filters.FromDate)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "Invalid 'fromDate' filter. Please use a valid date in the format '02-01-2006'."}`)
			logrus.Warnf("Invalid 'fromDate' filter provided: %s", filters.FromDate)
			return
		}
		filters.FromDate = fmt.Sprintf("%d", fromDate)
	}

	if filters.ToDate != "" {
		toDate, err = utils.UnixTimestamp(filters.ToDate)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "Invalid 'toDate' filter. Please use a valid date in the format '02-01-2006'."}`)
			logrus.Warnf("Invalid 'toDate' filter provided: %s", filters.ToDate)
			return
		}
		filters.ToDate = fmt.Sprintf("%d", toDate)
	}

	var queryBuilder, countQueryBuilder strings.Builder
	filterArgs := make([]any, 0)

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

	countQueryBuilder.WriteString(`
SELECT COUNT(*)
FROM entry as e
JOIN compound as c ON e.compound_id = c.id
JOIN quantity as q ON e.quantity_id = q.id
WHERE 1=1`)

	if filters.Type == utils.TypeIncoming || filters.Type == utils.TypeOutgoing {
		queryBuilder.WriteString(" AND e.type = ?")
		countQueryBuilder.WriteString(" AND e.type = ?")
		filterArgs = append(filterArgs, filters.Type)
	}
	if filters.CompoundName != "" && filters.CompoundName != "all" {
		queryBuilder.WriteString(" AND c.name = ?")
		countQueryBuilder.WriteString(" AND c.name = ?")
		filterArgs = append(filterArgs, filters.CompoundName)
	}
	if filters.FromDate != "" {
		queryBuilder.WriteString(" AND e.date >= ?")
		countQueryBuilder.WriteString(" AND e.date >= ?")
		filterArgs = append(filterArgs, fromDate)
	}
	if filters.ToDate != "" {
		queryBuilder.WriteString(" AND e.date < ?")
		countQueryBuilder.WriteString(" AND e.date < ?")
		filterArgs = append(filterArgs, toDate)
	}

	queryBuilder.WriteString(" ORDER BY e.date DESC")

	queryStr := queryBuilder.String()
	countQueryStr := countQueryBuilder.String()

	logrus.Debugf("Data Query: %s, Args: %v", queryStr, filterArgs)
	logrus.Debugf("Count Query: %s, Args: %v", countQueryStr, filterArgs)

	var count int
	err = retry.Do(
		func() error {
			return db.Db.QueryRowContext(ctx, countQueryStr, filterArgs...).Scan(&count)
		},
		retry.Attempts(utils.MaxRetries+1),
		retry.Delay(utils.RetryDelay),
		retry.Context(ctx),
	)
	if err != nil {
		logrus.Errorf("Error executing count query: %v", err)

		// TODO: use an appropriate error message
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return
	}

	entries := make([]utils.GetEntry, count)
	i := 0
	var rows *sql.Rows

	err = retry.Do(
		func() error {
			var queryErr error
			rows, queryErr = db.Db.QueryContext(ctx, queryStr, filterArgs...)
			if queryErr != nil {
				return queryErr
			}
			defer rows.Close()

			for rows.Next() {
				var e utils.GetEntry
				scanErr := rows.Scan(
					&e.ID, &e.Type, &e.Date,
					&e.Remark, &e.VoucherNo, &e.NetStock,
					&e.CompoundName, &e.Scale,
					&e.NumOfUnits, &e.QuantityPerUnit,
				)
				if scanErr != nil {
					return fmt.Errorf("error scanning data row: %w", scanErr)
				}
				entries[i] = e
				i++
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("error during rows iteration: %w", err)
			}
			return nil
		},
		retry.Attempts(utils.MaxRetries+1),
		retry.Delay(utils.RetryDelay),
		retry.Context(ctx),
	)

	if err != nil {
		logrus.Errorf("Error executing data query: %v", err)

		// TODO: use an appropriate error message
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return
	}

	// firstErr := <-errCh
	// if firstErr != nil {
	// 	logrus.Errorf("Error fetching data: %v", firstErr)
	// 	if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
	// 		http.Error(w, `{"error": "Request cancelled or timed out."}`, http.StatusServiceUnavailable)
	// 	} else {
	// 		http.Error(w, `{"error": "Failed to retrieve data. Please try again later."}`, http.StatusInternalServerError)
	// 	}
	// 	return
	// }

	logrus.Infof("Successfully retrieved %d entries (total matching count: %d).", len(entries), count)

	response := map[string]interface{}{
		"total":   count,
		"results": entries,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logrus.Errorf("Error encoding JSON response: %v", err)
	}
}
