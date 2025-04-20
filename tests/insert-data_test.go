package tests

import (
	"bytes"
	"encoding/json"
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
)

func setupTestDB() {
	db.InitDB("test.db")
	if err := migrate.CreateTables(db.Db); err != nil {
		panic("Failed to create tables: " + err.Error())
	}
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

func createRequest(method, url string, body map[string]any) *http.Request {
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req
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
		resp := executeRequest(req, handlers.InsertData)

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
				resp := executeRequest(req, handlers.InsertData)

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
				resp := executeRequest(req, handlers.InsertData)
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
		resp := executeRequest(req, handlers.InsertData)

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
				resp := executeRequest(req, handlers.InsertData)
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
				expectedStatus: http.StatusBadRequest, // Or appropriate validation error
				expectedError:  utils.MissingFields_or_inappropriate_value,
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
				expectedStatus: http.StatusBadRequest,
				expectedError:  utils.MissingFields_or_inappropriate_value,
			},
			// Add more cases for other string fields if needed
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := createRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := executeRequest(req, handlers.InsertData)
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
				resp := executeRequest(req, handlers.InsertData)
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
				name: "Lowercase Type",
				requestBody: map[string]any{
					"type":              strings.ToLower(utils.TypeIncoming),
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
				resp := executeRequest(req, handlers.InsertData)
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
				resp := executeRequest(req, handlers.InsertData)
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
				resp := executeRequest(req, handlers.InsertData)
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
				resp := executeRequest(req, handlers.InsertData)
				checkResponseCode(t, tc.expectedStatus, resp.Code)
				checkResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})
}
