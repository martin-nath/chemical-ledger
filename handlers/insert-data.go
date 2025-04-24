package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// InsertData handles the insertion of chemical ledger data.
func InsertData(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodPost, w); err != nil {
		return
	}

	entry := &utils.Entry{}
	if err := utils.JsonReq(r, entry, w); err != nil {
		return
	}
	logrus.Infof("Received request to insert data: %+v", entry)

	if err := validateEntryFields(entry, w); err != nil {
		return
	}

	wg := sync.WaitGroup{}
	entryDateCh := make(chan int64, 1)
	errCh := make(chan error, 3)
	wg.Add(2)

	// Check if the compound exists.
	go func(entry *utils.Entry, w http.ResponseWriter) {
		defer wg.Done()
		if err := utils.CheckIfCompoundExists(entry.CompoundId, w); err != nil {
			errCh <- err
			return
		}
	}(entry, w)

	// Parse and validate the date.
	go func(entry *utils.Entry, w http.ResponseWriter) {
		defer wg.Done()
		var entryDate int64
		var err error
		if entryDate, err = utils.ParseAndValidateDate(entry.Date, w); err != nil {
			errCh <- err
			return
		}
		entryDateCh <- entryDate
	}(entry, w)

	// Calculate the quantity of the stock transaction.
	currentStock, err := validateAndCalcCurrTxQuantity(entry, w)
	if err != nil {
		errCh <- err
		return
	}

	wg.Wait()
	close(errCh)
	close(entryDateCh)
	for err := range errCh {
		if err != nil {
			return
		}
	}

	entryDate := <-entryDateCh

	dbTx, err := utils.BeginDbTx(w)
	if err != nil {
		return
	}

	defer func() {
		if rbe := dbTx.Rollback(); rbe != nil && rbe != sql.ErrTxDone {
			logrus.Errorf("Error during deferred transaction rollback: %v", rbe)
		} else if rbe == nil {
			logrus.Debug("Deferred transaction rollback executed.")
		}
	}()

	_, entryID, err := insertEntryData(dbTx, entry, entryDate, currentStock, w)
	if err != nil {
		return
	}

	if err := utils.UpdateSubSequentNetStock(dbTx, entryDate, entry.CompoundId, w); err != nil {
		return
	}

	if err := utils.CommitDbTx(dbTx, w); err != nil {
		return
	}

	utils.JsonRes(w, http.StatusCreated, &utils.Resp{
		Message: utils.Entry_inserted_successfully,
		Data:    map[string]any{"entry_id": entryID},
	})
}

// validateEntryFields validates the required fields for inserting an entry.
func validateEntryFields(entry *utils.Entry, w http.ResponseWriter) error {
	if entry.CompoundId == "" || entry.QuantityPerUnit <= 0 || entry.NumOfUnits <= 0 || (entry.Type != utils.TypeIncoming && entry.Type != utils.TypeOutgoing) {
		logrus.Warn("Missing or invalid required fields in the request.")
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("missing or invalid required fields")
	}
	return nil
}

// validateAndCalcCurrTxQuantity validates stock levels for outgoing transactions and calculates the new stock.
func validateAndCalcCurrTxQuantity(entry *utils.Entry, w http.ResponseWriter) (int, error) {
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit
	currentStock := 0

	if entry.Type == utils.TypeOutgoing {
		err := db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", entry.CompoundId).Scan(&currentStock)
		if err != nil && err != sql.ErrNoRows {
			logrus.Errorf("Error retrieving current stock for compound '%s': %v", entry.CompoundId, err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			return 0, err
		}
		if currentStock < txnQuantity {
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", entry.CompoundId, currentStock, txnQuantity)
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{Error: utils.Insufficient_stock})
			return 0, errors.New("insufficient stock")
		}
		currentStock -= txnQuantity
		logrus.Debugf("Outgoing transaction: Compound '%s', Quantity: %d, Remaining Stock: %d", entry.CompoundId, txnQuantity, currentStock)
	}

	if entry.Type == utils.TypeIncoming {
		err := db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", entry.CompoundId).Scan(&currentStock)
		if err != nil && err != sql.ErrNoRows {
			logrus.Errorf("Error retrieving current stock for compound '%s': %v", entry.CompoundId, err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			return 0, err
		}
		currentStock += txnQuantity
		logrus.Debugf("Incoming transaction: Compound '%s', Quantity: %d, New Stock: %d", entry.CompoundId, txnQuantity, currentStock)
	}
	return currentStock, nil
}

// insertEntryData inserts the entry and quantity data into the database.
func insertEntryData(dbTx *sql.Tx, entry *utils.Entry, entryDate int64, currentStock int, w http.ResponseWriter) (string, string, error) {
	quantityID := fmt.Sprintf("Q_%s", uuid.NewString())
	entryID := fmt.Sprintf("E_%s", uuid.NewString())

	err := utils.Retry(func() error {
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
	})

	if err != nil {
		logrus.Errorf("Error during batch insert for entry '%s' and quantity '%s': %v", entryID, quantityID, err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Save_entry_details_error})
		return "", "", err
	}

	logrus.Debugf("Inserted quantity with ID '%s'", quantityID)
	logrus.Infof("Entry inserted successfully with ID '%s'", entryID)
	return quantityID, entryID, nil
}
