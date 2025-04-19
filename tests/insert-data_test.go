package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/migrate"
	"github.com/martin-nath/chemical-ledger/utils"
)

// TODO: Fix the tests
// TODO: Add tests for the update-data.go file
// TODO: Add tests for the get-data.go file

func setupTestDB() {
	// Initialize in-memory database
	db.InitDB(":memory:")

	// Run migrations
	if err := migrate.CreateTables(db.Db); err != nil {
		panic("Failed to create tables: " + err.Error())
	}
}

func teardownTestDB() {
	defer db.Db.Close()
	err := migrate.DropTables(db.Db)
	if err != nil {
		panic("Failed to drop tables: " + err.Error())
	}
}

func executeRequest(req *http.Request, handler http.HandlerFunc) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func checkResponseCode(t *testing.T, expected, actual int) {
	if expected != actual {
		t.Errorf("Expected response code %d, got %d", expected, actual)
	}
}

func checkResponseBodyContains(t *testing.T, expectedSubstring string, actualBody string) {
	if !strings.Contains(actualBody, expectedSubstring) {
		t.Errorf("Expected response body to contain '%s', \n but got '%s'", expectedSubstring, actualBody)
	}
}

func TestInsertData(t *testing.T) {
	setupTestDB()
	defer teardownTestDB()

	// Helper function to create a request
	createRequest := func(method, url string, body map[string]interface{}) *http.Request {
		reqBody, _ := json.Marshal(body)
		req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	t.Run("Basic Tests", func(t *testing.T) {
		pastDate := "01-01-2006"

		testCases := []struct {
			name           string
			requestBody    map[string]interface{}
			expectedStatus int
			expectedError  string // Optional: substring expected in the error message
		}{
			{
				name: "Valid Incoming Transaction - New Compound",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "sodiumChloride",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Missing Required Field - QuantityPerUnit",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":        "mg",
					"num_of_units": 10,
					// Missing "quantity_per_unit"
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Invalid Date Format",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        "15042025", // Invalid date format
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.Invalid_date_format,
			},
			{
				name: "Invalid Date - Future Date",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        time.Now().AddDate(0, 1, 0).Format("02-01-2006"), // Future date
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.Future_date_error,
			},
			{
				name: "Invalid Scale",
				requestBody: map[string]interface{}{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "12345",
					"compound_id":       "benzene",
					"scale":             "kg", // Invalid scale
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Invalid Type",
				requestBody: map[string]interface{}{
					"type":        "transfer", // Invalid type
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Zero QuantityPerUnit",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 0,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Zero NumOfUnits",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      0,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name:           "Empty Payload",
				requestBody:    map[string]interface{}{},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value, // Expecting date error for empty payload
			},
			{
				name: "Invalid Request Method - GET",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusMethodNotAllowed,
				expectedError:  utils.InvalidMethod,
			},
			{
				name: "Case Sensitivity - Invalid Scale (Uppercase)",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "MG", // Uppercase scale
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Case Sensitivity - Invalid Type (Uppercase)",
				requestBody: map[string]interface{}{
					"type":        "Incoming", // Uppercase type
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			{
				name: "Empty Remark and Voucher Number",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "",
					"voucher_no":  "",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.name == "Invalid Request Method - GET" {
					req = createRequest(http.MethodGet, "/insert", tc.requestBody)
				} else {
					req = createRequest(http.MethodPost, "/insert", tc.requestBody)
				}
				resp := executeRequest(req, handlers.InsertData)
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				if tc.expectedError != "" {
					checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
				}
			})
		}
	})

	t.Run("Advanced Tests", func(t *testing.T) {
		pastDate := time.Now().AddDate(0, -1, 0).Format("02-01-2006")

		// Helper function to create a request
		createRequest := func(method, url string, body map[string]interface{}) *http.Request {
			reqBody, _ := json.Marshal(body)
			req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			return req
		}

		// Helper function to insert initial stock
		insertInitialStock := func() {
			_, err := db.Db.Exec(`
				INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?);
				INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?);
			`,
				"Q_benzene_1", 10, 5,
				"E_benzene_1", utils.TypeIncoming, pastDate, "benzene", "Initial Stock", "12345", "Q_benzene_1", 50,
			)
			if err != nil {
				t.Fatalf("Failed to insert initial stock: %v", err)
			}
		}

		// Helper function to clear the database before each advanced test
		clearDatabase := func() {
			_, err := db.Db.Exec("DELETE FROM entry")
			if err != nil {
				t.Fatalf("Failed to delete from entry: %v", err)
			}
			_, err = db.Db.Exec("DELETE FROM quantity")
			if err != nil {
				t.Fatalf("Failed to delete from quantity: %v", err)
			}
			_, err = db.Db.Exec("DELETE FROM compound")
			if err != nil {
				t.Fatalf("Failed to delete from compound: %v", err)
			}
		}

		testCases := []struct {
			name           string
			requestBody    map[string]interface{}
			expectedStatus int
			expectedError  string // Optional: substring expected in the error message
		}{
			{
				name: "Valid Outgoing Transaction",
				requestBody: map[string]interface{}{
					"type":        utils.TypeOutgoing,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      5,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Outgoing Transaction with Exactly Enough Stock",
				requestBody: map[string]interface{}{
					"type":        utils.TypeOutgoing,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Outgoing Transaction with Insufficient Stock",
				requestBody: map[string]interface{}{
					"type":        utils.TypeOutgoing,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      11,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusNotAcceptable,
				expectedError:  utils.Insufficient_stock,
			},
			{
				name: "Outgoing Transaction for Nonexistent Compound",
				requestBody: map[string]interface{}{
					"type":        utils.TypeOutgoing,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "NonexistentCompound",
					// "scale":             "mg",
					"num_of_units":      5,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusNotFound,
				expectedError:  utils.Item_not_found,
			},
			{
				name: "Incoming Transaction for Existing Compound",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "67890",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      5,
					"quantity_per_unit": 10,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Incoming Transaction for Existing Compound with Different Scale (Should Work)",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "13579",
					"compound_id": "benzene",
					// "scale":             "ml", // Different scale, but compound already exists with 'mg' - this might be a design consideration, current logic allows it.
					"num_of_units":      2,
					"quantity_per_unit": 25,
				},
				expectedStatus: http.StatusCreated, // Expecting success, the logic allows this
			},
			{
				name: "Incoming Transaction for New Compound",
				requestBody: map[string]interface{}{
					"type":        utils.TypeIncoming,
					"date":        pastDate,
					"remark":      "New Compound Added",
					"voucher_no":  "24680",
					"compound_id": "sodiumChloride",
					// "scale":             "ml",
					"num_of_units":      3,
					"quantity_per_unit": 100,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Outgoing Transaction with Exactly Enough Stock - Boundary",
				requestBody: map[string]interface{}{
					"type":        utils.TypeOutgoing,
					"date":        pastDate,
					"remark":      "Test Remark",
					"voucher_no":  "12345",
					"compound_id": "benzene",
					// "scale":             "mg",
					"num_of_units":      5,
					"quantity_per_unit": 10, // Total withdrawal of 5 * 10 = 50, which is the initial stock
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				clearDatabase() // Clear database before each test case
				insertInitialStock()
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, handlers.InsertData)
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				if tc.expectedError != "" {
					checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
				}
			})
		}
	})
}
