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
)

const (
	Id_invalidMethod                        = "This action requires using the POST method."
	Id_invalid_date_format                  = "Please provide the date in the format DD-MM-YYYY."
	Id_req_body_decode_error                = "The data provided is not in the correct format."
	Id_missingFields_or_inappropriate_value = "Please make sure all required information is filled in correctly."
	Id_future_date_error                    = "The date cannot be in the future."
	Id_date_conversion_error                = "There was an issue with the date provided."
	Id_item_not_found                       = "The item requested could not be found."
	Id_internal_server_error                = "Internal Server Error. Please try again later."
	Id_stock_retrieval_error                = "There was a problem retrieving stock information. Please try again later."
	Id_insufficient_stock                   = "The requested quantity is not available in stock."
	Id_add_new_item_error                   = "There was a problem adding the new item. Please try again later."
	Id_save_entry_details_error             = "There was a problem saving the entry details. Please try again later."
	Id_save_changes_error                   = "A problem occurred while saving your changes. Please try again later."
	Id_entry_inserted_successfully          = "Entry inserted successfully"
)

func InsertData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.JsonRes(w, http.StatusMethodNotAllowed, &utils.Resp{
			Error: Id_invalidMethod,
		})
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_req_body_decode_error,
		})
		return
	}

	logrus.Infof("Received request to insert data: %+v", entry)

	// Validate entry fields early
	if entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || (entry.Scale != scaleMg && entry.Scale != scaleMl) || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_missingFields_or_inappropriate_value,
		})
		logrus.Warn("Missing or invalid required fields in the request.")
		return
	}

	// Date parsing and validation
	parsedDate, err := time.Parse("02-01-2006", entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_invalid_date_format,
		})
		logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
		return
	}

	if parsedDate.After(time.Now()) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_future_date_error,
		})
		logrus.Warnf("Future date provided: %s", entry.Date)
		return
	}

	entryDate, err := utils.UnixTimestamp(entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_date_conversion_error,
		})
		logrus.Errorf("Error converting date '%s' to timestamp: %v", entry.Date, err)
		return
	}
	logrus.Debugf("Parsed entry date to timestamp: %d", entryDate)

	compoundID := utils.ToCamelCase(entry.CompoundName)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	var currentStock int
	compoundCheckErr := retry.Do(func() error {
		return db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", compoundID).Scan(&currentStock)
	}, retry.Attempts(utils.MaxRetries), retry.Delay(utils.RetryDelay))
	compoundNotFound := errors.Is(compoundCheckErr, sql.ErrNoRows)

	// Handle outgoing transaction before starting the database transaction if compound is not found
	if entry.Type == entryOutgoing && compoundNotFound {
		utils.JsonRes(w, http.StatusNotFound, &utils.Resp{
			Error: Id_item_not_found,
		})
		logrus.Warnf("Compound '%s' not found for outgoing transaction.", compoundID)
		return
	}

	var dbTx *sql.Tx
	dbErr := retry.Do(func() error {
		var err error
		dbTx, err = db.Db.Begin()
		return err
	}, retry.Attempts(utils.MaxRetries), retry.Delay(utils.RetryDelay))
	if dbErr != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: Id_internal_server_error,
		})
		logrus.Errorf("Error starting database transaction: %v", dbErr)
		return
	}
	defer dbTx.Rollback() // Rollback will be called if commit doesn't happen

	if entry.Type == entryOutgoing {
		if compoundCheckErr != nil && !errors.Is(compoundCheckErr, sql.ErrNoRows) {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
				Error: Id_stock_retrieval_error,
			})
			logrus.Errorf("Error retrieving current stock for compound '%s': %v", compoundID, compoundCheckErr)
			return
		}

		if currentStock < txnQuantity {
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{
				Error: Id_insufficient_stock,
			})
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", compoundID, currentStock, txnQuantity)
			return
		}
		currentStock -= txnQuantity
		logrus.Debugf("Outgoing transaction: Compound '%s', Quantity: %d, Remaining Stock: %d", compoundID, txnQuantity, currentStock)
	} else {
		if compoundNotFound {
			insertCompound := `INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`
			_, err = dbTx.Exec(insertCompound, compoundID, entry.CompoundName, entry.Scale)
			if err != nil {
				utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
					Error: Id_add_new_item_error,
				})
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

	err = retry.Do(func() error {
		_, err := dbTx.Exec(`
			INSERT INTO quantity (id, num_of_units, quantity_per_unit)
			VALUES (?, ?, ?);

			INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?);
		`,
			quantityID, entry.NumOfUnits, entry.QuantityPerUnit,
			entryID, entry.Type, entryDate, compoundID, entry.Remark, entry.VoucherNo, quantityID, currentStock,
		)
		return err
	}, retry.Attempts(utils.MaxRetries), retry.Delay(utils.RetryDelay))

	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: Id_save_entry_details_error,
		})
		logrus.Errorf("Error during batch insert for entry '%s' and quantity '%s': %v", entryID, quantityID, err)
		return
	}
	logrus.Debugf("Inserted quantity with ID '%s'", quantityID)
	logrus.Infof("Entry inserted successfully with ID '%s'", entryID)

	if err := dbTx.Commit(); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: Id_save_changes_error,
		})
		logrus.Errorf("Error committing database transaction: %v", err)
		return
	}
	logrus.Debug("Database transaction committed successfully.")

	utils.JsonRes(w, http.StatusCreated, &utils.Resp{
		Message: Id_entry_inserted_successfully,
		Data:    map[string]any{"entry_id": entryID},
	})
}
