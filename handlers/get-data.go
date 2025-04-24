package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// GetData retrieves chemical ledger data based on filters.
func GetData(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodGet, w); err != nil {
		return
	}

	logrus.Info("Received request to get data.")

	filters, err := parseGetDataFilters(r)
	if err != nil {
		logrus.Errorf("Failed to parse get data filters: %v", err)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: err.Error()})
		return
	}
	logrus.Debugf("Parsed filters: %+v", filters)

	if err := validateGetDataFilterType(w, filters); err != nil {
		return
	}

	fromDate, toDate, err := parseAndValidateDateRangeGetData(filters.FromDate, filters.ToDate, w)
	if err != nil {
		return
	}
	logrus.Debugf("Parsed date range: FromDateUnix=%d, ToDateUnix=%d", fromDate, toDate)

	queryStr, countQueryStr, filterArgs := buildGetDataQueries(filters, fromDate, toDate)
	logrus.Debugf("Data Query: %s, Args: %v", queryStr, filterArgs)
	logrus.Debugf("Count Query: %s, Args: %v", countQueryStr, filterArgs)

	totalCount, err := executeGetDataCountQuery(w, countQueryStr, filterArgs)
	if err != nil {
		return
	}
	logrus.Debugf("Total count of entries: %d", totalCount)

	entries, err := executeGetDataQuery(w, queryStr, filterArgs, totalCount)
	if err != nil {
		return
	}
	logrus.Debugf("Retrieved %d entries.", len(entries))

	response := map[string]any{
		"total":   totalCount,
		"results": entries,
	}
	utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: response})
	logrus.Info("Successfully processed get data request.")
}

// parseGetDataFilters extracts filter parameters.
func parseGetDataFilters(r *http.Request) (*utils.Filters, error) {
	filters := &utils.Filters{}
	query := r.URL.Query()
	filters.Type = query.Get("type")
	filters.CompoundName = query.Get("compound")
	filters.FromDate = query.Get("fromDate")
	filters.ToDate = query.Get("toDate")
	logrus.Debugf("Extracted query parameters: %+v", filters)
	return filters, nil
}

// validateGetDataFilterType checks if the 'type' filter is valid.
func validateGetDataFilterType(w http.ResponseWriter, filters *utils.Filters) error {
	if filters.Type != utils.TypeIncoming && filters.Type != utils.TypeOutgoing && filters.Type != utils.TypeBoth && filters.Type != "" {
		logrus.Warnf("Invalid 'type' filter provided: %s", filters.Type)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."})
		return errors.New("invalid 'type' filter")
	}
	return nil
}

// parseAndValidateDateRangeGetData parses and validates date range.
func parseAndValidateDateRangeGetData(fromDate string, toDate string, w http.ResponseWriter) (int64, int64, error) {
	var fromDateUnix int64
	var toDateUnix int64

	if fromDate != "" {
		dateTimeString := fmt.Sprintf("%sT%02d:%02d:%02d+05:30",
			fromDate, 0, 0, 0)

		parsedDate, err := time.Parse(time.RFC3339, dateTimeString)
		if err != nil {
			logrus.Warnf("Invalid 'fromDate' format: %s, error: %v", fromDate, err)
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_format})
			return 0, 0, err
		}
		fromDateUnix = parsedDate.Unix()
	}

	if toDate != "" {
		dateTimeString := fmt.Sprintf("%sT%02d:%02d:%02d+05:30",
			toDate, 23, 59, 59)

		parsedDate, err := time.Parse(time.RFC3339, dateTimeString)
		if err != nil {
			logrus.Warnf("Invalid 'toDate' format: %s, error: %v", toDate, err)
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_format})
			return 0, 0, err
		}
		toDateUnix = parsedDate.Unix()
	}

	if fromDateUnix > 0 && toDateUnix > 0 && fromDateUnix > toDateUnix {
		logrus.Warnf("Invalid date range: 'fromDate' (%s) after 'toDate' (%s)", fromDate, toDate)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_range})
		return 0, 0, errors.New("invalid date range")
	}

	return fromDateUnix, toDateUnix, nil
}

// buildGetDataQueries constructs SQL queries based on filters.
func buildGetDataQueries(filters *utils.Filters, fromDate int64, toDate int64) (string, string, []any) {
	queryBuilder := strings.Builder{}
	countQueryBuilder := strings.Builder{}
	filterArgs := make([]any, 0)

	queryBuilder.WriteString(`
SELECT
	e.id, e.type, datetime(e.date, 'unixepoch', 'localtime') AS formatted_date,
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
	if fromDate > 0 {
		queryBuilder.WriteString(" AND e.date >= ?")
		countQueryBuilder.WriteString(" AND e.date >= ?")
		filterArgs = append(filterArgs, fromDate)
	}
	if toDate > 0 {
		queryBuilder.WriteString(" AND e.date <= ?")
		countQueryBuilder.WriteString(" AND e.date <= ?")
		filterArgs = append(filterArgs, toDate)
	}

	queryBuilder.WriteString(" ORDER BY e.date DESC")

	return queryBuilder.String(), countQueryBuilder.String(), filterArgs
}

// executeGetDataCountQuery executes the count query.
func executeGetDataCountQuery(w http.ResponseWriter, countQueryStr string, filterArgs []any) (int, error) {
	var count int
	err := utils.Retry(func() error {
		return db.Db.QueryRow(countQueryStr, filterArgs...).Scan(&count)
	})
	if err != nil {
		logrus.Errorf("Error executing count query: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return 0, err
	}
	return count, nil
}

// executeGetDataQuery executes the data retrieval query.
func executeGetDataQuery(w http.ResponseWriter, queryStr string, filterArgs []any, totalCount int) ([]utils.GetEntry, error) {
	rows, err := db.Db.Query(queryStr, filterArgs...)
	if err != nil {
		logrus.Errorf("Error executing data query: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return nil, err
	}
	defer rows.Close()

	entries := make([]utils.GetEntry, 0, totalCount)
	for rows.Next() {
		var e utils.GetEntry
		err := rows.Scan(
			&e.ID, &e.Type, &e.Date,
			&e.Remark, &e.VoucherNo, &e.NetStock,
			&e.CompoundId, &e.CompoundName, &e.Scale,
			&e.NumOfUnits, &e.QuantityPerUnit,
		)
		if err != nil {
			logrus.Errorf("Error scanning data row: %v", err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			return nil, err
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		logrus.Errorf("Error during rows iteration: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return nil, err
	}
	return entries, nil
}