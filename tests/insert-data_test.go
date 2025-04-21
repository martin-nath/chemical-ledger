package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/utils"
)

func TestInsertData(t *testing.T) {
	utils.SetupTestDB()
	defer utils.TeardownTestDB()

	t.Run("Valid Data Insertion", func(t *testing.T) {
		pastDate := "2006-01-02"
		validPayload := map[string]any{
			"type":              utils.TypeIncoming,
			"date":              pastDate,
			"remark":            "Test Remark",
			"voucher_no":        "12345",
			"compound_id":       "sodiumChloride",
			"num_of_units":      10,
			"quantity_per_unit": 5,
		}

		req := utils.CreateRequest(http.MethodPost, "/insert", validPayload)
		resp := utils.ExecuteRequest(req, handlers.InsertData)

		utils.CheckResponseCode(t, http.StatusCreated, resp.Code)
		utils.CheckResponseBodyContains(t, utils.Entry_inserted_successfully, resp.Body.String())
	})

	t.Run("Invalid Input Handling", func(t *testing.T) {
		pastDate := "2006-01-02"
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
					"date":              time.Now().AddDate(0, 1, 0).Format("2006-01-02"),
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
				req := utils.CreateRequest(method, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)

				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
				utils.CheckResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Edge Cases", func(t *testing.T) {
		pastDate := "2006-01-02"
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Compound Existence Validation", func(t *testing.T) {
		pastDate := "2006-01-02"
		invalidCompoundPayload := map[string]any{
			"type":              utils.TypeIncoming,
			"date":              pastDate,
			"remark":            "Test Remark",
			"voucher_no":        "12345",
			"compound_id":       "nonExistentCompound",
			"num_of_units":      10,
			"quantity_per_unit": 5,
		}

		req := utils.CreateRequest(http.MethodPost, "/insert", invalidCompoundPayload)
		resp := utils.ExecuteRequest(req, handlers.InsertData)

		utils.CheckResponseCode(t, http.StatusNotFound, resp.Code)
		utils.CheckResponseBodyContains(t, utils.Item_not_found, resp.Body.String())
	})

	t.Run("More Invalid Input", func(t *testing.T) {
		pastDate := "2006-01-02"
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
				utils.CheckResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("String Length Validation", func(t *testing.T) {
		pastDate := "2006-01-02"
		longString := strings.Repeat("A", 256)

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
				expectedStatus: http.StatusCreated,
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
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
				if tc.expectedError != "" {
					utils.CheckResponseBodyContains(t, tc.expectedError, resp.Body.String())
				}
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
					"date":              "1970-01-01",
					"remark":            "Epoch",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
				expectedError:  "",
			},
			{
				name: "Near Future Date",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
				if tc.expectedError != "" {
					utils.CheckResponseBodyContains(t, tc.expectedError, resp.Body.String())
				}
			})
		}
	})

	t.Run("Case Sensitivity", func(t *testing.T) {
		pastDate := "2006-01-02"
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
				utils.CheckResponseBodyContains(t, tc.expectedError, resp.Body.String())
			})
		}
	})

	t.Run("Boundary Values", func(t *testing.T) {
		pastDate := "2006-01-02"
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
				expectedStatus: http.StatusCreated,
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Trailing Whitespace", func(t *testing.T) {
		pastDate := "2006-01-02"
		testCases := []struct {
			name           string
			requestBody    map[string]any
			expectedStatus int
		}{
			{
				name: "Trailing Whitespace in Remark",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark ",
					"voucher_no":        "123",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
			{
				name: "Trailing Whitespace in Voucher",
				requestBody: map[string]any{
					"type":              utils.TypeIncoming,
					"date":              pastDate,
					"remark":            "Test Remark",
					"voucher_no":        "123 ",
					"compound_id":       "benzene",
					"num_of_units":      10,
					"quantity_per_unit": 5,
				},
				expectedStatus: http.StatusCreated,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})

	t.Run("Unicode Characters", func(t *testing.T) {
		pastDate := "2006-01-02"
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
				req := utils.CreateRequest(http.MethodPost, "/insert", tc.requestBody)
				resp := utils.ExecuteRequest(req, handlers.InsertData)
				utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
			})
		}
	})
}
