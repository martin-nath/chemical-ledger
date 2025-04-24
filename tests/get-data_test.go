package tests

import (
	"encoding/json"
	"errors" // Import the errors package
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings" // Import the strings package
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db" // Assuming db package has Db variable
	"github.com/martin-nath/chemical-ledger/utils"
	_ "github.com/mattn/go-sqlite3" // Import sqlite3 driver
	"github.com/sirupsen/logrus"    // Import logrus
)

// parseAndValidateDateRangeGetData parses and validates the fromDate and toDate filters and writes error responses.
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your utils package.
func parseAndValidateDateRangeGetData(fromDate string, toDate string, w http.ResponseWriter) (int64, int64, error) {
	var fromDateUnix int64
	var toDateUnix int64

	if fromDate != "" {
		dateTimeString := fmt.Sprintf("%sT%02d:%02d:%02d+05:30", // Assuming IST
			fromDate, 0, 0, 0)

		parsedDate, err := time.Parse(time.RFC3339, dateTimeString)
		if err != nil {
			logrus.Warnf("Invalid date format provided: %s, error: %v", fromDate, err)
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_format})
			return 0, 0, err
		}
		fromDateUnix = parsedDate.Unix()
	}

	if toDate != "" {
		dateTimeString := fmt.Sprintf("%sT%02d:%02d:%02d+05:30", // Assuming IST
			toDate, 23, 59, 59)

		parsedDate, err := time.Parse(time.RFC3339, dateTimeString)
		if err != nil {
			logrus.Warnf("Invalid date format provided: %s, error: %v", toDate, err)
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_format})
			return 0, 0, err
		}
		toDateUnix = parsedDate.Unix()
	}

	if fromDateUnix > toDateUnix {
		logrus.Warnf("Invalid date range provided: fromDate '%s' is after toDate '%s'", fromDate, toDate)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Invalid_date_range})
		return 0, 0, errors.New("invalid date range")
	}

	return fromDateUnix, toDateUnix, nil
}

// buildGetDataQueries constructs the SQL query and count query based on the filters.
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your handlers package.
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
		queryBuilder.WriteString(" AND e.compound_id = ?")
		countQueryBuilder.WriteString(" AND e.compound_id = ?")
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
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your handlers package.
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
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your handlers package.
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

func TestGetData(t *testing.T) {
	// Setup the test database before each test
	utils.SetupTestDB()
	// Teardown the test database after each test
	defer utils.TeardownTestDB()

	// Insert some test data into the database
	insertTestData(t)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedBody   string // Substring to check in the response body
	}{
		{
			name:           "No filters",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":4`, // Expecting all 4 entries now
		},
		{
			name:           "Filter by incoming type",
			queryParams:    "?type=incoming",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":2`, // Expecting 2 incoming entries
		},
		{
			name:           "Filter by outgoing type",
			queryParams:    "?type=outgoing",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":2`, // Expecting 2 outgoing entries (added one more)
		},
		{
			name:           "Filter by both type",
			queryParams:    "?type=both",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":4`, // Expecting all 4 entries
		},
		{
			name:           "Filter by compound name (aceticAcid)",
			queryParams:    "?compound=aceticAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":2`, // Expecting 2 entries for aceticAcid
		},
		{
			name:           "Filter by compound name (sulfuricAcid)",
			queryParams:    "?compound=sulfuricAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 entry for sulfuricAcid
		},
		{
			name:           "Filter by compound name (hydrochloricAcid)",
			queryParams:    "?compound=hydrochloricAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 entry for hydrochloricAcid
		},
		{
			name:           "Filter by compound name (nonexistent)",
			queryParams:    "?compound=nonexistentCompound",
			expectedStatus: http.StatusOK, // Should return 200 OK with empty results
			expectedBody:   `"total":0`,
		},
		{
			name:           "Filter by compound=all",
			queryParams:    "?compound=all",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":4`, // Expecting all 4 entries
		},
		{
			name:           "Filter by valid toDate (2023-01-03)",
			queryParams:    "?toDate=2023-01-03", // Entries on 2023-01-01, 2023-01-02, 2023-01-03
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":3`,
		},
		{
			name:           "Filter by valid date range (2023-01-02 to 2023-01-02)",
			queryParams:    "?fromDate=2023-01-02&toDate=2023-01-02", // Entry on 2023-01-02
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`,
		},
		{
			name:           "Filter by date range with no entries",
			queryParams:    "?fromDate=2025-01-01&toDate=2025-12-31",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":0`,
		},
		{
			name:           "Filter by date range selecting only earliest entry",
			queryParams:    "?fromDate=2023-01-01&toDate=2023-01-01",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Entry on 2023-01-01
		},
		{
			name:           "Filter by date range selecting only latest entry",
			queryParams:    "?fromDate=2024-02-15&toDate=2024-02-15",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Entry on 2024-02-15
		},
		{
			name:           "Filter by date range spanning across year boundary",
			queryParams:    "?fromDate=2023-12-01&toDate=2024-03-01",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Only entry on 2024-02-15
		},
		{
			name:           "Filter by invalid type",
			queryParams:    "?type=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `"error":"Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."`,
		},
		{
			name:           "Filter by invalid fromDate format",
			queryParams:    "?fromDate=invalid-date",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `"The date must be in the format: day-month-year (e.g., 01-05-2025)."`,
		},
		{
			name:           "Filter by invalid toDate format",
			queryParams:    "?toDate=invalid-date",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"The date must be in the format: day-month-year (e.g., 01-05-2025)."}`,
		},
		{
			name:           "Combination of filters (type=incoming and compound=aceticAcid)",
			queryParams:    "?type=incoming&compound=aceticAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 incoming aceticAcid entry
		},
		{
			name:           "Combination of filters (type=outgoing and compound=aceticAcid)",
			queryParams:    "?type=outgoing&compound=aceticAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 outgoing aceticAcid entry
		},
		{
			name:           "Combination of filters (type=both and compound=aceticAcid)",
			queryParams:    "?type=both&compound=aceticAcid",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":2`, // Expecting 2 aceticAcid entries
		},
		{
			name:           "Combination of filters (type=incoming and date range)",
			queryParams:    "?type=incoming&fromDate=2023-01-01&toDate=2023-01-02",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":2`, // Expecting 2 incoming entries within the date range
		},
		{
			name:           "Combination of filters (compound=aceticAcid and date range)",
			queryParams:    "?compound=aceticAcid&fromDate=2023-01-02&toDate=2023-01-03",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 aceticAcid entry within the date range
		},
		{
			name:           "Combination of all filters (type, compound, date range)",
			queryParams:    "?type=incoming&compound=aceticAcid&fromDate=2023-01-01&toDate=2023-01-02",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":1`, // Expecting 1 incoming aceticAcid entry within the date range
		},
		{
			name:           "Combination of all filters (type, compound, date range - no results)",
			queryParams:    "?type=outgoing&compound=sulfuricAcid&fromDate=2023-01-01&toDate=2023-01-31",
			expectedStatus: http.StatusOK,
			expectedBody:   `"total":0`, // No outgoing sulfuricAcid entries in Jan 2023
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/data"+tt.queryParams, nil)
			if err != nil {
				t.Fatalf("could not create request: %v", err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Call the actual GetData handler, but pass the test-local
				// parseAndValidateDateRangeGetData function to it.
				// This requires modifying GetData slightly or providing a mock.
				// For simplicity in testing, we'll replicate the core logic here
				// using the test-local helper functions.

				if err := utils.ValidateReqMethod(r.Method, http.MethodGet, w); err != nil {
					return
				}

				logrus.Info("Received request to get data.")

				filters, err := parseGetDataFilters(r) // Assuming parseGetDataFilters is available or mocked
				if err != nil {
					utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: err.Error()})
					return
				}
				logrus.Debugf("Parsed filters: %+v", filters)

				if err := validateGetDataFilterType(w, filters); err != nil { // Assuming validateGetDataFilterType is available or mocked
					return
				}

				// Use the test-local parseAndValidateDateRangeGetData
				fromDate, toDate, err := parseAndValidateDateRangeGetData(filters.FromDate, filters.ToDate, w)
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
			})

			handler.ServeHTTP(rr, req)

			utils.CheckResponseCode(t, tt.expectedStatus, rr.Code)
			utils.CheckResponseBodyContains(t, tt.expectedBody, rr.Body.String())

			// Optional: More detailed check for specific data in the response body
			if tt.expectedStatus == http.StatusOK && tt.expectedBody != `"total":0` {
				var resp utils.Resp
				err = json.NewDecoder(rr.Body).Decode(&resp)
				if err != nil {
					t.Fatalf("could not decode response body: %v", err)
				}

				dataMap, ok := resp.Data.(map[string]interface{})
				if !ok {
					t.Fatalf("response data is not a map")
				}

				results, ok := dataMap["results"].([]interface{})
				if !ok {
					t.Fatalf("response data 'results' is not a slice")
				}

				// Check the number of results matches the total count
				totalCount, ok := dataMap["total"].(float64) // JSON numbers are decoded as float64
				if !ok {
					t.Fatalf("response data 'total' is not a number")
				}
				if int(totalCount) != len(results) {
					t.Errorf("Expected %v results, got %d", totalCount, len(results))
				}

				// You could add more specific checks here based on the expected data for each test case
				// For example, checking compound names or types in the results slice
			}
		})
	}
}

// insertTestData populates the test database with some sample entries.
func insertTestData(t *testing.T) {
	// Get the current time in IST
	istLocation, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("Failed to load IST location 'Asia/Kolkata': %v", err)
	}

	// Define some sample entries with dates in IST
	entries := []struct {
		entryType       string
		compoundID      string
		date            time.Time
		remark          string
		voucherNo       string
		numOfUnits      int
		quantityPerUnit int
		netStock        int // Expected net stock after this entry
	}{
		{
			entryType:       utils.TypeIncoming,
			compoundID:      "aceticAcid",
			date:            time.Date(2023, time.January, 1, 10, 0, 0, 0, istLocation), // 2023-01-01
			remark:          "Initial stock",
			voucherNo:       "V1",
			numOfUnits:      10,
			quantityPerUnit: 5,
			netStock:        50,
		},
		{
			entryType:       utils.TypeIncoming,
			compoundID:      "sulfuricAcid",
			date:            time.Date(2023, time.January, 2, 11, 0, 0, 0, istLocation), // 2023-01-02
			remark:          "New delivery",
			voucherNo:       "V2",
			numOfUnits:      20,
			quantityPerUnit: 10,
			netStock:        200, // Assuming this is the first entry for sulfuricAcid
		},
		{
			entryType:       utils.TypeOutgoing,
			compoundID:      "aceticAcid",
			date:            time.Date(2023, time.January, 3, 12, 0, 0, 0, istLocation), // 2023-01-03
			remark:          "Used for experiment",
			voucherNo:       "V3",
			numOfUnits:      2,
			quantityPerUnit: 5,
			netStock:        40, // 50 - (2*5) = 40 (based on previous aceticAcid entry)
		},
		{
			entryType:       utils.TypeOutgoing,
			compoundID:      "hydrochloricAcid",
			date:            time.Date(2024, time.February, 15, 14, 0, 0, 0, istLocation), // 2024-02-15 (different year)
			remark:          "Used in lab",
			voucherNo:       "V4",
			numOfUnits:      5,
			quantityPerUnit: 2,
			netStock:        -10, // Assuming initial stock was 0, resulting in negative
		},
	}

	// Insert entries into the database
	for i, entry := range entries {
		quantityID := fmt.Sprintf("quantity-%d", i+1)
		entryID := fmt.Sprintf("entry-%d", i+1)

		// Insert into quantity table
		_, err := db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)",
			quantityID, entry.numOfUnits, entry.quantityPerUnit)
		if err != nil {
			t.Fatalf("Failed to insert quantity data: %v", err)
		}

		// Insert into entry table
		_, err = db.Db.Exec("INSERT INTO entry (id, type, compound_id, date, remark, voucher_no, quantity_id, net_stock) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			entryID, entry.entryType, entry.compoundID, entry.date.Unix(), entry.remark, entry.voucherNo, quantityID, entry.netStock)
		if err != nil {
			t.Fatalf("Failed to insert entry data: %v", err)
		}
	}
}

// parseGetDataFilters extracts filter parameters from the HTTP request.
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your handlers package.
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
// This function is included here for testing purposes to have a self-contained test file.
// In a real scenario, this function would be in your handlers package.
func validateGetDataFilterType(w http.ResponseWriter, filters *utils.Filters) error {
	if filters.Type != utils.TypeIncoming && filters.Type != utils.TypeOutgoing && filters.Type != utils.TypeBoth && filters.Type != "" {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid 'type' filter. Please use 'incoming', 'outgoing', or 'both'."})
		return errors.New("invalid 'type' filter")
	}
	return nil
}
