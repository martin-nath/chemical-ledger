package tests

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "sync"
    "testing"

    "github.com/martin-nath/chemical-ledger/db"
    "github.com/martin-nath/chemical-ledger/handlers"
    "github.com/martin-nath/chemical-ledger/migrate"
)

func setupTestDB() {
    // Initialize in-memory database
    db.InitDB(":memory:")

    // Run migrations
    if err := migrate.CreateTables(db.Db); err != nil {
        panic("Failed to create tables: " + err.Error())
    }
}

func teardownTestDB() {
    db.Db.Close()
}

func TestInsertData(t *testing.T) {
    setupTestDB()
    defer teardownTestDB()

    t.Run("Basic", func(t *testing.T) {
        type TestCase struct {
            name           string
            requestBody    map[string]interface{}
            expectedStatus int
        }

        testCases := []TestCase{
            {
                name: "Valid Incoming Transaction",
                requestBody: map[string]interface{}{
                    "type":             "incoming",
                    "date":             "2025-04-15",
                    "remark":           "Test Remark",
                    "voucher_no":       "12345",
                    "compound_name":    "TestCompound",
                    "scale":            "mg",
                    "num_of_units":     10,
                    "quantity_per_unit": 5,
                },
                expectedStatus: http.StatusCreated,
            },
            {
                name: "Missing Required Field",
                requestBody: map[string]interface{}{
                    "type":          "incoming",
                    "date":          "2025-04-15",
                    "remark":        "Test Remark",
                    "voucher_no":    "12345",
                    "compound_name": "TestCompound",
                    "scale":         "mg",
                    "num_of_units":  10,
                    // Missing "quantity_per_unit"
                },
                expectedStatus: http.StatusBadRequest,
            },
            {
                name: "Invalid Date Format",
                requestBody: map[string]interface{}{
                    "type":             "incoming",
                    "date":             "2025-04-15", // Invalid date format
                    "remark":           "Test Remark",
                    "voucher_no":       "12345",
                    "compound_name":    "TestCompound",
                    "scale":            "mg",
                    "num_of_units":     10,
                    "quantity_per_unit": 5,
                },
                expectedStatus: http.StatusBadRequest,
            },
            {
                name: "Outgoing Transaction with Insufficient Stock",
                requestBody: map[string]interface{}{
                    "type":             "outgoing",
                    "date":             "2025-04-15",
                    "remark":           "Test Remark",
                    "voucher_no":       "12345",
                    "compound_name":    "TestCompound",
                    "scale":            "mg",
                    "num_of_units":     20,
                    "quantity_per_unit": 5,
                },
                expectedStatus: http.StatusBadRequest,
            },
            {
                name: "Empty Payload",
                requestBody:    map[string]interface{}{},
                expectedStatus: http.StatusBadRequest,
            },
        }

        var wg sync.WaitGroup

        for _, tc := range testCases {
            wg.Add(1)
            go func(tc TestCase) {
                defer wg.Done()
                t.Run(tc.name, func(t *testing.T) {
                    requestBody, _ := json.Marshal(tc.requestBody)
                    req := httptest.NewRequest(http.MethodPost, "/insert", bytes.NewBuffer(requestBody))
                    req.Header.Set("Content-Type", "application/json")

                    respRec := httptest.NewRecorder()
                    handlers.InsertData(respRec, req)

                    if respRec.Code != tc.expectedStatus {
                        t.Errorf("Test %q failed: expected status %v; got %v\nResponse Body: %s", tc.name, tc.expectedStatus, respRec.Code, respRec.Body.String())
                    }
                })
            }(tc)
        }

        wg.Wait()
    })

    t.Run("Advanced", func(t *testing.T) {
        type TestCase struct {
            name           string
            requestBody    map[string]interface{}
            expectedStatus int
        }

        // Insert initial stock for advanced tests
        _, err := db.Db.Exec(`
            INSERT INTO compound (id, name, scale) VALUES ('TestCompound', 'TestCompound', 'mg');
            INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES ('Q_TestCompound_1', 10, 5);
            INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
            VALUES ('E_TestCompound_1', 'incoming', '2025-04-15', 'TestCompound', 'Initial Stock', '12345', 'Q_TestCompound_1', 50);
        `)
        if err != nil {
            t.Fatalf("Failed to insert initial stock: %v", err)
        }

        testCases := []TestCase{
            {
                name: "Valid Outgoing Transaction",
                requestBody: map[string]interface{}{
                    "type":             "outgoing",
                    "date":             "2025-04-15",
                    "remark":           "Test Remark",
                    "voucher_no":       "12345",
                    "compound_name":    "TestCompound",
                    "scale":            "mg",
                    "num_of_units":     5,
                    "quantity_per_unit": 5,
                },
                expectedStatus: http.StatusCreated,
            },
            {
                name: "Outgoing Transaction with Nonexistent Compound",
                requestBody: map[string]interface{}{
                    "type":             "outgoing",
                    "date":             "2025-04-15",
                    "remark":           "Test Remark",
                    "voucher_no":       "12345",
                    "compound_name":    "NonexistentCompound",
                    "scale":            "mg",
                    "num_of_units":     5,
                    "quantity_per_unit": 5,
                },
                expectedStatus: http.StatusNotFound,
            },
        }

        for _, tc := range testCases {
            t.Run(tc.name, func(t *testing.T) {
                requestBody, _ := json.Marshal(tc.requestBody)
                req := httptest.NewRequest(http.MethodPost, "/insert", bytes.NewBuffer(requestBody))
                req.Header.Set("Content-Type", "application/json")

                respRec := httptest.NewRecorder()
                handlers.InsertData(respRec, req)

                if respRec.Code != tc.expectedStatus {
                    t.Errorf("Test %q failed: expected status %v; got %v\nResponse Body: %s", tc.name, tc.expectedStatus, respRec.Code, respRec.Body.String())
                }
            })
        }
    })
}