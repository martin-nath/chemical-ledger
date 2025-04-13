package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/martin-nath/chemical-ledger/db"
)

func GetData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	filters := &Filters{}

	if val, ok := r.URL.Query()["type"]; ok && len(val[0]) > 0 {
		filters.Type = val[0]
	}
	if val, ok := r.URL.Query()["compound"]; ok && len(val[0]) > 0 {
		filters.CompoundName = val[0]
	}
	if val, ok := r.URL.Query()["fromDate"]; ok && len(val[0]) > 0 {
		filters.FromDate = val[0]
	}
	if val, ok := r.URL.Query()["toDate"]; ok && len(val[0]) > 0 {
		filters.ToDate = val[0]
	}

	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
SELECT
	e.id, e.type, e.date, e.remark, e.voucher_no, e.net_stock,
	c.name, c.scale,
	q.num_of_units, q.quantity_per_unit
FROM entry as e
JOIN compound as c ON e.compound_id = c.id
JOIN quantity as q ON e.quantity_id = q.id
WHERE 1=1
`)

	countQueryBuilder := strings.Builder{}
	countQueryBuilder.WriteString(`
SELECT COUNT(*)
FROM entry as e
JOIN compound as c ON e.compound_id = c.id
JOIN quantity as q ON e.quantity_id = q.id
WHERE 1=1
`)

	filterArgs := make([]any, 4)
	ind := 0

	if filters.Type == "incoming" || filters.Type == "outgoing" {
		queryBuilder.WriteString(" AND e.type = ?")
		countQueryBuilder.WriteString(" AND e.type = ?")
		filterArgs[ind] = filters.Type
		ind++
	}
	if filters.CompoundName != "" && filters.CompoundName != "all" {
		queryBuilder.WriteString(" AND c.name = ?")
		countQueryBuilder.WriteString(" AND c.name = ?")
		filterArgs[ind] = filters.CompoundName
		ind++
	}
	if filters.FromDate != "" {
		queryBuilder.WriteString(" AND e.date >= ?")
		countQueryBuilder.WriteString(" AND e.date >= ?")
		filterArgs[ind] = filters.FromDate
		ind++
	}
	if filters.ToDate != "" {
		queryBuilder.WriteString(" AND e.date <= ?")
		countQueryBuilder.WriteString(" AND e.date <= ?")
		filterArgs[ind] = filters.ToDate
		ind++
	}

	queryBuilder.WriteString(" ORDER BY e.date DESC")

	query := queryBuilder.String()
	countQuery := countQueryBuilder.String()

	wg := sync.WaitGroup{}
	count := make(chan int, 1)
	errCh := make(chan error, 1)

	wg.Add(1)
	go func(q string, filterArgs []any, db *sql.DB) {
		defer wg.Done()
		c := 0

		err := db.QueryRow(q, filterArgs...).Scan(&c)
		if err != nil {
			errCh <- err
			return
		}
		count <- c
	}(countQuery, filterArgs, db.Db)

	rows, err := db.Db.Query(query, filterArgs...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	wg.Wait()
	close(errCh)
	close(count)

	if err := <-errCh; err != nil {
		http.Error(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	entries := make([]Entry, <-count)
	ind = 0

	defer rows.Close()

	for rows.Next() {
		var e Entry

		err := rows.Scan(
			&e.ID, &e.Type, &e.Date, &e.Remark, &e.VoucherNo, &e.NetStock,
			&e.CompoundName, &e.Scale,
			&e.NumOfUnits, &e.QuantityPerUnit,
		)
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		entries[ind] = e
		ind++
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "rows iteration error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		http.Error(w, "json encoding error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
