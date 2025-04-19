package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"sync"

	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

func UpdateData(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodPut, w); err != nil {
		return
	}

	var entry *utils.Entry
	if err := utils.JsonReq(r, &entry, w); err != nil {
		return
	}
	logrus.Infof("Received request to update data: %+v", entry)

	if err := validateUpdateEntryFields(entry, w); err != nil {
		return
	}

	dbTx, err := utils.BeginDbTx(w)
	if err != nil {
		return
	}

	var originalEntryDate int64
	var originalCompoundId string
	var originalQuantity utils.Quantity

	if err := retrieveOriginalEntryData(dbTx, entry.ID, &originalEntryDate, &originalCompoundId, &originalQuantity, w); err != nil {
		return
	}

	var updatedEntryDate int64
	if entry.Date != "" {
		updatedEntryDate, err = utils.ParseAndValidateDate(entry.Date, w)
		if err != nil {
			return
		}
	} else {
		updatedEntryDate = originalEntryDate
	}

	if entry.CompoundId != "" {
		if err := utils.CheckIfCompoundExists(entry.CompoundId, w); err != nil {
			return
		}
	}

	var currentStock int
	var quantityID string

	if entry.NumOfUnits != 0 || entry.QuantityPerUnit != 0 || entry.Type != "" {
		currentStock, quantityID, err = validateAndUpdateQuantity(dbTx, entry, originalQuantity, w)
		if err != nil {
			return
		}
	}

	if err := updateEntryDetails(dbTx, entry, updatedEntryDate, currentStock, quantityID, w); err != nil {
		return
	}

	var wg sync.WaitGroup
	if entry.CompoundId != "" && entry.CompoundId != originalCompoundId {
		wg.Add(1)
		go func(dbTx *sql.Tx, date int64, compoundId string, respWriter http.ResponseWriter) {
			defer wg.Done()
			utils.UpdateSubSequentNetStock(dbTx, date, compoundId, respWriter)
		}(dbTx, updatedEntryDate, entry.CompoundId, w)
	}

	utils.UpdateSubSequentNetStock(dbTx, updatedEntryDate, originalCompoundId, w)
	wg.Wait()

	if err := utils.CommitDbTx(dbTx, w); err != nil {
		return
	}

	utils.JsonRes(w, http.StatusOK, &utils.Resp{
		Message: utils.Entry_updated_successfully,
		Data:    map[string]any{"entry_id": entry.ID},
	})
}

// Validates the fields required for updating an entry. If any errors occur, it will return an error and write the error message to the response writer.
func validateUpdateEntryFields(entry *utils.Entry, w http.ResponseWriter) error {
	if entry.ID == "" && entry.CompoundId == "" && entry.Date == "" && entry.Type == "" && entry.Remark == "" && entry.VoucherNo == "" && entry.QuantityPerUnit == 0 && entry.NumOfUnits == 0 {
		logrus.Warn("No fields provided for update.")
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("no fields provided for update")
	}
	if entry.Type != "" && entry.Type != utils.TypeIncoming && entry.Type != utils.TypeOutgoing {
		logrus.Warnf("Invalid entry type provided: %s", entry.Type)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("invalid entry type")
	}
	if entry.QuantityPerUnit < 0 || entry.NumOfUnits < 0 {
		logrus.Warn("Quantity per unit and number of units cannot be negative.")
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("invalid quantity values")
	}
	return nil
}

// Retrieves the original entry data from the database. If any errors occur, it will return an error and write the error message to the response writer.
func retrieveOriginalEntryData(dbTx *sql.Tx, entryID string, originalEntryDate *int64, originalCompoundId *string, originalQuantity *utils.Quantity, w http.ResponseWriter) error {
	err := dbTx.QueryRow(`
		SELECT e.date, e.compound_id, q.num_of_units, q.quantity_per_unit
		FROM entry e
		JOIN quantity q ON e.quantity_id = q.id
		WHERE e.id = ?
	`, entryID).Scan(originalEntryDate, originalCompoundId, &originalQuantity.NumOfUnits, &originalQuantity.QuantityPerUnit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logrus.Warnf("Entry with ID '%s' not found.", entryID)
			utils.JsonRes(w, http.StatusNotFound, &utils.Resp{Error: utils.Item_not_found})
			return errors.New("entry not found")
		}
		logrus.Errorf("Error retrieving original entry data for ID '%s': %v", entryID, err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return errors.New("error retrieving entry data")
	}
	return nil
}

// Validates the updated quantity and updates the quantity table if necessary.
func validateAndUpdateQuantity(dbTx *sql.Tx, updatedEntry *utils.Entry, originalQuantity utils.Quantity, w http.ResponseWriter) (int, string, error) {
	txnQuantity := updatedEntry.NumOfUnits * updatedEntry.QuantityPerUnit

	entryType, err := getEntryType(dbTx, updatedEntry)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		logrus.Errorf("Error retrieving entry type for ID '%s': %v", updatedEntry.ID, err)
		return 0, "", errors.New("error retrieving entry type")
	}

	previousStock, err := getCurrentStock(dbTx, updatedEntry.CompoundId)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		logrus.Errorf("Error retrieving current stock for compound '%s': %v", updatedEntry.CompoundId, err)
		return 0, "", errors.New("error retrieving current stock")
	}

	currentStock, err := calculateCurrentStock(entryType, previousStock, txnQuantity, updatedEntry.CompoundId, w)
	if err != nil {
		return 0, "", err
	}

	quantityID, err := updateQuantityIfChanged(dbTx, updatedEntry, originalQuantity)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Save_entry_details_error})
		logrus.Errorf("Error updating quantity for entry '%s': %v", updatedEntry.ID, err)
		return 0, "", errors.New("error updating quantity")
	}

	if quantityID == "" {
		quantityID, err = getQuantityID(dbTx, updatedEntry.ID)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			logrus.Errorf("Error retrieving quantity ID for entry '%s': %v", updatedEntry.ID, err)
			return 0, "", errors.New("error retrieving quantity ID")
		}
	}

	return currentStock, quantityID, nil
}

func getEntryType(dbTx *sql.Tx, entry *utils.Entry) (string, error) {
	if entry.Type != "" {
		return entry.Type, nil
	}
	var entryType string
	err := dbTx.QueryRow("SELECT type FROM entry WHERE id = ?", entry.ID).Scan(&entryType)
	return entryType, err
}

func getCurrentStock(dbTx *sql.Tx, compoundId string) (int, error) {
	var previousStock int
	err := dbTx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", compoundId).Scan(&previousStock)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil // No previous stock, treat as 0
	}
	return previousStock, err
}

func calculateCurrentStock(entryType string, previousStock int, txnQuantity int, compoundId string, w http.ResponseWriter) (int, error) {
	switch entryType {
	case utils.TypeOutgoing:
		if previousStock < txnQuantity {
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", compoundId, previousStock, txnQuantity)
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{Error: utils.Insufficient_stock})
			return 0, errors.New("insufficient stock")
		}
		return previousStock - txnQuantity, nil
	case utils.TypeIncoming:
		return previousStock + txnQuantity, nil
	default:
		return previousStock, nil // No change if type is not incoming or outgoing
	}
}

func updateQuantityIfChanged(dbTx *sql.Tx, updatedEntry *utils.Entry, originalQuantity utils.Quantity) (string, error) {
	var quantityID string
	updated := false

	if updatedEntry.NumOfUnits != 0 && updatedEntry.NumOfUnits != originalQuantity.NumOfUnits {
		err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
		if err != nil {
			return "", err
		}
		_, err = dbTx.Exec(
			"UPDATE quantity SET num_of_units = ? WHERE id = ?",
			updatedEntry.NumOfUnits, quantityID,
		)
		if err != nil {
			return "", err
		}
		logrus.Debugf("Updated num_of_units for quantity ID '%s'", quantityID)
		updated = true
	}

	if updatedEntry.QuantityPerUnit != 0 && updatedEntry.QuantityPerUnit != originalQuantity.QuantityPerUnit {
		if !updated {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				return "", err
			}
		}
		_, err := dbTx.Exec(
			"UPDATE quantity SET quantity_per_unit = ? WHERE id = ?",
			updatedEntry.QuantityPerUnit, quantityID,
		)
		if err != nil {
			return "", err
		}
		logrus.Debugf("Updated quantity_per_unit for quantity ID '%s'", quantityID)
		updated = true
	}

	if updated {
		// Retrieve the quantity ID if it was updated
		if quantityID == "" {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				return "", err
			}
		}
		return quantityID, nil
	}

	return "", nil
}

func getQuantityID(dbTx *sql.Tx, entryID string) (string, error) {
	var quantityID string
	err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", entryID).Scan(&quantityID)
	return quantityID, err
}

// Updates the entry details in the database using CASE statements.
func updateEntryDetails(dbTx *sql.Tx, entry *utils.Entry, entryDate int64, currentStock int, quantityID string, w http.ResponseWriter) error {
	query := `
		UPDATE entry
		SET
			type = CASE WHEN ? != '' THEN ? ELSE type END,
			date = CASE WHEN ? != 0 THEN ? ELSE date END,
			compound_id = CASE WHEN ? != '' THEN ? ELSE compound_id END,
			remark = CASE WHEN ? != '' THEN ? ELSE remark END,
			voucher_no = CASE WHEN ? != '' THEN ? ELSE voucher_no END,
			quantity_id = CASE WHEN ? != '' THEN ? ELSE quantity_id END,
			net_stock = CASE WHEN ? != 0 THEN ? ELSE net_stock END
		WHERE id = ?
	`
	_, err := dbTx.Exec(
		query,
		entry.Type, entry.Type,
		entryDate, entryDate,
		entry.CompoundId, entry.CompoundId,
		entry.Remark, entry.Remark,
		entry.VoucherNo, entry.VoucherNo,
		quantityID, quantityID,
		currentStock, currentStock,
		entry.ID,
	)

	if err != nil {
		logrus.Errorf("Error updating entry with ID '%s': %v", entry.ID, err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Save_entry_details_error})
		return errors.New("error updating entry details")
	}
	logrus.Debugf("Updated entry with ID '%s'", entry.ID)
	return nil
}
