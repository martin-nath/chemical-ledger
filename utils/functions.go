package utils

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/migrate"
	"github.com/sirupsen/logrus"
)

// Retry executes the given function and retries it if it returns an error.
// It retries up to the number of times defined by MaxRetries with a delay of RetryDelay between retries.
func Retry(fn func() error) error {
	var err error
	for i := range MaxRetries {
		err = fn()
		if err == nil {
			return nil
		}
		logrus.Debugf("Error after retry #%d: %v", i+1, err)
		time.Sleep(RetryDelay)
	}
	return err
}

// UnixTimestamp converts a date string in "YYYY-MM-DD" format to a Unix timestamp.
// It combines the provided date with the current time's hour, minute, and second in the local timezone.
// Returns the Unix timestamp and nil error on success, or 0 and an error on failure.
func UnixTimestamp(dateStr string) (int64, error) {
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0, err
	}

	now := time.Now()

	combined := time.Date(date.Year(), date.Month(), date.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, time.Local)

	return combined.Unix(), nil
}

// JsonRes sends a JSON response to the client with the specified HTTP status code and response object.
// It sets the "Content-Type" header to "application/json" before encoding the response.
func JsonRes(w http.ResponseWriter, status int, resObj *Resp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resObj)
}

// JsonReq decodes the JSON request body into the provided destination struct.
// If the decoding fails, it logs the error and sends a 400 Bad Request response with a generic error message.
// Returns nil on successful decoding, or an error if decoding fails.
func JsonReq(r *http.Request, dst any, w http.ResponseWriter) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		logrus.Errorf("Error decoding JSON request body: %v", err)
		JsonRes(w, http.StatusBadRequest, &Resp{Error: Req_body_decode_error})
		return err
	}
	return nil
}

// UpdateSubSequentNetStock updates the net stock of all subsequent entries for a given compound starting from the specified date.
// It retrieves the stock level before the given entry date and then iterates through subsequent entries, updating their net stock based on their type and quantity.
// It performs these operations within the provided database transaction.
func UpdateSubSequentNetStock(dbTx *sql.Tx, entryDate int64, compoundId string) string {
	var previousStock int
	err := Retry(func() error {
		err := dbTx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? AND date < ? ORDER BY date DESC LIMIT 1", compoundId, entryDate).Scan(&previousStock)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Stock_retrieval_error})
			logrus.Errorf("Error retrieving previous stock for compound '%s': %v", compoundId, err)
			return errors.New("error retrieving previous stock")
		}
		return nil
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Stock_retrieval_error})
		logrus.Errorf("Error retrieving previous stock for compound '%s': %v", compoundId, err)
		return Stock_retrieval_error
	}

	var rows *sql.Rows
	err = Retry(func() error {
		var queryErr error
		rows, queryErr = dbTx.Query(`
			SELECT
				e.id AS entry_id,
				e.type AS entry_type,
				q.num_of_units * q.quantity_per_unit AS quantity,
				e.date AS entry_date
			FROM entry e
			JOIN quantity q ON e.quantity_id = q.id
			WHERE
				e.compound_id = ? AND e.date >= ?
			ORDER BY
				e.date ASC
		`, compoundId, entryDate)
		return queryErr
	})

	if err != nil {
		// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Stock_retrieval_error})
		logrus.Errorf("Error retrieving subsequent entries for compound '%s': %v", compoundId, err)
		return Stock_retrieval_error
	}
	defer rows.Close()

	var updateQueriesBuilder strings.Builder
	for rows.Next() {
		var entryUpdate struct {
			EntryID   string
			EntryType string
			Quantity  int
			EntryDate int64
		}
		err := rows.Scan(&entryUpdate.EntryID, &entryUpdate.EntryType, &entryUpdate.Quantity, &entryUpdate.EntryDate)
		if err != nil {
			// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Entry_update_scan_error})
			logrus.Errorf("Error reading entry details while updating stock: %v", err)
			return Entry_update_scan_error
		}

		switch entryUpdate.EntryType {
		case TypeIncoming:
			previousStock += entryUpdate.Quantity
		case TypeOutgoing:
			previousStock -= entryUpdate.Quantity
			if previousStock < 0 {
				// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Insufficient_stock})
				logrus.Errorf("Error calculating net stock after entry '%s': insufficient stock", entryUpdate.EntryID)
				return Insufficient_stock
			}
		}

		updateQueriesBuilder.WriteString(fmt.Sprintf("UPDATE entry SET net_stock = %d WHERE id = '%s';\n", previousStock, entryUpdate.EntryID))
	}

	updateQueries := updateQueriesBuilder.String()
	if updateQueries != "" {
		_, err = dbTx.Exec(updateQueries)
		if err != nil {
			// JsonRes(w, http.StatusInternalServerError, &Resp{Error: Update_subsequent_entries_error})
			logrus.Errorf("Error saving the updated stock information for compound '%s': %v", compoundId, err)
			return Update_subsequent_entries_error
		}
		logrus.Debugf("Updated net stock for subsequent entries of compound '%s'", compoundId)
	}

	return ""
}

// BeginDbTx starts a new database transaction.
// It retries the operation based on the Retry policy.
// Returns the transaction object and nil error on success, or nil and an error on failure.
func BeginDbTx(w http.ResponseWriter) (*sql.Tx, error) {
	var dbTx *sql.Tx
	err := Retry(func() error {
		var err error
		dbTx, err = db.Db.Begin()
		return err
	})

	if err != nil {
		logrus.Errorf("Error starting database transaction: %v", err)
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Record_transaction_error})
		return nil, err
	}
	return dbTx, nil
}

// CommitDbTx commits the given database transaction.
// If the commit fails, it logs the error and sends a 500 Internal Server Error response.
// Returns nil on successful commit, or an error if the commit fails.
func CommitDbTx(dbTx *sql.Tx, w http.ResponseWriter) error {
	if err := dbTx.Commit(); err != nil {
		logrus.Errorf("Error committing database transaction: %v", err)
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Commit_transaction_error})
		return err
	}
	logrus.Debug("Database transaction committed successfully.")
	return nil
}

// CheckIfCompoundExists checks if a compound with the given ID exists in the database.
// If the compound does not exist, it logs a warning and sends a 404 Not Found response.
// Returns nil if the compound exists, or an error if it doesn't or if there's a database error.
func CheckIfCompoundExists(compoundId string, w http.ResponseWriter) error {
	var compoundExists bool
	if err := db.Db.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", compoundId).Scan(&compoundExists); err != nil {
		logrus.Errorf("Error checking if compound exists: %v", err)
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Compound_check_error})
		return err
	}

	if !compoundExists {
		logrus.Warnf("Compound '%s' not found.", compoundId)
		JsonRes(w, http.StatusNotFound, &Resp{Error: Item_not_found})
		return errors.New("compound not found")
	}
	return nil
}

// ParseAndValidateDate parses a date string in "YYYY-MM-DD" format and validates it.
// It combines the input date with the current time (IST) and checks if the resulting time is in the future.
// It also converts the valid date to a Unix timestamp.
// Returns the Unix timestamp and nil error for a valid, non-future date, or 0 and an error otherwise.
func ParseAndValidateDate(date string, w http.ResponseWriter) (int64, error) {
	now := time.Now().Local()
	dateTimeString := fmt.Sprintf("%sT%02d:%02d:%02d+05:30", // Assuming IST
		date, now.Hour(), now.Minute(), now.Second())

	parsedDate, err := time.Parse(time.RFC3339, dateTimeString)
	if err != nil {
		logrus.Warnf("Invalid date format provided: %s, error: %v", date, err)
		JsonRes(w, http.StatusBadRequest, &Resp{Error: Invalid_date_format})
		return 0, err
	}

	if parsedDate.After(time.Now()) {
		logrus.Warnf("Future date provided: %s", date)
		JsonRes(w, http.StatusBadRequest, &Resp{Error: Future_date_error})
		return 0, errors.New("future date provided")
	}

	entryDate, err := UnixTimestamp(date)
	if err != nil {
		logrus.Errorf("Error converting date '%s' to timestamp: %v", date, err)
		JsonRes(w, http.StatusBadRequest, &Resp{Error: Date_conversion_error})
		return 0, err
	}

	logrus.Debugf("Parsed entry date to timestamp: %d", entryDate)
	return entryDate, nil
}

// ValidateReqMethod checks if the HTTP request method matches the expected method.
// If the methods do not match, it logs a warning and sends a 405 Method Not Allowed response.
// Returns nil if the method is valid, or an error otherwise.
func ValidateReqMethod(reqMethod string, expectedMethod string, w http.ResponseWriter) error {
	if reqMethod != expectedMethod {
		logrus.Warnf("Invalid request method provided: %s", reqMethod)
		JsonRes(w, http.StatusMethodNotAllowed, &Resp{Error: InvalidMethod})
		return errors.New("invalid request method")
	}
	return nil
}

// SetupTestDB initializes an in-memory SQLite database for testing purposes.
// It removes any existing "test.db" file, initializes the database connection, and creates the necessary tables using the migrate package.
// Panics if table creation fails.
func SetupTestDB() {
	os.Remove("test.db")
	db.InitDB("test.db")
	if err := migrate.CreateTables(db.Db); err != nil {
		panic("Failed to create tables: " + err.Error())
	}
}

// TeardownTestDB closes the test database connection and removes the "test.db" file.
// It handles potential errors during closing and removal, logging them but not returning them.
func TeardownTestDB() {
	defer func() {
		if db.Db != nil {
			log.Println("Attempting to close test database connection...")
			closeErr := db.Db.Close()
			if closeErr != nil {
				log.Printf("Error closing test database connection: %v", closeErr)
			} else {
				log.Println("Test database connection closed successfully.")
			}
		}

		log.Println("Attempting to remove test database file...")
		removeErr := os.Remove("test.db")
		if removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("Error removing test database file: %v", removeErr)
		} else if removeErr == nil {
			log.Println("Test database file removed successfully.")
		} else {
			log.Println("Test database file did not exist or was already removed.")
		}
	}()
}

// ExecuteRequest creates a new ResponseRecorder and serves the HTTP request using the provided handler function.
// Returns the ResponseRecorder containing the result of the handler execution.
func ExecuteRequest(req *http.Request, handler http.HandlerFunc) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// CheckResponseCode checks if the actual HTTP response code matches the expected code.
// It uses t.Errorf to report a test failure if the codes do not match.
func CheckResponseCode(t *testing.T, expected, actual int) {
	if expected != actual {
		t.Errorf("Expected response code %d, got %d", expected, actual)
	}
}

// CheckResponseBodyContains checks if the actual response body string contains the expected substring.
// It uses t.Errorf to report a test failure if the substring is not found.
func CheckResponseBodyContains(t *testing.T, expectedSubstring string, actualBody string) {
	if !strings.Contains(actualBody, expectedSubstring) {
		t.Errorf("Expected response body to contain '%s', \n but got '%s'", expectedSubstring, actualBody)
	}
}

// CreateRequest creates a new HTTP request with the specified method, URL, and JSON body.
// It sets the "Content-Type" header to "application/json".
func CreateRequest(method, url string, body map[string]any) *http.Request {
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest(method, url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// FormatDate formats a time.Time object into a "YYYY-MM-DD" string.
func FormatDate(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), t.Day())
}