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
	Type       string `json:"type"`
	CompoundId string `json:"compound_id"`
	FromDate   string `json:"from_date"`
	ToDate     string `json:"to_date"`
}

func GetEntryHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &GetEntryReq{
		Type:       utils.GetParam(r, "type"),
		CompoundId: utils.GetParam(r, "compound_id"),
		FromDate:   utils.GetParam(r, "from_date"),
		ToDate:     utils.GetParam(r, "to_date"),
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
		slog.Error("failed to query last transactions", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}

	wg.Wait()
	close(countCh)
	close(errCh)
	if err := <-errCh; err != nil {
		slog.Error("failed to scan last transactions", "error", err)
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
			slog.Error("failed to scan entry", "error", err)
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
		slog.Error("missing required fields")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING && reqBody.Type != "both" {
		slog.Error("invalid entry type, received: " + reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	if _, err := time.Parse("2006-01-02", reqBody.FromDate); err != nil {
		slog.Error(err.Error())
		return utils.INVALID_DATE_FORMAT
	}
	if _, err := time.Parse("2006-01-02", reqBody.ToDate); err != nil {
		slog.Error(err.Error())
		return utils.INVALID_DATE_FORMAT
	}

	unixFromDate := utils.GetDateUnix(reqBody.FromDate)
	unixToDate := utils.GetDateUnix(reqBody.ToDate)

	if unixFromDate > time.Now().Unix() && unixToDate > time.Now().Unix() {
		slog.Error("future date provided, " + reqBody.FromDate + " and " + reqBody.ToDate)
		return utils.FUTURE_DATE_ERR
	}

	if unixFromDate > unixToDate {
		slog.Error("from date is after to date")
		return utils.INVALID_DATE_RANGE
	}

	switch reqBody.CompoundId {
	case "all", "lastTransactions":
		return utils.NO_ERR
	}

	compoundExists, err := utils.CheckIfCompoundExists(reqBody.CompoundId)
	if err != nil {
		slog.Error("compound check error: " + err.Error())
		return utils.COMPOUND_ID_CHECK_ERR
	}

	if !compoundExists {
		slog.Error("compound not found")
		return utils.INVALID_COMPOUND_ID
	}

	return utils.NO_ERR
}

func buildGetEntryQueries(filters *GetEntryReq) (string, string, []any) {
	if filters.CompoundId == "lastTransactions" {
		return `
					SELECT
						e.id,
						e.type,
						datetime(e.date, 'unixepoch', 'localtime'),
						e.remark,
						e.voucher_no,
						e.net_stock,
						c.id,
						c.name,
						c.scale,
						q.num_of_units,
						q.quantity_per_unit
					FROM entry e
					JOIN (
							SELECT compound_id, MAX(date) AS latest_date
							FROM entry
							GROUP BY compound_id
					) latest ON e.compound_id = latest.compound_id AND e.date = latest.latest_date
					JOIN compound c ON e.compound_id = c.id
					JOIN quantity q ON e.quantity_id = q.id
					ORDER BY c.name;
			`,

			// Count query
			`
				SELECT COUNT(*)
				FROM entry e
				JOIN (
						SELECT compound_id, MAX(date) AS latest_date
						FROM entry
						GROUP BY compound_id
				) latest ON e.compound_id = latest.compound_id AND e.date = latest.latest_date;
			`, []any{}
	}

	now := time.Now().Local()

	fromDateObj, _ := time.Parse("2006-01-02", filters.FromDate)
	fromDate := time.Date(fromDateObj.Year(), fromDateObj.Month(), fromDateObj.Day(), 0, 0, 0, 0, now.Location()).Unix()

	toDateObj, _ := time.Parse("2006-01-02", filters.ToDate)
	toDate := time.Date(toDateObj.Year(), toDateObj.Month(), toDateObj.Day(), 23, 59, 59, 0, now.Location()).Unix()

	queryBuilder := strings.Builder{}
	countQueryBuilder := strings.Builder{}
	filterArgs := make([]any, 0)

	queryBuilder.WriteString(`
SELECT
	e.id, e.type, datetime(e.date, 'unixepoch', 'localtime'),
	e.remark, e.voucher_no, e.net_stock,
	c.id, c.name, c.scale,
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

	filterQueryBuilder := strings.Builder{}

	if filters.Type == utils.ENTRY_TYPE_INCOMING || filters.Type == utils.ENTRY_TYPE_OUTGOING {
		filterQueryBuilder.WriteString(" AND e.type = ?")
		filterArgs = append(filterArgs, filters.Type)
	}
	if filters.CompoundId != "all" {
		filterQueryBuilder.WriteString(" AND e.compound_id = ?")
		filterArgs = append(filterArgs, filters.CompoundId)
	}
	if fromDate > 0 {
		filterQueryBuilder.WriteString(" AND e.date >= ?")
		filterArgs = append(filterArgs, fromDate)
	}
	if toDate > 0 {
		filterQueryBuilder.WriteString(" AND e.date <= ?")
		filterArgs = append(filterArgs, toDate)
	}

	filterQueryStr := filterQueryBuilder.String()

	countQueryBuilder.WriteString(filterQueryStr)
	queryBuilder.WriteString(filterQueryStr)

	queryBuilder.WriteString(" ORDER BY e.date DESC")

	return queryBuilder.String(), countQueryBuilder.String(), filterArgs
}
