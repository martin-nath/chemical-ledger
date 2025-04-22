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

	defer func() {
		if rbe := dbTx.Rollback(); rbe != nil && rbe != sql.ErrTxDone {
			logrus.Errorf("Error during deferred transaction rollback: %v", rbe)
		} else if rbe == nil {
			logrus.Debug("Deferred transaction rollback executed.")
		}
	}()

	var originalEntryDate int64
	var originalCompoundId string
	var originalQuantity utils.Quantity
	var originalEntryType string

	if err := retrieveOriginalEntryData(dbTx, entry.ID, &originalEntryDate, &originalCompoundId, &originalQuantity, &originalEntryType, w); err != nil {
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

	if entry.CompoundId != "" && entry.CompoundId != originalCompoundId && originalEntryType == utils.TypeOutgoing {
		originalTxnQuantity := originalQuantity.NumOfUnits * originalQuantity.QuantityPerUnit

		stockBeforeNewEntryDate, err := getStockBeforeDate(dbTx, entry.CompoundId, updatedEntryDate)
		if err != nil {
			logrus.Errorf("Error checking stock before compound ID change for new compound '%s': %v", entry.CompoundId, err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			return
		}

		if stockBeforeNewEntryDate < originalTxnQuantity {
			logrus.Warnf("Insufficient stock in target compound '%s' (%d) for original outgoing transaction quantity (%d) when changing compound ID for entry '%s'",
				entry.CompoundId, stockBeforeNewEntryDate, originalTxnQuantity, entry.ID)
			utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{Error: utils.Insufficient_stock})
			// Transaction will be implicitly rolled back
			return
		}
		logrus.Debugf("Stock check passed for compound ID change. New compound '%s' stock before date %d: %d, original outgoing quantity: %d",
			entry.CompoundId, updatedEntryDate, stockBeforeNewEntryDate, originalTxnQuantity)
	}

	var currentStock int
	var quantityID string

	quantityRelatedFieldsProvided := entry.NumOfUnits != 0 || entry.QuantityPerUnit != 0 || entry.Type != ""

	if quantityRelatedFieldsProvided {
		currentStock, quantityID, err = validateAndUpdateQuantity(dbTx, entry, originalQuantity, w)
		if err != nil {
			// Transaction will be implicitly rolled back
			return
		}
	} else {
		quantityID, err = getQuantityID(dbTx, entry.ID)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
			logrus.Errorf("Error retrieving quantity ID for entry '%s': %v", entry.ID, err)
			return
		}
		currentStock = 0
	}

	if err := updateEntryDetails(dbTx, entry, updatedEntryDate, currentStock, quantityID, w); err != nil {

		return
	}

	if entry.CompoundId != "" && entry.CompoundId != originalCompoundId {
		logrus.Debugf("Compound changed from %s to %s. Updating original compound's chain from original date %d.", originalCompoundId, entry.CompoundId, originalEntryDate)
		utils.UpdateSubSequentNetStock(dbTx, originalEntryDate, originalCompoundId, w)
	}

	targetCompoundIdForStockUpdate := originalCompoundId
	targetDateForStockUpdate := originalEntryDate

	if entry.CompoundId != "" && entry.CompoundId != originalCompoundId {
		targetCompoundIdForStockUpdate = entry.CompoundId
		targetDateForStockUpdate = updatedEntryDate
	} else if entry.Date != "" {
		targetDateForStockUpdate = updatedEntryDate
	}

	logrus.Debugf("Updating subsequent stock for compound %s from effective date %d", targetCompoundIdForStockUpdate, targetDateForStockUpdate)
	utils.UpdateSubSequentNetStock(dbTx, targetDateForStockUpdate, targetCompoundIdForStockUpdate, w)

	var wg sync.WaitGroup // Keep if utils.UpdateSubSequentNetStock uses goroutines internally

	wg.Wait()

	err = utils.CommitDbTx(dbTx, w)
	if err != nil {
		logrus.Errorf("Error committing transaction: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Save_entry_details_error})
		return
	}

	utils.JsonRes(w, http.StatusOK, &utils.Resp{
		Message: utils.Entry_updated_successfully,
		Data:    map[string]any{"id": entry.ID},
	})
}

func validateUpdateEntryFields(entry *utils.Entry, w http.ResponseWriter) error {

	if entry.ID == "" {
		logrus.Warn("No entry ID provided for update.")
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("no entry ID provided for update")
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

func retrieveOriginalEntryData(dbTx *sql.Tx, entryID string, originalEntryDate *int64, originalCompoundId *string, originalQuantity *utils.Quantity, originalEntryType *string, w http.ResponseWriter) error {
	var tempQuantityPerUnit float64
	err := dbTx.QueryRow(`
		SELECT e.type, e.date, e.compound_id, q.num_of_units, q.quantity_per_unit
		FROM entry e
		JOIN quantity q ON e.quantity_id = q.id
		WHERE e.id = ?
	`, entryID).Scan(originalEntryType, originalEntryDate, originalCompoundId, &originalQuantity.NumOfUnits, &tempQuantityPerUnit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logrus.Warnf("Entry with ID '%s' not found.", entryID)
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.Item_not_found})
			return errors.New("entry not found")
		}
		logrus.Errorf("Error retrieving original entry data for ID '%s': %v", entryID, err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		return errors.New("error retrieving entry data")
	}

	if tempQuantityPerUnit != float64(int(tempQuantityPerUnit)) {
		logrus.Warnf("Truncating quantity_per_unit float value %f to int %d for entry '%s'", tempQuantityPerUnit, int(tempQuantityPerUnit), entryID)
	}
	originalQuantity.QuantityPerUnit = int(tempQuantityPerUnit)

	return nil
}

func getStockBeforeDate(dbTx *sql.Tx, compoundId string, date int64) (int, error) {
	var stock int
	err := dbTx.QueryRow(`
        SELECT net_stock
        FROM entry
        WHERE compound_id = ? AND date < ?
        ORDER BY date DESC
        LIMIT 1
    `, compoundId, date).Scan(&stock)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logrus.Debugf("No entries found for compound '%s' before date %d. Stock assumed 0.", compoundId, date)
			return 0, nil
		}
		logrus.Errorf("Error getting stock for compound '%s' before date %d: %v", compoundId, date, err)
		return 0, err
	}
	logrus.Debugf("Stock for compound '%s' before date %d is %d.", compoundId, date, stock)
	return stock, nil
}

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

	if entryType == utils.TypeOutgoing && previousStock < txnQuantity {
		logrus.Warnf("Insufficient LATEST stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", updatedEntry.CompoundId, previousStock, txnQuantity)
		utils.JsonRes(w, http.StatusNotAcceptable, &utils.Resp{Error: utils.Insufficient_stock})
		return 0, "", errors.New("insufficient latest stock")
	}

	currentStock, err := calculateCurrentStock(entryType, previousStock, txnQuantity, updatedEntry.CompoundId)
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
	if errors.Is(err, sql.ErrNoRows) {
		logrus.Warnf("Entry with ID '%s' not found when getting type.", entry.ID)
		return "", errors.New("entry not found to get type")
	}
	if err != nil {
		logrus.Errorf("Error querying entry type for '%s': %v", entry.ID, err)
		return "", err
	}
	return entryType, nil
}

func getCurrentStock(dbTx *sql.Tx, compoundId string) (int, error) {
	var previousStock int
	err := dbTx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", compoundId).Scan(&previousStock)
	if errors.Is(err, sql.ErrNoRows) {
		logrus.Debugf("No previous stock found for compound '%s', starting from 0.", compoundId)
		return 0, nil
	}
	if err != nil {
		logrus.Errorf("Error retrieving current stock for compound '%s': %v", compoundId, err)
		return 0, err
	}
	return previousStock, nil
}

func calculateCurrentStock(entryType string, previousStock int, txnQuantity int, compoundId string) (int, error) { // nolint: gocyclo
	switch entryType {
	case utils.TypeOutgoing:
		if previousStock < txnQuantity {
			logrus.Warnf("Insufficient stock for outgoing transaction of compound '%s'. Available: %d, Requested: %d", compoundId, previousStock, txnQuantity)
			return 0, errors.New("insufficient stock during calculation")
		}
		return previousStock - txnQuantity, nil
	case utils.TypeIncoming:
		return previousStock + txnQuantity, nil
	default:
		logrus.Warnf("Unknown entry type '%s' encountered, net stock unchanged.", entryType)
		return previousStock, nil
	}
}

func updateQuantityIfChanged(dbTx *sql.Tx, updatedEntry *utils.Entry, originalQuantity utils.Quantity) (string, error) {
	var quantityID string
	updated := false

	if updatedEntry.NumOfUnits != 0 && updatedEntry.NumOfUnits != originalQuantity.NumOfUnits {
		if quantityID == "" {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				return "", err
			}
		}
		_, err := dbTx.Exec(
			"UPDATE quantity SET num_of_units = ? WHERE id = ?",
			updatedEntry.NumOfUnits, quantityID,
		)
		if err != nil {
			return "", err
		}
		logrus.Debugf("Updated num_of_units for quantity ID '%s' to %d", quantityID, updatedEntry.NumOfUnits)
		updated = true
	}

	if updatedEntry.QuantityPerUnit != 0 && updatedEntry.QuantityPerUnit != originalQuantity.QuantityPerUnit {
		if !updated && quantityID == "" {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				return "", err
			}
		} else if quantityID == "" {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				logrus.Errorf("Safeguard failed: could not get quantity ID for entry '%s'", updatedEntry.ID)
				return "", err
			}
			logrus.Warnf("Safeguard: Re-fetched quantityID '%s' for entry '%s' after update logic", quantityID, updatedEntry.ID)
		}

		_, err := dbTx.Exec(
			"UPDATE quantity SET quantity_per_unit = ? WHERE id = ?",
			updatedEntry.QuantityPerUnit, quantityID,
		)
		if err != nil {
			return "", err
		}
		logrus.Debugf("Updated quantity_per_unit for quantity ID '%s' to %d", quantityID, updatedEntry.QuantityPerUnit)
		updated = true
	}

	if updated {
		if quantityID == "" {
			err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", updatedEntry.ID).Scan(&quantityID)
			if err != nil {
				logrus.Errorf("Logical error: quantity updated but ID not found for entry '%s'", quantityID)
				return "", err
			}
			logrus.Warnf("Safeguard: Re-fetched quantityID '%s' for entry '%s' after successful quantity update check", quantityID, updatedEntry.ID)
		}
		return quantityID, nil
	}

	return "", nil
}

func getQuantityID(dbTx *sql.Tx, entryID string) (string, error) {
	var quantityID string
	err := dbTx.QueryRow("SELECT quantity_id FROM entry WHERE id = ?", entryID).Scan(&quantityID)
	if errors.Is(err, sql.ErrNoRows) {
		logrus.Warnf("Entry with ID '%s' not found when getting quantity ID.", entryID)
		return "", errors.New("entry not found to get quantity ID")
	}
	if err != nil {
		logrus.Errorf("Error querying quantity ID for '%s': %v", entryID, err)
		return "", err
	}
	return quantityID, nil
}

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
	logrus.Debugf("Updated entry with ID '%s'. New net_stock: %d", entry.ID, currentStock)
	return nil
}
