package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type GetEntryReq struct {
	Type         string `json:"entry_type"`
	CompoundId   string `json:"compound_id"`
	FromDate     string `json:"from_date"`
	ToDate       string `json:"to_date"`
	Transactions string `json:"transactions"`
}

func GetEntryHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &GetEntryReq{
		Type:         utils.GetParam(r, "entry_type"),
		CompoundId:   utils.GetParam(r, "compound_id"),
		FromDate:     utils.GetParam(r, "from_date"),
		ToDate:       utils.GetParam(r, "to_date"),
		Transactions: utils.GetParam(r, "transactions"),
	}

	if errStr := validateGetEntryReq(reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	filterQuery, countQuery, filterArgs := buildGetEntryQueries(reqBody)

	wg := sync.WaitGroup{}
	wg.Add(1)
	countCh := make(chan int, 1)
	errCh := make(chan error, 1)

	go func() {
		defer wg.Done()
		count := 0
		errCh <- db.Conn.QueryRow(countQuery, filterArgs...).Scan(&count)
		countCh <- count
	}()

	type Entry struct {
		Id          string `json:"id"`
		Type        string `json:"type"`
		Date        string `json:"date"`
		Remark      string `json:"remark"`
		VoucherNo   string `json:"voucher_no"`
		NetStock    int    `json:"net_stock"`
		CompoundId  string `json:"compound_id"`
		Name        string `json:"name"`
		Scale       string `json:"scale"`
		NumOfUnits  int    `json:"num_of_units"`
		QuantityPer int    `json:"quantity_per_unit"`
	}

	rows, err := db.Conn.Query(filterQuery, filterArgs...)
	if err != nil {
		slog.Error("failed to query entry data", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}

	wg.Wait()
	close(countCh)
	close(errCh)
	if err := <-errCh; err != nil {
		slog.Error("failed to scan count of entries", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}

	data := make([]*Entry, <-countCh)
	i := 0

	for rows.Next() {
		entry := &Entry{}
		if err := rows.Scan(
			&entry.Id, &entry.Type, &entry.Date, &entry.Remark, &entry.VoucherNo, &entry.NetStock,
			&entry.CompoundId, &entry.Name, &entry.Scale,
			&entry.NumOfUnits, &entry.QuantityPer); err != nil {
			slog.Error("failed to scan entry row", "error", err)
			utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
			return
		}
		data[i] = entry
		i++
	}

	utils.RespWithData(w, http.StatusOK, data)
}

func validateGetEntryReq(reqBody *GetEntryReq) utils.ErrorMessage {
	if reqBody.Type == "" || reqBody.CompoundId == "" || reqBody.FromDate == "" || reqBody.ToDate == "" {
		slog.Error("missing required fields", "entry_type", reqBody.Type, "compound_id", reqBody.CompoundId, "from_date", reqBody.FromDate, "to_date", reqBody.ToDate)
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING && reqBody.Type != "both" {
		slog.Error("invalid entry type", "received", reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	if _, err := time.Parse("2006-01-02", reqBody.FromDate); err != nil {
		slog.Error("invalid from_date format", "from_date", reqBody.FromDate, "error", err)
		return utils.INVALID_DATE_FORMAT
	}
	if _, err := time.Parse("2006-01-02", reqBody.ToDate); err != nil {
		slog.Error("invalid to_date format", "to_date", reqBody.ToDate, "error", err)
		return utils.INVALID_DATE_FORMAT
	}

	if reqBody.Transactions != "basedOnDates" && reqBody.Transactions != "all" && reqBody.Transactions != "last" {
		slog.Error("invalid transactions type", "received", reqBody.Transactions)
		return utils.INVALID_TRANSACTIONS_TYPE
	}

	unixFromDate := utils.GetDateUnix(reqBody.FromDate)
	unixToDate := utils.GetDateUnix(reqBody.ToDate)

	if unixFromDate > time.Now().Unix() && unixToDate > time.Now().Unix() {
		slog.Error("future date range provided", "from_date", reqBody.FromDate, "to_date", reqBody.ToDate)
		return utils.FUTURE_DATE_ERR
	}

	if unixFromDate > unixToDate {
		slog.Error("from_date is after to_date", "from_date", reqBody.FromDate, "to_date", reqBody.ToDate)
		return utils.INVALID_DATE_RANGE
	}

	err := validateCompoundIdField(reqBody.CompoundId)
	if err != utils.NO_ERR {
		slog.Error("invalid compound_id", "compound_id", reqBody.CompoundId)
		return err
	}

	return utils.NO_ERR
}

func buildGetEntryQueries(filters *GetEntryReq) (string, string, []any) {
	var filterArgs []any
	var whereClause string

	switch filters.Transactions {
	case "basedOnDates":
		fromDate, _ := time.Parse("2006-01-02", filters.FromDate)
		toDate, _ := time.Parse("2006-01-02", filters.ToDate)
		fromUnix := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.Local).Unix()
		toUnix := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 23, 59, 59, 0, time.Local).Unix()

		whereClause = "e.date BETWEEN ? AND ?"
		filterArgs = append(filterArgs, fromUnix, toUnix)

		if filters.Type != "both" {
			whereClause += " AND e.type = ?"
			filterArgs = append(filterArgs, filters.Type)
		}
		if filters.CompoundId != "all" {
			whereClause += " AND e.compound_id = ?"
			filterArgs = append(filterArgs, filters.CompoundId)
		}

	case "all":
		if filters.Type != "both" {
			whereClause = "e.type = ?"
			filterArgs = append(filterArgs, filters.Type)
		}
		if filters.CompoundId != "all" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "e.compound_id = ?"
			filterArgs = append(filterArgs, filters.CompoundId)
		}

	case "last":
		subQuery := `
			SELECT compound_id, MAX(date) AS latest_date
			FROM entry
			GROUP BY compound_id
		`
		mainQuery := `
			SELECT
				e.id, e.type, datetime(e.date, 'unixepoch', 'localtime'),
				e.remark, e.voucher_no, e.net_stock,
				c.id, c.name, c.scale,
				q.num_of_units, q.quantity_per_unit
			FROM entry e
			JOIN (` + subQuery + `) latest
				ON e.compound_id = latest.compound_id AND e.date = latest.latest_date
			JOIN compound c ON e.compound_id = c.id
			JOIN quantity q ON e.quantity_id = q.id
		`
		countQuery := `
			SELECT COUNT(*)
			FROM entry e
			JOIN (` + subQuery + `) latest
				ON e.compound_id = latest.compound_id AND e.date = latest.latest_date
		`

		if filters.Type != "both" {
			whereClause = "e.type = ?"
			filterArgs = append(filterArgs, filters.Type)
		}
		if filters.CompoundId != "all" {
			if whereClause != "" {
				whereClause += " AND "
			}
			whereClause += "e.compound_id = ?"
			filterArgs = append(filterArgs, filters.CompoundId)
		}
		if whereClause != "" {
			mainQuery += " WHERE " + whereClause
			countQuery += " WHERE " + whereClause
		}
		mainQuery += " ORDER BY c.name;"
		return mainQuery, countQuery, filterArgs
	}

	query := `
		SELECT
			e.id, e.type, datetime(e.date, 'unixepoch', 'localtime'),
			e.remark, e.voucher_no, e.net_stock,
			c.id, c.name, c.scale,
			q.num_of_units, q.quantity_per_unit
		FROM entry e
		JOIN compound c ON e.compound_id = c.id
		JOIN quantity q ON e.quantity_id = q.id
	`
	countQuery := `
		SELECT COUNT(*)
		FROM entry e
		JOIN compound c ON e.compound_id = c.id
		JOIN quantity q ON e.quantity_id = q.id
	`
	if whereClause != "" {
		query += " WHERE " + whereClause
		countQuery += " WHERE " + whereClause
	}
	query += " ORDER BY e.date DESC;"
	return query, countQuery, filterArgs
}

func validateCompoundIdField(id string) utils.ErrorMessage {
	if strings.TrimSpace(id) == "all" {
		return utils.NO_ERR
	}

	var exists bool
	err := db.Conn.QueryRow("SELECT EXISTS (SELECT 1 FROM compound WHERE id = ?)", id).Scan(&exists)
	if err != nil || !exists {
		slog.Error("compound ID does not exist or DB error", "compound_id", id, "error", err)
		return utils.INVALID_COMPOUND_ID
	}

	return utils.NO_ERR
}
