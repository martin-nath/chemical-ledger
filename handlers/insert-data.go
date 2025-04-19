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
	scaleMg = "mg"
	scaleMl = "ml"
)

func InsertData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.JsonRes(w, http.StatusMethodNotAllowed, &utils.Resp{
			Error: utils.InvalidMethod,
		})
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.Req_body_decode_error,
		})
		return
	}
	logrus.Infof("Received request to insert data: %+v", entry)

	if entry.CompoundId == "" || entry.QuantityPerUnit <= 0 || entry.NumOfUnits <= 0 || (entry.Scale != scaleMg && entry.Scale != scaleMl) ||(entry.Type != entryIncoming && entry.Type != entryOutgoing) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.MissingFields_or_inappropriate_value,
		})
		logrus.Warn("Missing or invalid required fields in the request.")
		return
	}

	// Date parsing and validation
	parsedDate, err := time.Parse("02-01-2006", entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.Invalid_date_format,
		})
		logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
		return
	}

	if parsedDate.After(time.Now()) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.Future_date_error,
		})
		logrus.Warnf("Future date provided: %s", entry.Date)
		return
	}

	entryDate, err := utils.UnixTimestamp(entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.Date_conversion_error,
		})
		logrus.Errorf("Error converting date '%s' to timestamp: %v", entry.Date, err)
		return
	}

	logrus.Debugf("Parsed entry date to timestamp: %d", entryDate)

	var compoundExists bool
	if err := db.Db.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", entry.CompoundId).Scan(&compoundExists); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: utils.Compound_check_error,
		})
		logrus.Errorf("Error checking if compound exists: %v", err)
		return
	}

	if !compoundExists {
		utils.JsonRes(w, http.StatusNotFound, &utils.Resp{
			Error: utils.Item_not_found,
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
			Error: utils.Internal_server_error,
		})
		logrus.Errorf("Error starting database transaction: %v", dbErr)
		return
	}

	defer dbTx.Rollback()

	currentStock := 0

	if entry.Type == entryOutgoing {
		if currentStock < txnQuantity {
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{
				Error: utils.Insufficient_stock,
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
			Error: utils.Save_entry_details_error,
		})
		logrus.Errorf("Error during batch insert for entry '%s' and quantity '%s': %v", entryID, quantityID, err)
		return
	}

	logrus.Debugf("Inserted quantity with ID '%s'", quantityID)
	logrus.Infof("Entry inserted successfully with ID '%s'", entryID)

	utils.UpdateSubSequentNetStock(dbTx, entryDate, entry.CompoundId, w)

	if err := dbTx.Commit(); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: utils.Commit_transaction_error,
		})
		logrus.Errorf("Error committing database transaction: %v", err)
		return
	}

	logrus.Debug("Database transaction committed successfully.")

	utils.JsonRes(w, http.StatusCreated, &utils.Resp{
		Message: utils.Entry_inserted_successfully,
		Data:    map[string]any{"entry_id": entryID},
	})
}
