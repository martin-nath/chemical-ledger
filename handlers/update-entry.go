package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type UpdateEntryReq struct {
	InsertEntryReq
	Id string `json:"id"`
}

func UpdateEntryHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &UpdateEntryReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateUpdateEntryReq(reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundValid, err := utils.CheckIfCompoundExists(reqBody.CompoundId)
	if err != nil {
		slog.Error("compound check error: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	if !compoundValid {
		slog.Error("compound not found")
		utils.RespWithError(w, http.StatusNotFound, utils.INVALID_COMPOUND_ID)
		return
	}

	var oldEntry struct {
		Id         string
		Type       string
		CompoundId string
		QuantityId string
		Date       int64
	}

	if err := db.Conn.QueryRow("SELECT id, type, compound_id, quantity_id, date FROM entry WHERE id = ?", reqBody.Id).Scan(&oldEntry.Id, &oldEntry.Type, &oldEntry.CompoundId, &oldEntry.QuantityId, &oldEntry.Date); err != nil {
		slog.Error("Error retrieving entry: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}

	currTxQuantity := reqBody.NumOfUnits * reqBody.QuantityPerUnit
	entryDate := utils.GetDateUnix(reqBody.Date)

	tx, err := db.Conn.Begin()
	if err != nil {
		slog.Error("Error starting transaction: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.TX_START_ERR)
		return
	}

	_, err = tx.Exec("UPDATE quantity SET num_of_units = ?, quantity_per_unit = ? WHERE id = ?", reqBody.NumOfUnits, reqBody.QuantityPerUnit, oldEntry.QuantityId)
	if err != nil {
		slog.Error("Error updating quantity: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.UPDATE_ENTRY_ERR)
		return
	}

	_, err = tx.Exec("UPDATE entry SET type = ?, compound_id = ?, date = ?, remark = ?, voucher_no = ?, quantity_id = ?, net_stock = ? WHERE id = ?", reqBody.Type, reqBody.CompoundId, entryDate, reqBody.Remark, reqBody.VoucherNo, oldEntry.QuantityId, currTxQuantity, reqBody.Id)
	if err != nil {
		slog.Error("Error updating entry: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.UPDATE_ENTRY_ERR)
		return
	}

	wg := sync.WaitGroup{}
	errStrCh := make(chan utils.ErrorMessage, 2)
	if oldEntry.CompoundId != reqBody.CompoundId {
		wg.Add(1)

		go func() {
			defer wg.Done()
			errStrCh <- utils.UpdateNetStockFromTodayOnwards(tx, oldEntry.CompoundId, oldEntry.Date)
		}()
	}

	errStrCh <- utils.UpdateNetStockFromTodayOnwards(tx, reqBody.CompoundId, entryDate)

	wg.Wait()
	close(errStrCh)
	for errStr := range errStrCh {
		if errStr != utils.NO_ERR {
			utils.RespWithError(w, http.StatusInternalServerError, errStr)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Error committing transaction: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMMIT_TRANSACTION_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"entry_id": reqBody.Id,
	})
}

func validateUpdateEntryReq(reqBody *UpdateEntryReq) utils.ErrorMessage {
	if reqBody.Id == "" {
		slog.Error("missing required fields")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING {
		slog.Error("invalid entry type, received: " + reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	if reqBody.NumOfUnits <= 0 || reqBody.QuantityPerUnit <= 0 || reqBody.CompoundId == "" {
		slog.Error("missing required fields")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if _, err := time.Parse("2006-01-02", reqBody.Date); err != nil {
		slog.Error(err.Error())
		return utils.INVALID_DATE_FORMAT
	}

	entryExists := false
	if err := db.Conn.QueryRow("SELECT EXISTS(SELECT 1 FROM entry WHERE id = ?)", reqBody.Id).Scan(&entryExists); err != nil {
		slog.Error("Error checking entry: " + err.Error())
		return utils.ENTRY_RETRIEVAL_ERR
	}

	if !entryExists {
		slog.Error("entry not found")
		return utils.INVALID_ENTRY_ID
	}

	return utils.NO_ERR
}
