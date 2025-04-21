package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertGetDataTestData(t *testing.T) error {
	_, err := db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty1', 5, 10)")
	require.NoError(t, err, "failed to insert test quantity 'qty1'")
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty2', 3, 20)")
	require.NoError(t, err, "failed to insert test quantity 'qty2'")
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty3', 2, 15)")
	require.NoError(t, err, "failed to insert test quantity 'qty3'")

	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)
	twoDaysAgo := yesterday.Add(-24 * time.Hour)

	twoDaysAgoTimestamp, err := utils.UnixTimestamp(utils.FormatDate(twoDaysAgo))
	require.NoError(t, err, "failed to create timestamp for two days ago")

	yesterdayTimestamp, err := utils.UnixTimestamp(utils.FormatDate(yesterday))
	require.NoError(t, err, "failed to create timestamp for yesterday")

	todayTimestamp, err := utils.UnixTimestamp(utils.FormatDate(today))
	require.NoError(t, err, "failed to create timestamp for today")

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry1', 'incoming', ?, 'Initial stock', 'V001', 50, 'aceticAcid', 'qty1')
	`, twoDaysAgoTimestamp)
	require.NoError(t, err, "failed to insert test entry 'entry1'")

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry2', 'outgoing', ?, 'Experiment use', 'V002', 30, 'aceticAcid', 'qty2')
	`, yesterdayTimestamp)
	require.NoError(t, err, "failed to insert test entry 'entry2'")

	_, err = db.Db.Exec(`
		INSERT INTO entry (id, type, date, remark, voucher_no, net_stock, compound_id, quantity_id)
		VALUES ('entry3', 'incoming', ?, 'New stock', 'V003', 60, 'ethanol', 'qty3')
	`, todayTimestamp)
	require.NoError(t, err, "failed to insert test entry 'entry3'")
	return nil
}

func createRequest(method, url string, body map[string]any) *http.Request {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(fmt.Sprintf("Failed to marshal request body: %v", err))
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		panic(fmt.Sprintf("Failed to create request: %v", err))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req
}

func TestGetData(t *testing.T) {
	utils.SetupTestDB()
	defer utils.TeardownTestDB()

	err := insertGetDataTestData(t)
	require.NoError(t, err, "failed to insert test data")

	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	test := []struct {
		name           string
		queryParams    map[string]string
		expectedStatus int
		validateResp   func(t *testing.T, body string, resp map[string]any)
	}{
		{
			name:           "Get all entries",
			queryParams:    map[string]string{},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(3), total, "Should return all 3 entries")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 3, "Should return 3 entries")
			},
		},
		{
			name:           "Filter by type - incoming",
			queryParams:    map[string]string{"type": "incoming"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(2), total, "Should return 2 incoming entries")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 2, "Should return 2 entries")

				for _, result := range results {
					entry, ok := result.(map[string]any)
					require.True(t, ok, "Each result should be an entry map")
					assert.Equal(t, "incoming", entry["type"], "Entry should be of type 'incoming'")
				}
			},
		},
		{
			name:           "Filter by type - outgoing",
			queryParams:    map[string]string{"type": "outgoing"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 outgoing entry")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]any)
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "outgoing", entry["type"], "Entry should be of type 'outgoing'")
			},
		},
		{
			name:           "Filter by compound name",
			queryParams:    map[string]string{"compound": "Acetic acid"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(2), total, "Should return 2 entries for 'Acetic acid'")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 2, "Should return 2 entries")

				for _, result := range results {
					entry, ok := result.(map[string]any)
					require.True(t, ok, "Each result should be an entry map")
					assert.Equal(t, "Acetic acid", entry["compound_name"], "Entry should be for 'Acetic acid'")
				}
			},
		},
		{
			name: "Filter by date range (yesterday to today)",
			queryParams: map[string]string{
				"fromDate": utils.FormatDate(yesterday),
				"toDate":   utils.FormatDate(today),
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(2), total, "Should return 2 entries within the date range (yesterday and today's entry)")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 2, "Should return 2 entries")
			},
		},
		{
			name: "Filter by single date (today)",
			queryParams: map[string]string{
				"fromDate": utils.FormatDate(today),
				"toDate":   utils.FormatDate(today),
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry for today")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")
			},
		},
		{
			name: "Combined filters (type and compound)",
			queryParams: map[string]string{
				"type":     "incoming",
				"compound": "Ethanol",
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry matching both filters")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]any)
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "incoming", entry["type"], "Entry should be of type 'incoming'")
				assert.Equal(t, "Ethanol", entry["compound_name"], "Entry should be for 'Ethanol'")
			},
		},
		{
			name: "Combined filters (type, compound, and date)",
			queryParams: map[string]string{
				"type":     "outgoing",
				"compound": "Acetic acid",
				"fromDate": utils.FormatDate(yesterday),
				"toDate":   utils.FormatDate(yesterday),
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(1), total, "Should return 1 entry matching all filters")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 1, "Should return 1 entry")

				entry, ok := results[0].(map[string]any)
				require.True(t, ok, "Result should be an entry map")
				assert.Equal(t, "outgoing", entry["type"], "Entry should be of type 'outgoing'")
				assert.Equal(t, "Acetic acid", entry["compound_name"], "Entry should be for 'Acetic acid'")
			},
		},
		{
			name:           "Filter returning no results",
			queryParams:    map[string]string{"type": "outgoing", "compound": "Ethanol"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				data, ok := resp["data"].(map[string]any)
				require.True(t, ok, "Response data should be a map")

				total, ok := data["total"].(float64)
				require.True(t, ok, "Total count should be a number")
				assert.Equal(t, float64(0), total, "Should return 0 entries")

				results, ok := data["results"].([]any)
				require.True(t, ok, "Results should be an array")
				assert.Len(t, results, 0, "Should return 0 entries")
			},
		},
		{
			name:           "Invalid type filter",
			queryParams:    map[string]string{"type": "invalid"},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Contains(t, errorMsg, "Invalid 'type' filter", "Error message should indicate invalid type")
				utils.CheckResponseBodyContains(t, "Invalid 'type' filter", body)
			},
		},
		{
			name:           "Invalid fromDate format",
			queryParams:    map[string]string{"fromDate": "invaliddateformat"},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Invalid_date_format, errorMsg, "Error message should match Invalid_date_format util constant")
				utils.CheckResponseBodyContains(t, utils.Invalid_date_format, body)
			},
		},
		{
			name:           "Invalid toDate format",
			queryParams:    map[string]string{"toDate": "invaliddateformat"},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Invalid_date_format, errorMsg, "Error message should match Invalid_date_format util constant")
				utils.CheckResponseBodyContains(t, utils.Invalid_date_format, body)
			},
		},
		{
			name:           "Future fromDate",
			queryParams:    map[string]string{"fromDate": utils.FormatDate(today.Add(2 * 24 * time.Hour))},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Future_date_error, errorMsg, "Error message should match Future_date_error util constant")
				utils.CheckResponseBodyContains(t, utils.Future_date_error, body)
			},
		},
		{
			name: "Future toDate (fromDate is valid)",
			queryParams: map[string]string{
				"fromDate": utils.FormatDate(today),
				"toDate":   utils.FormatDate(today.Add(2 * 24 * time.Hour)),
			},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Equal(t, utils.Future_date_error, errorMsg, "Error message should match Future_date_error util constant")
				utils.CheckResponseBodyContains(t, utils.Future_date_error, body)
			},
		},
		{
			name: "Invalid date range (toDate before fromDate)",
			queryParams: map[string]string{
				"fromDate": utils.FormatDate(today),
				"toDate":   utils.FormatDate(yesterday),
			},
			expectedStatus: http.StatusBadRequest,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				errorMsg, ok := resp["error"].(string)
				require.True(t, ok, "Response should contain an 'error' string")
				assert.Contains(t, errorMsg, "Invalid date range", "Error message should indicate invalid date range")
				utils.CheckResponseBodyContains(t, "Invalid date range", body)
			},
		},
		{
			name:           "Filter by type=both",
			queryParams:    map[string]string{"type": "both"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				assert.Equal(t, float64(3), resp["data"].(map[string]any)["total"])
			},
		},
		{
			name:           "Compound name is case-sensitive",
			queryParams:    map[string]string{"compound": "acetic acid"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				assert.Equal(t, float64(0), resp["data"].(map[string]any)["total"])
			},
		},
		{
			name:           "Nonexistent compound returns zero",
			queryParams:    map[string]string{"compound": "Nonexistent Acid"},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				assert.Equal(t, float64(0), resp["data"].(map[string]any)["total"])
			},
		},
		{
			name:           "Results sorted descending date",
			queryParams:    nil,
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, body string, resp map[string]any) {
				res := resp["data"].(map[string]any)["results"].([]any)
				parse := func(s any) time.Time {
					t, _ := time.Parse("2006-01-02 15:04:05", s.(string))
					return t
				}
				t0 := parse(res[0].(map[string]any)["date"])
				t1 := parse(res[1].(map[string]any)["date"])
				t2 := parse(res[2].(map[string]any)["date"])
				assert.True(t, t0.After(t1) || t0.Equal(t1))
				assert.True(t, t1.After(t2) || t1.Equal(t2))
			},
		},
	}

	for _, tc := range test {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := "/fetch"
			u, err := url.Parse(baseURL)
			require.NoError(t, err, "failed to parse base URL")

			q := u.Query()
			for key, value := range tc.queryParams {
				q.Set(key, value)
			}
			u.RawQuery = q.Encode()
			finalURL := u.String()

			req := createRequest(http.MethodGet, finalURL, nil)
			rr := utils.ExecuteRequest(req, http.HandlerFunc(handlers.GetData))

			utils.CheckResponseCode(t, tc.expectedStatus, rr.Code)

			var response map[string]any
			responseBodyBytes := rr.Body.Bytes()

			if len(responseBodyBytes) > 0 {
				err = json.Unmarshal(responseBodyBytes, &response)
				if tc.expectedStatus >= 400 {
					require.NoError(t, err, fmt.Sprintf("Failed to parse error response JSON for status %d: %s", rr.Code, rr.Body.String()))
				} else {
					require.NoError(t, err, fmt.Sprintf("Failed to parse success response JSON for status %d: %s", rr.Code, rr.Body.String()))
				}
			} else {
				response = make(map[string]any)
			}

			tc.validateResp(t, rr.Body.String(), response)
		})
	}
}
