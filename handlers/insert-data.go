package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

const (
	entryIncoming = "incoming"
	entryOutgoing = "outgoing"
	scaleMg       = "mg"
	scaleMl       = "ml"
	maxRetries    = 2
	retryDelay    = 100 * time.Millisecond
)

func InsertData(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, "{\"error\": \"This action requires using the POST method.\"}")
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "The data provided is not in the correct format."}`)
		return
	}

	logrus.Infof("Received request to insert data: %+v", entry)

	// Date validation
	parsedDate, dateFormatErr := time.Parse("02-01-2006", entry.Date)
	if dateFormatErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "{\"error\": \"Please provide the date in the format DD-MM-YYYY.\"}")
		logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, dateFormatErr)
		return
	}

	if parsedDate.After(time.Now()) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "{\"error\": \"The date cannot be in the future.\"}")
		logrus.Warnf("Future date provided: %s", entry.Date)
		return
	}

	entryDate, err := utils.UnixTimestamp(entry.Date)
	logrus.Debugf("Parsed entry date to timestamp: %d", entryDate)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "{\"error\": \"There was an issue with the date provided.\"}")
		logrus.Errorf("Error converting date '%s' to timestamp: %v", entry.Date, err)
		return
	}

	if entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || (entry.Scale != scaleMg && entry.Scale != scaleMl) || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "{\"error\": \"Please make sure all required information is filled in correctly.\"}")
		logrus.Warn("Missing or invalid required fields in the request.")
		return
	}

	compoundID := utils.ToCamelCase(entry.CompoundName)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	var currentStock int
	compoundCheckErr := retry.Do(func() error {
		return db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", compoundID).Scan(&currentStock)
	}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay))

	if compoundCheckErr != nil && !errors.Is(compoundCheckErr, sql.ErrNoRows) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"error\": \"There was a problem retrieving stock information. Please try again later.\"}")
		logrus.Errorf("Error querying current stock for compound '%s': %v", compoundID, compoundCheckErr)
		return
	}
	compoundNotFound := errors.Is(compoundCheckErr, sql.ErrNoRows)

	var dbTx *sql.Tx
	dbErr := retry.Do(func() error {
		var err error
		dbTx, err = db.Db.Begin()
		return err
	}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay))
	if dbErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"error\": \"There was a problem starting a transaction. Please try again later.\"}")
		logrus.Errorf("Error starting database transaction: %v", dbErr)
		return
	}
	defer dbTx.Rollback() // Rollback will be called if commit doesn't happen

	switch entry.Type {
	case entryOutgoing:

		if compoundNotFound {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "{\"error\": \"The item requested could not be found.\"}")
			logrus.Warnf("Compound '%s' not found for outgoing transaction.", compoundID)
			return
		}

		if compoundCheckErr != nil {
			// compoundCheckErr was already handled above, but keeping this for extra safety in case of logic changes
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "{\"error\": \"There was a problem retrieving stock information. Please try again later.\"}")
			logrus.Errorf("Error retrieving stock information for compound '%s': %v", compoundID, compoundCheckErr)
			return
		}

		if currentStock < txnQuantity {
			w.WriteHeader(http.StatusNotAcceptable)
			fmt.Fprint(w, "{\"error\": \"The requested quantity is not available in stock.\"}")
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", compoundID, currentStock, txnQuantity)
			return
		}
		currentStock -= txnQuantity
		logrus.Debugf("Outgoing transaction: Compound '%s', Quantity: %d, Remaining Stock: %d", compoundID, txnQuantity, currentStock)

	case entryIncoming:

		if compoundNotFound {
			insertCompound := `INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`
			if err := retry.Do(func() error {
				_, err := dbTx.Exec(insertCompound, compoundID, entry.CompoundName, entry.Scale)
				return err
			}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay)); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "{\"error\": \"There was a problem adding the new item. Please try again later.\"}")
				logrus.Errorf("Error inserting new compound '%s': %v", compoundID, err)
				return
			}
			logrus.Infof("New compound '%s' added.", compoundID)
		}
		currentStock += txnQuantity
		logrus.Debugf("Incoming transaction: Compound '%s', Quantity: %d, New Stock: %d", compoundID, txnQuantity, currentStock)

	}

	// Generate IDs
	quantityID := fmt.Sprintf("Q_%s_%d", compoundID, time.Now().UnixNano())
	entryID := fmt.Sprintf("%s%s_%d", map[string]string{"incoming": "I", "outgoing": "O"}[entry.Type], compoundID, time.Now().UnixNano())

	// Insert into quantity
	insertQty := `INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)`
	if err := retry.Do(func() error {
		_, err := dbTx.Exec(insertQty, quantityID, entry.NumOfUnits, entry.QuantityPerUnit)
		return err
	}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"error\": \"There was a problem recording the quantity. Please try again later.\"}")
		logrus.Errorf("Error inserting quantity for entry '%s': %v", entryID, err)
		return
	}
	logrus.Debugf("Inserted quantity with ID '%s'", quantityID)

	// Insert into entry
	insertEntry := `
		INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	if err := retry.Do(func() error {
		_, err := dbTx.Exec(insertEntry, entryID, entry.Type, entryDate, compoundID, entry.Remark, entry.VoucherNo, quantityID, currentStock)
		return err
	}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"error\": \"There was a problem saving the entry. Please try again later.\"}")
		logrus.Errorf("Error inserting entry with ID '%s': %v", entryID, err)
		return
	}
	logrus.Infof("Entry inserted successfully with ID '%s'", entryID)

	if err := retry.Do(func() error {
		return dbTx.Commit()
	}, retry.Attempts(maxRetries+1), retry.Delay(retryDelay)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "{\"error\": \"A problem occurred while saving your changes. Please try again later.\"}")
		logrus.Errorf("Error committing database transaction: %v", err)
		return
	}
	logrus.Debug("Database transaction committed successfully.")

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"message": "Entry inserted successfully", "entry_id": "%s"}`, entryID)
}
