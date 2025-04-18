package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/uuid"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

const (
	entryIncoming = "incoming"
	entryOutgoing = "outgoing"
)

const (
	Id_invalidMethod                        = "This action only works when you send the information in a specific way. Please try again using the correct method."
	Id_req_body_decode_error                = "Sorry, but we couldn't understand the information you sent. Could you please double-check it and try again?"
	Id_missingFields_or_inappropriate_value = "Please make sure you've filled in all the necessary details and that they are correct."
	Id_invalid_date_format                  = "The date needs to be in this format: day-month-year (like 01-05-2025)."
	Id_future_date_error                    = "The date you entered can't be in the future. Please enter a valid date."
	Id_date_conversion_error                = "We couldn't figure out the date you gave us. Could you check it and try again?"
	Id_compound_check_error                 = "Something went wrong with checking the compound right now. Please try again in a little while."
	Id_item_not_found                       = "We couldn't find the compound you were looking for."
	Id_stock_retrieval_error                = "Sorry, we're having trouble getting the stock information right now. Please try again later."
	Id_insufficient_stock                   = "We don't have enough of that item in stock to fulfill your request."
	Id_add_new_item_error                   = "There was a problem recording the quantity. Please try again."
	Id_save_entry_details_error             = "We couldn't save the details you entered. Please try again."
	Id_update_subsequent_entries_error      = "We're having trouble updating the stock information. Please try again."
	Id_record_transaction_error             = "We couldn't start saving this entry right now. Please try again later."
	Id_commit_transaction_error             = "We couldn't finish saving this entry. Please try again later."
	Id_entry_update_scan_error              = "Something went wrong while reading the updated stock information. Please try again later."
	Id_entry_inserted_successfully          = "Great! Your entry has been saved."
	Id_internal_server_error                = "Oops! Something went wrong on our end. Please try again later."
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
	if entry.CompoundId == "" || entry.QuantityPerUnit <= 0 || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
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

	var compoundExists bool
	if err := db.Db.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", entry.CompoundId).Scan(&compoundExists); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: Id_compound_check_error,
		})
		logrus.Errorf("Error checking if compound exists: %v", err)
		return
	}

	if !compoundExists {
		utils.JsonRes(w, http.StatusNotFound, &utils.Resp{
			Error: Id_item_not_found,
		})
		logrus.Warnf("Compound '%s' not found.", entry.CompoundId)
		return
	}

	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

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

	defer dbTx.Rollback()

	currentStock := 0

	if entry.Type == entryOutgoing {
		if currentStock < txnQuantity {
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{
				Error: Id_insufficient_stock,
			})
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", entry.CompoundId, currentStock, txnQuantity)
			return
		}
		currentStock -= txnQuantity
		logrus.Debugf("Outgoing transaction: Compound '%s', Quantity: %d, Remaining Stock: %d", entry.CompoundId, txnQuantity, currentStock)
	}

	if entry.Type == entryIncoming {
		currentStock += txnQuantity
		logrus.Debugf("Incoming transaction: Compound '%s', Quantity: %d, New Stock: %d", entry.CompoundId, txnQuantity, currentStock)
	}

	// Generate IDs
	quantityID := fmt.Sprintf("Q_%s", uuid.NewString())
	entryID := fmt.Sprintf("E_%s", uuid.NewString())

	err = retry.Do(func() error {
		_, err := dbTx.Exec(`
			INSERT INTO quantity (id, num_of_units, quantity_per_unit)
			VALUES (?, ?, ?);

			INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?);
		`,
			quantityID, entry.NumOfUnits, entry.QuantityPerUnit,
			entryID, entry.Type, entryDate, entry.CompoundId, entry.Remark, entry.VoucherNo, quantityID, currentStock,
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

	utils.UpdateSubSequentNetStock(dbTx, entryDate, entry.CompoundId, w)

	if err := dbTx.Commit(); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: Id_commit_transaction_error,
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
