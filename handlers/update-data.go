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

	entry := &utils.Entry{}
	if err := utils.JsonReq(r, entry, w); err != nil {
		return
	}
	logrus.Infof("Received request to update data: %+v", entry)

	if err := validateUpdateEntryFields2(entry, w); err != nil {
		return
	}

	var updatedEntryDate int64
	var err error

	if entry.Date != "" {
		if updatedEntryDate, err = utils.ParseAndValidateDate(entry.Date, w); err != nil {
			return
		}
	}

	dbTx, err := utils.BeginDbTx(w)
	if err != nil {
		return
	}

	defer func() {
		if err := dbTx.Rollback(); err != nil && err != sql.ErrTxDone {
			logrus.Errorf("Error during deferred transaction rollback: %v", err)
		} else if err == nil {
			logrus.Debug("Deferred transaction rollback executed.")
		}
	}()

	var originalEntryDate int64
	var originalCompoundId string
	var originalQuantity utils.Quantity
	var originalEntryType string

	if err := retrieveOriginalEntryData2(dbTx, entry.ID, &originalEntryDate, &originalCompoundId, &originalQuantity, &originalEntryType, w); err != nil {
		return
	}

	if entry.QuantityPerUnit != originalQuantity.QuantityPerUnit {
		entry.QuantityPerUnit = originalQuantity.QuantityPerUnit
	}
	if entry.NumOfUnits != originalQuantity.NumOfUnits {
		entry.NumOfUnits = originalQuantity.NumOfUnits
	}

	if updatedEntryDate == 0 {
		updatedEntryDate = originalEntryDate
	}

	if entry.CompoundId != "" {
		if err := utils.CheckIfCompoundExists(entry.CompoundId, w); err != nil {
			return
		}
	}

	var quantityId string
	quantityId, err = getQuantityID2(dbTx, entry.ID)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: utils.Stock_retrieval_error})
		logrus.Errorf("Error retrieving quantity ID for entry '%s': %v", entry.ID, err)
		return
	}

	if err = updateEntryDetails2(dbTx, entry, updatedEntryDate, quantityId, w); err != nil {
		return
	}

	wg := sync.WaitGroup{}
	errCh := make(chan string, 2)

	if originalCompoundId != entry.CompoundId {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- utils.UpdateSubSequentNetStock(dbTx, updatedEntryDate, entry.CompoundId)
		}()
	}
	errCh <- utils.UpdateSubSequentNetStock(dbTx, originalEntryDate, originalCompoundId)

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != "" {
			logrus.Errorf("Error updating subsequent stock: %v", err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: err})
			return
		}
	}

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

func validateUpdateEntryFields2(entry *utils.Entry, w http.ResponseWriter) error {
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
		logrus.Warn("Quantity per unit and number of units cannot be negative or zero.")
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: utils.MissingFields_or_inappropriate_value})
		return errors.New("invalid quantity values")
	}

	return nil
}

func retrieveOriginalEntryData2(dbTx *sql.Tx, entryID string, originalEntryDate *int64, originalCompoundId *string, originalQuantity *utils.Quantity, originalEntryType *string, w http.ResponseWriter) error {
	err := dbTx.QueryRow(`
		SELECT e.type, e.date, e.compound_id, q.num_of_units, q.quantity_per_unit
		FROM entry e
		JOIN quantity q ON e.quantity_id = q.id
		WHERE e.id = ?
	`, entryID).Scan(originalEntryType, originalEntryDate, originalCompoundId, &originalQuantity.NumOfUnits, &originalQuantity.QuantityPerUnit)
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

	return nil
}

// updateEntryDetails updates the main entry details in the database.
func updateEntryDetails2(dbTx *sql.Tx, entry *utils.Entry, entryDate int64, quantityID string, w http.ResponseWriter) error {
	query := `
		UPDATE entry
		SET
			type = CASE WHEN ? != '' THEN ? ELSE type END,
			date = CASE WHEN ? != 0 THEN ? ELSE date END,
			compound_id = CASE WHEN ? != '' THEN ? ELSE compound_id END,
			remark = CASE WHEN ? != '' THEN ? ELSE remark END,
			voucher_no = CASE WHEN ? != '' THEN ? ELSE voucher_no END,
			quantity_id = CASE WHEN ? != '' THEN ? ELSE quantity_id END,
			net_stock = 0
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

func getQuantityID2(dbTx *sql.Tx, entryID string) (string, error) {
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
