package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/stretchr/testify/require"
)

func insertUpdateDataTestData(t *testing.T) error {
	// TODO: Use insert data route or handler to insert test data for update data route
	_, err := db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty1', 5, 10)")
	require.NoError(t, err, "failed to insert test quantity 'qty1'")
	_, err = db.Db.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('qty2', 2, 20)")
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

func TestUpdateData(t *testing.T) {
	utils.SetupTestDB()
	defer utils.TeardownTestDB()

	err := insertUpdateDataTestData(t)
	require.NoError(t, err, "failed to insert test data")

	test := []struct {
		name           string
		reqBody        map[string]any
		expectedStatus int
	}{
		{
			name: "Update Remark",
			reqBody: map[string]any{
				"entry_id": "entry1",
				"remark":   "Updated remark",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Voucher No",
			reqBody: map[string]any{
				"entry_id": "entry1",
				"voucher":  "V002",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Type",
			reqBody: map[string]any{
				"entry_id": "entry2",
				"type":     "incoming",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Date",
			reqBody: map[string]any{
				"entry_id": "entry3",
				"date":     "01-02-2004",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range test {
		t.Run(tc.name, func(t *testing.T) {
			req := utils.CreateRequest(http.MethodPut, "/update", tc.reqBody)
			resp := utils.ExecuteRequest(req, handlers.UpdateData)
			utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)

			utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
		})
	}
}
