package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/migrate"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// const testDBPath = ":memory:"

func setupTestDB() error {
	db.InitDB("test.db")
	err := migrate.DropTables(db.Db)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return fmt.Errorf("failed to drop tables: %w", err)
	}
	if err := migrate.CreateTables(db.Db); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}
	return nil
}

func teardownTestDB() {
	defer func() {
		db.Db.Close()
		os.Remove("test.db")
	}()
	err := migrate.DropTables(db.Db)
	if err != nil {
		panic("Failed to drop tables: " + err.Error())
	}
}

func executeRequest(req *http.Request, handler http.Handler) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func checkResponseCode(t *testing.T, expected, actual int) {
	assert.Equal(t, expected, actual, "Handler returned wrong status code")
}

func checkResponseBodyContains(t *testing.T, expectedSubstring string, actualBody string) {
	assert.Contains(t, actualBody, expectedSubstring, "Response body should contain substring")
}

func insertGetDataTestData() error {
	_, err := db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty1', 5, 10)")
	if err != nil {
		return errors.New("failed to insert test quantity 'qty1'")
	}
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty2', 3, 20)")
	if err != nil {
		return errors.New("failed to insert test quantity 'qty2'")
	}
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty3', 2, 15)")
	if err != nil {
		return errors.New("failed to insert test quantity 'qty3'")
	}
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty_update_1', 10, 10)")
	if err != nil {
		return errors.New("failed to insert test quantity 'qty_update_1'")
	}

	_, err = db.Db.Exec("INSERT INTO compound (id, name, scale) VALUES ('aceticAcid', 'Acetic acid', 'ml')")
	if err != nil {
		return errors.New("failed to insert test compound 'aceticAcid'")
	}
	_, err = db.Db.Exec("INSERT INTO compound (id, name, scale) VALUES ('ethanol', 'Ethanol', 'ml')")
	if err != nil {
		return errors.New("failed to insert test compound 'ethanol'")
	}
	_, err = db.Db.Exec("INSERT INTO compound (id, name, scale) VALUES ('benzene', 'Benzene', 'ml')")
	if err != nil {
		return errors.New("failed to insert test compound 'benzene'")
	}
	_, err = db.Db.Exec("INSERT INTO compound (id, name, scale) VALUES ('sodiumChloride', 'Sodium Chloride', 'g')")
	if err != nil {
		return errors.New("failed to insert test compound 'sodiumChloride'")
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	todayTimestamp := today.Unix()
	yesterdayTimestamp := today.AddDate(0, 0, -1).Unix()
	twoDaysAgoTimestamp := today.AddDate(0, 0, -2).Unix()

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry1', 'incoming', ?, 'Initial stock', 'V001', 50, 'aceticAcid', 'qty1')
	`, twoDaysAgoTimestamp)
	if err != nil {
		return errors.New("failed to insert test entry 'entry1'")
	}

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry2', 'outgoing', ?, 'Experiment use', 'V002', 30, 'aceticAcid', 'qty2')
	`, yesterdayTimestamp)
	if err != nil {
		return errors.New("failed to insert test entry 'entry2'")
	}

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry3', 'incoming', ?, 'New stock', 'V003', 60, 'ethanol', 'qty3')
	`, todayTimestamp)
	if err != nil {
		return errors.New("failed to insert test entry 'entry3'")
	}

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry_update_1', 'incoming', ?, 'Entry to be updated', 'V_UPDATE_1', 100, 'benzene', 'qty_update_1')
	`, twoDaysAgoTimestamp)
	if err != nil {
		return errors.New("failed to insert test entry 'entry_update_1'")
	}

	return nil
}

func formatDateForURL(date time.Time) string {
	return date.Format("02-01-2006")
}

// createRequest creates a new http.Request with the given method, url, and optional body.
func createRequest(method, url string, body map[string]any) *http.Request {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			// In a real test, you might want to handle this error more gracefully
			// or use a testing helper to fail the test.
			panic(fmt.Sprintf("Failed to marshal request body: %v", err))
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		// In a real test, you might want to handle this error more gracefully
		// or use a testing helper to fail the test.
		panic(fmt.Sprintf("Failed to create request: %v", err))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req
}

func TestGetData(t *testing.T) {
	err := setupTestDB()
	require.NoError(t, err, "Database setup failed")
	defer func() {
		teardownTestDB()
		// require.NoError(t, err, "Database teardown failed")
	}()

	err = insertGetDataTestData()
	require.NoError(t, err, "Test data insertion failed")

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		validateResp   func(t *testing.T, body string, resp map[string]interface{})
		expectedMsg    string             // Added expectedMsg field
		expectedError  string             // Added expectedError field
		validateDb     func(t *testing.T) // Added validateDb field
	}{
		{
			name:           "Get all entries",
			url:            "/api/data",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(4), total, "Should return all 4 entries") // Updated count

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 4, "Should return 4 entries") // Updated count
			},
		},
		{
			name:           "Filter by type - incoming",
			url:            "/api/data?type=incoming",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(3), total, "Should return 3 incoming entries") // Updated count

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 3, "Should return 3 entries") // Updated count

				for _, result := range results {
					entry, ok := result.(map[string]interface{})
					require.True(t, ok, "Each result should be an entry map")
					assert.Equal(t, "incoming", entry["type"], "Entry should be of type 'incoming'")
				}
			},
		},
		{
			name:           "Filter by type - outgoing",
			url:            "/api/data?type=outgoing",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 outgoing entry")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]interface{})
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "outgoing", entry["type"], "Entry should be of type 'outgoing'")
			},
		},
		{
			name:           "Filter by compound name",
			url:            "/api/data?compound=Acetic%20acid",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(2), total, "Should return 2 entries for 'Acetic acid'")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 2, "Should return 2 entries")

				for _, result := range results {
					entry, ok := result.(map[string]interface{})
					require.True(t, ok, "Each result should be an entry map")
					assert.Equal(t, "Acetic acid", entry["compound_name"], "Entry should be for 'Acetic acid'")
				}
			},
		},
		{
			name:           "Filter by date range (yesterday to today)",
			url:            "/api/data?fromDate=" + formatDateForURL(time.Now().AddDate(0, 0, -1)) + "&toDate=" + formatDateForURL(time.Now()),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(3), total, "Should return 3 entries within the date range (yesterday and today's entry)") // Updated count

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 3, "Should return 3 entries") // Updated count
			},
		},
		{
			name:           "Filter by single date (today)",
			url:            "/api/data?fromDate=" + formatDateForURL(time.Now()) + "&toDate=" + formatDateForURL(time.Now()),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry for today")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")
			},
		},
		{
			name:           "Combined filters (type and compound)",
			url:            "/api/data?type=incoming&compound=Ethanol",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry matching both filters")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]interface{})
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "incoming", entry["type"], "Entry should be of type 'incoming'")
				assert.Equal(t, "Ethanol", entry["compound_name"], "Entry should be for 'Ethanol'")
			},
		},
		{
			name:           "Combined filters (type, compound, and date)",
			url:            "/api/data?type=outgoing&compound=Acetic%20acid&fromDate=" + formatDateForURL(time.Now().AddDate(0, 0, -1)) + "&toDate=" + formatDateForURL(time.Now().AddDate(0, 0, -1)),
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry matching all filters")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]interface{})
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "outgoing", entry["type"], "Entry should be of type 'outgoing'")
				assert.Equal(t, "Acetic acid", entry["compound_name"], "Entry should be for 'Acetic acid'")
			},
		},
		{
			name:           "Filter returning no results",
			url:            "/api/data?type=outgoing&compound=Ethanol",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				data, ok := resp["data"].(map[string]interface{})
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(0), total, "Should return 0 entries")

				results, ok := data["results"].([]interface{})
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 0, "Should return 0 entries")
			},
		},
		{
			name:           "Invalid type filter",
			url:            "/api/data?type=invalid",
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Contains(t, errorMsg, "Invalid 'type' filter", "Error message should indicate invalid type")
				checkResponseBodyContains(t, "Invalid 'type' filter", body)
			},
		},
		{
			name:           "Invalid fromDate format",
			url:            "/api/data?fromDate=invaliddateformat",
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Invalid_date_format, errorMsg, "Error message should match Invalid_date_format util constant")
				checkResponseBodyContains(t, utils.Invalid_date_format, body)
			},
		},
		{
			name:           "Invalid toDate format",
			url:            "/api/data?toDate=invaliddateformat",
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Invalid_date_format, errorMsg, "Error message should match Invalid_date_format util constant")
				checkResponseBodyContains(t, utils.Invalid_date_format, body)
			},
		},
		{
			name:           "Future fromDate",
			url:            "/api/data?fromDate=" + formatDateForURL(time.Now().AddDate(0, 0, 1)),
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Future_date_error, errorMsg, "Error message should match Future_date_error util constant")
				checkResponseBodyContains(t, utils.Future_date_error, body)
			},
		},
		{
			name:           "Future toDate (fromDate is valid)",
			url:            "/api/data?fromDate=" + formatDateForURL(time.Now().AddDate(0, 0, -2)) + "&toDate=" + formatDateForURL(time.Now().AddDate(0, 0, 1)),
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Future_date_error, errorMsg, "Error message should match Future_date_error util constant")
				checkResponseBodyContains(t, utils.Future_date_error, body)
			},
		},
		{
			name:           "Invalid date range (toDate before fromDate)",
			url:            "/api/data?fromDate=" + formatDateForURL(time.Now()) + "&toDate=" + formatDateForURL(time.Now().AddDate(0, 0, -1)),
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]interface{}) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Contains(t, errorMsg, "Invalid date range", "Error message should indicate invalid date range")
				checkResponseBodyContains(t, "Invalid date range", body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use createRequest for consistency, although for GET requests without body it's simpler
			req := createRequest(http.MethodGet, tc.url, nil)
			require.NoError(t, req.ParseForm(), "Failed to parse request form") // Parse form to simulate a real request

			rr := executeRequest(req, http.HandlerFunc(handlers.GetData))

			checkResponseCode(t, tc.expectedStatus, rr.Code)

			var response map[string]interface{}
			responseBodyBytes := rr.Body.Bytes()

			if len(responseBodyBytes) > 0 {
				err = json.Unmarshal(responseBodyBytes, &response)
				if tc.expectedStatus >= 400 {
					require.NoError(t, err, fmt.Sprintf("Failed to parse error response JSON for status %d: %s", rr.Code, rr.Body.String()))
					// In TestGetData, we check for error message in validateResp, not here directly.
					// errorMsg, ok := response["error"].(string)
					// require.True(t, ok, "Response should contain an 'error' string")
					// assert.Contains(t, errorMsg, tc.expectedError, "Error message should contain expected error substring")
				} else {
					require.NoError(t, err, fmt.Sprintf("Failed to parse success response JSON for status %d: %s", rr.Code, rr.Body.String()))
					// In TestGetData, we check for message and data in validateResp, not here directly.
					// message, ok := response["message"].(string)
					// require.True(t, ok, "Response should contain a 'message' string")
					// assert.Contains(t, message, tc.expectedMsg, "Success message should contain expected message substring")
					//
					// if tc.validateDb != nil {
					// 	tc.validateDb(t)
					// }
				}
			} else {
				if len(responseBodyBytes) > 0 {
					t.Errorf("Expected empty body for status %d, but received data", tc.expectedStatus)
				}
				// If no body is expected (e.g., for certain error statuses with no JSON response)
				// In TestGetData, we check body content in validateResp, not here directly.
				// if tc.expectedMsg != "" || tc.expectedError != "" {
				// 	// Check body content if expected message or error implies a body
				// 	checkResponseBodyContains(t, tc.expectedMsg+tc.expectedError, resp.Body.String())
				// }
			}
			// Call validateResp for all cases to handle specific body/response structure assertions
			tc.validateResp(t, rr.Body.String(), response)
		})
	}
}

func TestInsertData(t *testing.T) {
	setupTestDB()
	defer teardownTestDB()

	t.Run("Valid Data Insertion", func(t *testing.T) {
		pastDate := "01-01-2006"
		validPayload := map[string]any{
			"type":              utils.TypeIncoming,
			"date":              pastDate,
			"remark":            "Test Remark",
			"voucher_no":        "12345",
			"compound_id":       "sodiumChloride",
			"num_of_units":      10,
			"quantity_per_unit": 5,
		}

		req := createRequest(http.MethodPost, "/insert", validPayload)
		resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))

		checkResponseCode(t, http.StatusCreated, resp.Code)
		checkResponseBodyContains(t, utils.Entry_inserted_successfully, resp.Body.String())
	})

	t.Run("Invalid Input Handling", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Missing QuantityPerUnit",
				requestBody: map[string]any{
					"type":         utils.TypeIncoming,
					"date":         pastDate,
					"remark":       "Test Remark",
					"voucher_no":   "12345",
					"compound_id":  "benzene",
					"num_of_units": 10,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Invalid Date Format",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              "15042025",
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.Invalid_date_format,
			},
			{
				name: "Future Date",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              time.Now().AddDate(0, 1, 0).Format("02-01-2006"),
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.Future_date_error,
			},
			{
				name: "Invalid Type",
				requestBody: map[string]any{
					"type":              "transfer",
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Zero QuantityPerUnit",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 0,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Zero NumOfUnits",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      0,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name:           "Empty Payload",
				requestBody:    map[string]any{},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Invalid Request Method - GET",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusMethodNotAllowed,
				expectedError:  utils.InvalidMethod,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				method := http.MethodPost
				if tc.name == "Invalid Request Method - GET" {
					method = http.MethodGet
				}
				req := createRequest(method, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))

				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Edge Cases", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
		}{
			{
				name: "Empty Remark and Voucher Number",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "",
					"voucher_no":        "",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Compound Existence Validation", func(t *testing.T) {
		pastDate := "01-01-2006"
		invalidCompoundPayload := map[string]any{
			"type":              utils.TypeIncoming,
			"date":              pastDate,
			"remark":            "Test Remark",
			"voucher_no":        "12345",
			"compound_id":       "nonExistentCompound",
			"num_of_units":      10,
			"quantity_per_unit": 5,
		}

		req := createRequest(http.MethodPost, "/insert", invalidCompoundPayload)
		resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))

		checkResponseCode(t, http.StatusNotFound, resp.Code)
		checkResponseBodyContains(t, utils.Item_not_found, resp.Body.String())
	})

	t.Run("More Invalid Input", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Negative NumOfUnits",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      -1,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Negative QuantityPerUnit",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": -5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("String Length Validation", func(t *testing.T) {
		pastDate := "01-01-2006"
		longString := strings.Repeat("A", 256) // Exceeding a hypothetical max length

		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Long Remark",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            longString,
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated, // Or appropriate validation error
				expectedError:  "",
			},
			{
				name: "Long VoucherNo",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        longString,
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
				expectedError:  "",
			},
			// Add more cases for other string fields if needed
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Boundary Values", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
		}{
			{
				name: "Max Int for NumOfUnits",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Max Units",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      2147483647,
					"quantity_per_unit": 1,
				},
				expectedStatus: http.StatusCreated, // Assuming success if within DB limits
			},
			{
				name: "Max Int for QuantityPerUnit",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Max QPU",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      1,
					"quantity_per_unit": 2147483647,
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Case Sensitivity", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Uppercase Type",
				requestBody: map[string]any{
					"type":              strings.ToUpper(utils.TypeIncoming),
					"date":              pastDate,
					"remark":            "Case Test",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "MixedCase Type",
				requestBody: map[string]any{
					"type":              "InComIng",
					"date":              pastDate,
					"remark":            "Case Test",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Date Boundary Values", func(t *testing.T) {
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Epoch Date",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              "01-01-1970",
					"remark":            "Epoch",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated, // Or appropriate success
				expectedError:  "",
			},
			{
				name: "Near Future Date",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              time.Now().AddDate(0, 0, 1).Format("02-01-2006"),
					"remark":            "Near Future",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.Future_date_error,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Unicode Characters", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
		}{
			{
				name: "Unicode Remark",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Unicode Test: こんにちは、世界！",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Unicode Voucher",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Unicode Test",
					"voucher_no":        "你好世界",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Trailing Whitespace", func(t *testing.T) {
		pastDate := "01-01-2006"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
			expectedError  string
		}{
			{
				name: "Trailing Whitespace in Remark",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark  ",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated, // Or handle whitespace trimming
				expectedError:  "",
			},
			{
				name: "Trailing Whitespace in Voucher",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "123  ",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated, // Or handle whitespace trimming
				expectedError:  "",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, http.HandlerFunc(handlers.InsertData))
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})
}

func TestUpdateData(t *testing.T) {
	err := setupTestDB()
	require.NoError(t, err, "Database setup failed")
	defer teardownTestDB()

	// Insert data to be updated
	err = insertGetDataTestData() // Reusing this for initial data
	require.NoError(t, err, "Initial data insertion for update tests failed")

	tests := []struct {
		name           string
		updatePayload  map[string]any
		expectedStatus int
		expectedMsg    string
		expectedError  string
		validateDb     func(t *testing.T) // Optional: for more detailed database validation after update
	}{
		{
			name: "Successful Type Update",
			updatePayload: map[string]any{
				"id":   "entry_update_1",
				"type": utils.TypeOutgoing,
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var entryType string
				err := db.Db.QueryRow("SELECT type FROM entry WHERE id = 'entry_update_1'").Scan(&entryType)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				assert.Equal(t, utils.TypeOutgoing, entryType, "Entry type was not updated correctly")
			},
		},
		{
			name: "Successful Remark and Voucher Update",
			updatePayload: map[string]any{
				"id":         "entry_update_1",
				"remark":     "Updated Remark",
				"voucher_no": "V_UPDATED",
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var remark, voucherNo string
				err := db.Db.QueryRow("SELECT remark, voucher_no FROM entry WHERE id = 'entry_update_1'").Scan(&remark, &voucherNo)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				assert.Equal(t, "Updated Remark", remark, "Entry remark was not updated correctly")
				assert.Equal(t, "V_UPDATED", voucherNo, "Entry voucher number was not updated correctly")
			},
		},
		{
			name: "Successful Quantity Update",
			updatePayload: map[string]any{
				"id":                "entry_update_1",
				"num_of_units":      5,
				"quantity_per_unit": 20,
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var numUnits, qtyPerUnit int
				err := db.Db.QueryRow("SELECT q.num_of_units, q.quantity_per_unit FROM quantity q JOIN entry e ON q.id = e.quantity_id WHERE e.id = 'entry_update_1'").Scan(&numUnits, &qtyPerUnit)
				require.NoError(t, err, "Failed to retrieve updated quantity from DB")
				assert.Equal(t, 5, numUnits, "Quantity num_of_units was not updated correctly")
				assert.Equal(t, 20, qtyPerUnit, "Quantity quantity_per_unit was not updated correctly")
			},
		},
		{
			name: "Successful Compound Update",
			updatePayload: map[string]any{
				"id":          "entry_update_1",
				"compound_id": "sodiumChloride",
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var compoundId string
				err := db.Db.QueryRow("SELECT compound_id FROM entry WHERE id = 'entry_update_1'").Scan(&compoundId)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				assert.Equal(t, "sodiumChloride", compoundId, "Entry compound ID was not updated correctly")
			},
		},
		{
			name: "Successful Date Update",
			updatePayload: map[string]any{
				"id":   "entry_update_1",
				"date": formatDateForURL(time.Now().AddDate(0, 0, -10)), // Update to a different past date
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var entryDate int64
				err := db.Db.QueryRow("SELECT date FROM entry WHERE id = 'entry_update_1'").Scan(&entryDate)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				expectedDate, _ := utils.ParseAndValidateDate(formatDateForURL(time.Now().AddDate(0, 0, -10)), nil)
				assert.Equal(t, expectedDate, entryDate, "Entry date was not updated correctly")
			},
		},
		{
			name: "Partial Update - Only Type",
			updatePayload: map[string]any{
				"id":   "entry_update_1",
				"type": utils.TypeOutgoing,
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var entryType, remark, voucherNo string
				var numUnits, qtyPerUnit int
				err := db.Db.QueryRow("SELECT e.type, e.remark, e.voucher_no, q.num_of_units, q.quantity_per_unit FROM entry e JOIN quantity q ON e.quantity_id = q.id WHERE e.id = 'entry_update_1'").Scan(&entryType, &remark, &voucherNo, &numUnits, &qtyPerUnit)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				assert.Equal(t, utils.TypeOutgoing, entryType, "Entry type was not updated correctly")
				// Check that other fields remained unchanged from insertGetDataTestData
				assert.Equal(t, "Entry to be updated", remark, "Entry remark should not have been updated")
				assert.Equal(t, "V_UPDATE_1", voucherNo, "Entry voucher number should not have been updated")
				assert.Equal(t, 10, numUnits, "Quantity num_of_units should not have been updated")
				assert.Equal(t, 10, qtyPerUnit, "Quantity quantity_per_unit should not have been updated")
			},
		},
		{
			name: "Update Non-Existent Entry",
			updatePayload: map[string]any{
				"id":   "non_existent_entry",
				"type": utils.TypeOutgoing,
			},
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "",
			expectedError:  utils.Item_not_found,
		},
		{
			name: "Invalid Input - Missing ID",
			updatePayload: map[string]any{
				"type": utils.TypeOutgoing,
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "",
			expectedError:  utils.MissingFields_or_inappropriate_value, // Assuming ID is a required field for update
		},
		{
			name: "Invalid Input - Invalid Type",
			updatePayload: map[string]any{
				"id":   "entry_update_1",
				"type": "invalid_type",
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "",
			expectedError:  utils.MissingFields_or_inappropriate_value, // Or a more specific invalid type error if implemented
		},
		{
			name: "Invalid Input - Future Date",
			updatePayload: map[string]any{
				"id":   "entry_update_1",
				"date": formatDateForURL(time.Now().AddDate(0, 0, 1)),
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "",
			expectedError:  utils.Future_date_error,
		},
		{
			name: "Invalid Input - Negative NumOfUnits",
			updatePayload: map[string]any{
				"id":           "entry_update_1",
				"num_of_units": -5,
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "",
			expectedError:  utils.MissingFields_or_inappropriate_value,
		},
		{
			name: "Invalid Input - Negative QuantityPerUnit",
			updatePayload: map[string]any{
				"id":                "entry_update_1",
				"quantity_per_unit": -5,
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "",
			expectedError:  utils.MissingFields_or_inappropriate_value,
		},
		{
			name: "Invalid Input - Non-existent Compound ID",
			updatePayload: map[string]any{
				"id":          "entry_update_1",
				"compound_id": "non_existent_compound",
			},
			expectedStatus: http.StatusNotFound, // Or a more specific compound not found error if implemented
			expectedMsg:    "",
			expectedError:  utils.Item_not_found,
		},
		{
			name: "Insufficient Stock for Outgoing Update",
			updatePayload: map[string]any{
				"id":                "entry_update_1",
				"type":              utils.TypeOutgoing,
				"num_of_units":      20, // Original stock was 10 * 10 = 100, trying to make it 20 * 10 = 200 outgoing
				"quantity_per_unit": 10,
			},
			expectedStatus: http.StatusNotAcceptable,
			expectedMsg:    "",
			expectedError:  utils.Insufficient_stock,
		},
		{
			name: "Update with empty Remark and Voucher Number",
			updatePayload: map[string]any{
				"id":         "entry_update_1",
				"remark":     "",
				"voucher_no": "",
			},
			expectedStatus: http.StatusOK,
			expectedMsg:    utils.Entry_updated_successfully,
			expectedError:  "",
			validateDb: func(t *testing.T) {
				var remark, voucherNo string
				err := db.Db.QueryRow("SELECT remark, voucher_no FROM entry WHERE id = 'entry_update_1'").Scan(&remark, &voucherNo)
				require.NoError(t, err, "Failed to retrieve updated entry from DB")
				assert.Equal(t, "", remark, "Entry remark was not updated to empty correctly")
				assert.Equal(t, "", voucherNo, "Entry voucher number was not updated to empty correctly")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := createRequest(http.MethodPut, "/update", tc.updatePayload)
			resp := executeRequest(req, http.HandlerFunc(handlers.UpdateData))

			checkResponseCode(t, tc.expectedStatus, resp.Code)

			var response map[string]interface{}
			responseBodyBytes := resp.Body.Bytes()

			if len(responseBodyBytes) > 0 {
				err = json.Unmarshal(responseBodyBytes, &response)
				if tc.expectedStatus >= 400 {
					require.NoError(t, err, fmt.Sprintf("Failed to parse error response JSON for status %d: %s", resp.Code, resp.Body.String()))
					errorMsg, ok := response["error"].(string)
					require.True(t, ok, "Response should contain an 'error' string")
					assert.Contains(t, errorMsg, tc.expectedError, "Error message should contain expected error substring")
				} else {
					require.NoError(t, err, fmt.Sprintf("Failed to parse success response JSON for status %d: %s", resp.Code, resp.Body.String()))
					message, ok := response["message"].(string)
					require.True(t, ok, "Response should contain a 'message' string")
					assert.Contains(t, message, tc.expectedMsg, "Success message should contain expected message substring")

					if tc.validateDb != nil {
						tc.validateDb(t)
					}
				}
			} else {
				if len(responseBodyBytes) > 0 {
					t.Errorf("Expected empty body for status %d, but received data", tc.expectedStatus)
				}
				// If no body is expected (e.g., for certain error statuses with no JSON response)
				if tc.expectedMsg != "" || tc.expectedError != "" {
					// Check body content if expected message or error implies a body
					checkResponseBodyContains(t, tc.expectedMsg+tc.expectedError, resp.Body.String())
				}
			}
		})
	}
}
