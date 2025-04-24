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

	testCases := []struct {
		name           string
		reqBody        map[string]any // Using any to allow different types for test cases
		expectedStatus int
	}{
		{
			name: "Basic Update Remark",
			reqBody: map[string]any{
				"id":     "entry1",         // string
				"remark": "Updated remark", // string
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update Voucher No",
			reqBody: map[string]any{
				"id":      "entry1",       // string
				"voucher": "V002_updated", // string
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update Type",
			reqBody: map[string]any{
				"id":   "entry2",   // string
				"type": "incoming", // string
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update Date",
			reqBody: map[string]any{
				"id":   "entry3",     // string
				"date": "2023-10-26", // string (expected date format)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update Compound ID",
			reqBody: map[string]any{
				"id":          "entry1",  // string
				"compound_id": "benzene", // string
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update NumOfUnits",
			reqBody: map[string]any{
				"id":           "entry2", // string
				"num_of_units": 10,       // int
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Basic Update QuantityPerUnit",
			reqBody: map[string]any{
				"id":                "entry2", // string
				"quantity_per_unit": 25,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Multiple Entry Fields",
			reqBody: map[string]any{
				"id":      "entry1",                  // string
				"remark":  "Updated remark multiple", // string
				"voucher": "V001_multi",              // string
				"date":    "2024-01-15",              // string (expected date format)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Multiple Quantity Fields",
			reqBody: map[string]any{
				"id":                "entry2", // string
				"num_of_units":      5,        // int
				"quantity_per_unit": 50,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Mixed Entry and Quantity Fields",
			reqBody: map[string]any{
				"id":                "entry3",            // string
				"remark":            "Mixed update test", // string
				"num_of_units":      1,                   // int
				"quantity_per_unit": 100,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update Non-Existent Entry",
			reqBody: map[string]any{
				"id":     "nonexistent_entry", // string
				"remark": "This should fail",  // string
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid Type",
			reqBody: map[string]any{
				"id":   "entry1",       // string
				"type": "invalid_type", // string
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid Date Format",
			reqBody: map[string]any{
				"id":   "entry1",     // string
				"date": "2023/10/26", // string (invalid date format)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid Date Value",
			reqBody: map[string]any{
				"id":   "entry1",              // string
				"date": "invalid-date-string", // string (invalid date value)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid NumOfUnits (Negative)",
			reqBody: map[string]any{
				"id":           "entry2", // string
				"num_of_units": -5,       // int (negative value)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid NumOfUnits (Non-integer type)",
			reqBody: map[string]any{
				"id":           "entry2", // string
				"num_of_units": "abc",    // string (incorrect type)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid QuantityPerUnit (Negative)",
			reqBody: map[string]any{
				"id":                "entry2", // string
				"quantity_per_unit": -10,      // (negative value)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid QuantityPerUnit (Non-numeric type)",
			reqBody: map[string]any{
				"id":                "entry2", // string
				"quantity_per_unit": "xyz",    // string (incorrect type)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Invalid Compound ID Type (Number instead of string)",
			reqBody: map[string]any{
				"id":          "entry1", // string
				"compound_id": 12345,    // int (incorrect type)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty Request Body",
			reqBody:        map[string]any{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Request Body Missing Entry ID",
			reqBody: map[string]any{
				"remark": "Should fail without ID", // string
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Update with Empty Remark String",
			reqBody: map[string]any{
				"id":     "entry1", // string
				"remark": "",       // string (empty)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update with Empty Voucher String",
			reqBody: map[string]any{
				"id":      "entry1", // string
				"voucher": "",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Update NumOfUnits to Zero",
			reqBody: map[string]any{
				"id":           "entry2", // string
				"num_of_units": 0,        // int
			},
			expectedStatus: http.StatusOK, // Assuming zero units is allowed
		},
		{
			name: "Update QuantityPerUnit to Zero",
			reqBody: map[string]any{
				"id":                "entry2", // string
				"quantity_per_unit": 0,        // int
			},
			expectedStatus: http.StatusOK, // Assuming zero quantity per unit is allowed
		},
		{
			name: "Update with extra field in body",
			reqBody: map[string]any{
				"id":      "entry1",      // string
				"remark":  "Remark with extra", // string
				"extra_field": "some_value", // extra field
			},
			expectedStatus: http.StatusOK, // Assuming extra fields are ignored
		},
		// Additional Test Cases
{
    name: "Basic Update Net Stock (should be ignored or error)",
    reqBody: map[string]any{
        "id":        "entry1", // string
        "net_stock": 999,      // int (assuming net_stock is calculated and not directly updatable)
    },
    expectedStatus: http.StatusOK, // Assuming the field is ignored, or Bad Request if handler validates against extra fields not in UpdatedEntry
},
{
    name: "Update Multiple Entry and Quantity Fields - Different Entry",
    reqBody: map[string]any{
        "id":                "entry1", // string
        "type":              "outgoing", // string
        "date":              "2023-11-01", // string
        "num_of_units":      3,          // int
        "quantity_per_unit": 15,         // int
        "remark":            "Full update test", // string
        "voucher":           "V001_new", // string
        "compound_id":       "ethanol",  // string
    },
    expectedStatus: http.StatusOK,
},
{
    name: "Update with Invalid Compound ID (does not exist)",
    reqBody: map[string]any{
        "id":          "entry1",             // string
        "compound_id": "nonexistent_compound", // string
    },
    expectedStatus: http.StatusNotFound, // Assuming CheckIfCompoundExists returns NotFound
},
{
    name: "Update Date to Earlier Than Previous Entry (requires subsequent recalculation)",
    reqBody: map[string]any{
        "id":   "entry2",          // string (originally yesterday)
        "date": "2023-10-26",      // string (same date as entry1, requires reordering and recalculation)
    },
    expectedStatus: http.StatusOK, // Assuming successful update and recalculation
},
{
    name: "Update Quantity Causing Insufficient Stock for Subsequent Outgoing Entry",
    reqBody: map[string]any{
        "id":                "entry1", // string (originally 5 * 10 = 50 units, net stock 50)
        "num_of_units":      1,        // int
        "quantity_per_unit": 1,        // int (new quantity 1 * 1 = 1)
        // This update should cause entry2 (outgoing, originally 2 * 20 = 40 units) to fail due to insufficient stock
    },
    expectedStatus: http.StatusNotAcceptable, // Assuming insufficient stock error is returned
},
{
    name: "Update Type from Incoming to Outgoing",
    reqBody: map[string]any{
        "id":   "entry1",     // string (originally incoming)
        "type": "outgoing",   // string
    },
    expectedStatus: http.StatusOK, // Assuming successful update and recalculation
},
{
    name: "Update Type from Outgoing to Incoming",
    reqBody: map[string]any{
        "id":   "entry2",     // string (originally outgoing)
        "type": "incoming",   // string
    },
    expectedStatus: http.StatusOK, // Assuming successful update and recalculation
},
{
    name: "Update Compound ID (requires recalculation for old and new compounds)",
    reqBody: map[string]any{
        "id":          "entry1",  // string (originally aceticAcid)
        "compound_id": "ethanol", // string
    },
    expectedStatus: http.StatusOK, // Assuming successful update and recalculation for both compounds
},
{
    name: "Update Remark with Special Characters",
    reqBody: map[string]any{
        "id":     "entry1", // string
        "remark": "Updated with !@#$%^&*()_+", // string
    },
    expectedStatus: http.StatusOK,
},
{
    name: "Update Voucher No with Special Characters",
    reqBody: map[string]any{
        "id":      "entry1", // string
        "voucher": "Voucher: <>?\"'{}[];", // string
    },
    expectedStatus: http.StatusOK,
},
{
    name: "Update Date to a Leap Year Date",
    reqBody: map[string]any{
        "id":   "entry1", // string
        "date": "2024-02-29", // string
    },
    expectedStatus: http.StatusOK,
},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := utils.CreateRequest(http.MethodPut, "/update", tc.reqBody)
			resp := utils.ExecuteRequest(req, handlers.UpdateData)

			utils.CheckResponseCode(t, tc.expectedStatus, resp.Code)
		})
	}
}
