package utils

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/sirupsen/logrus"
)

func UnixTimestamp(dateStr string) (int64, error) {
	date, err := time.Parse("02-01-2006", dateStr)
	if err != nil {
		return 0, err
	}

	now := time.Now()

	combined := time.Date(date.Year(), date.Month(), date.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, time.Local)

	return combined.Unix(), nil
}

// Helper function to send a JSON response to the client.
func JsonRes(w http.ResponseWriter, status int, resObj *Resp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resObj)
}

// Helper function to decode a JSON request body. If any errors occur, it will return an error and write the error message to the response writer.
func JsonReq(r *http.Request, dst interface{}, w http.ResponseWriter) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		logrus.Errorf("Error decoding JSON request body: %v", err)
		JsonRes(w, http.StatusBadRequest, &Resp{Error: Req_body_decode_error})
		return err
	}
	return nil
}

// Updates the net stock of subsequent entries for a given compound from the given date till today.
func UpdateSubSequentNetStock(dbTx *sql.Tx, entryDate int64, compoundId string, w http.ResponseWriter) error {
	var previousStock int
	err := dbTx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? AND date < ? ORDER BY date DESC LIMIT 1", compoundId, entryDate).Scan(&previousStock)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Stock_retrieval_error})
		logrus.Errorf("Error retrieving previous stock for compound '%s': %v", compoundId, err)
		return errors.New("error retrieving previous stock")
	}

	rows, err := dbTx.Query(`
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
	if err != nil {
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Stock_retrieval_error})
		logrus.Errorf("Error retrieving subsequent entries for compound '%s': %v", compoundId, err)
		return errors.New("error retrieving subsequent entries")
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
			JsonRes(w, http.StatusInternalServerError, &Resp{Error: Entry_update_scan_error})
			logrus.Errorf("Error reading entry details while updating stock: %v", err)
			return errors.New("error reading entry details")
		}

		switch entryUpdate.EntryType {
		case TypeIncoming:
			previousStock += entryUpdate.Quantity
		case TypeOutgoing:
			previousStock -= entryUpdate.Quantity
			if previousStock < 0 {
				JsonRes(w, http.StatusInternalServerError, &Resp{Error: Insufficient_stock})
				logrus.Errorf("Error calculating net stock after entry '%s': insufficient stock", entryUpdate.EntryID)
				return errors.New("insufficient stock")
			}
		}

		updateQueriesBuilder.WriteString(fmt.Sprintf("UPDATE entry SET net_stock = %d WHERE id = '%s';\n", previousStock, entryUpdate.EntryID))
	}

	updateQueries := updateQueriesBuilder.String()
	if updateQueries != "" {
		_, err = dbTx.Exec(updateQueries)
		if err != nil {
			JsonRes(w, http.StatusInternalServerError, &Resp{Error: Update_subsequent_entries_error})
			logrus.Errorf("Error saving the updated stock information for compound '%s': %v", compoundId, err)
			return errors.New("error saving the updated stock information")
		}
		logrus.Debugf("Updated net stock for subsequent entries of compound '%s'", compoundId)
	}

	return nil
}


// Begins a database transaction. If any errors occur, it will return an error and write the error message to the response writer.
func BeginDbTx(w http.ResponseWriter) (*sql.Tx, error) {
	var dbTx *sql.Tx
	err := retry.Do(func() error {
		var err error
		dbTx, err = db.Db.Begin()
		return err
	}, retry.Attempts(MaxRetries), retry.Delay(RetryDelay))

	if err != nil {
		logrus.Errorf("Error starting database transaction: %v", err)
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Record_transaction_error})
		return nil, err
	}
	return dbTx, nil
}

// Helper function to commit a database transaction. If any errors occur, it will return an error and write the error message to the response writer.
func CommitDbTx(dbTx *sql.Tx, w http.ResponseWriter) error {
	if err := dbTx.Commit(); err != nil {
		logrus.Errorf("Error committing database transaction: %v", err)
		JsonRes(w, http.StatusInternalServerError, &Resp{Error: Commit_transaction_error})
		return err
	}
	logrus.Debug("Database transaction committed successfully.")
	return nil
}

// Checks if the compound exists. If any errors occur, it will return an error and write the error message to the response writer.
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

// Parses and validates the date of the entry. If any errors occur, it will return an error and write the error message to the response writer.
func ParseAndValidateDate(date string, w http.ResponseWriter) (int64, error) {
	parsedDate, err := time.Parse("02-01-2006", date)
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

// Validates the request method. If any errors occur, it will return an error and write the error message to the response writer.
func ValidateReqMethod(reqMethod string, expectedMethod string, w http.ResponseWriter) error {
	if reqMethod != expectedMethod {
		logrus.Warnf("Invalid request method provided: %s", reqMethod)
		JsonRes(w, http.StatusMethodNotAllowed, &Resp{Error: InvalidMethod})
		return errors.New("invalid request method")
	}
	return nil
}
