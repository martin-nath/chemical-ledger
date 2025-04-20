package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// GetData handles the retrieval of chemical ledger data based on provided filters,
// reusing functions from the utils package.
func GetData(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodGet, w); err != nil {
		return
	}

	logrus.Info("Received request to get data.")

	filters, err := parseGetDataFilters(r)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: err.Error()})
		return
	}
	logrus.Debugf("Parsed filters: %+v", filters)

	if err := validateGetDataFilterType(w, filters); err != nil {
		return
	}

	fromDate, toDate, err := parseAndValidateDateRangeGetData(w, filters)
	if err != nil {
		return
	}

	queryStr, countQueryStr, filterArgs := buildGetDataQueries(filters, fromDate, toDate)
	logrus.Debugf("Data Query: %s, Args: %v", queryStr, filterArgs)
	logrus.Debugf("Count Query: %s, Args: %v", countQueryStr, filterArgs)

	totalCount, err := executeGetDataCountQuery(w, countQueryStr, filterArgs)
	if err != nil {
		return
	}

	entries, err := executeGetDataQuery(w, queryStr, filterArgs, totalCount)
	if err != nil {
		return
	}

	response := map[string]any{
		"total":   totalCount,
		"results": entries,
	}
	utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: response})
}

// parseGetDataFilters extracts filter parameters from the HTTP request.
func parseGetDataFilters(r *http.Request) (*utils.Filters, error) {
	filters := &utils.Filters{}
	query := r.URL.Query()
	filters.Type = query.Get("type")
	filters.CompoundName = query.Get("compound")
	filters.FromDate = query.Get("fromDate")
	filters.ToDate = query.Get("toDate")
	return filters, nil
}

// validateGetDataFilterType checks if the 'type' filter is valid and writes an error response if not.
func validateGetDataFilterType(w http.ResponseWriter, filters *utils.Filters) error {
	if filters.Type != utils.TypeIncoming && filters.Type != utils.TypeOutgoing && filters.Type != utils.TypeBoth && filters.Type != "" {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."})
		return errors.New("invalid 'type' filter")
	}
	return nil
}

// parseAndValidateDateRangeGetData parses and validates the fromDate and toDate filters and writes error responses.
func parseAndValidateDateRangeGetData(w http.ResponseWriter, filters *utils.Filters) (int64, int64, error) {
	var fromDate int64
	var toDate int64
	var err error

	if filters.FromDate != "" {
		fromDate, err = utils.ParseAndValidateDate(filters.FromDate, w)
		if err != nil {
			return 0, 0, err
		}
	}

	if filters.ToDate != "" {
		toDate, err = utils.ParseAndValidateDate(filters.ToDate, w)
		if err != nil {
			return 0, 0, err
		}
		if filters.FromDate != "" && toDate < fromDate {
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid date range: 'toDate' cannot be earlier than 'fromDate'."})
			logrus.Warn("Invalid date range provided.")
			return 0, 0, errors.New("invalid date range")
		}
	}
	return fromDate, toDate, nil
}

// buildGetDataQueries constructs the SQL query and count query based on the filters.
func buildGetDataQueries(filters *utils.Filters, fromDate int64, toDate int64) (string, string, []any) {
	queryBuilder := strings.Builder{}
	countQueryBuilder := strings.Builder{}
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

// executeGetDataCountQuery executes the SQL query to get the total count of matching entries and writes an error response if it fails.
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

// executeGetDataQuery executes the SQL query to retrieve the data entries and writes an error response if it fails.
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
			&e.CompoundName, &e.Scale,
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
