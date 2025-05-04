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
		slog.Error("failed to decode JSON request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateUpdateEntryReq(reqBody); errStr != utils.NO_ERR {
		slog.Error("invalid update entry request", "entry_id", reqBody.Id, "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundValid, err := utils.CheckIfCompoundExists(reqBody.CompoundId)
	if err != nil {
		slog.Error("error checking compound existence", "compound_id", reqBody.CompoundId, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}
	if !compoundValid {
		slog.Warn("compound not found", "compound_id", reqBody.CompoundId)
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
	if err := db.Conn.QueryRow(
		"SELECT id, type, compound_id, quantity_id, date FROM entry WHERE id = ?",
		reqBody.Id,
	).Scan(&oldEntry.Id, &oldEntry.Type, &oldEntry.CompoundId, &oldEntry.QuantityId, &oldEntry.Date); err != nil {
		slog.Error("error retrieving entry", "entry_id", reqBody.Id, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}

	currTxQuantity := reqBody.NumOfUnits * reqBody.QuantityPerUnit
	entryDate, err := utils.MergeDateWithUnixTime(reqBody.Date, oldEntry.Date)
	if err != nil {
		slog.Error("failed to merge date with unix time", "input_date", reqBody.Date, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.INVALID_DATE_FORMAT)
		return
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.TX_START_ERR)
		return
	}
	defer tx.Rollback()

	if _, err = tx.Exec(
		"UPDATE quantity SET num_of_units = ?, quantity_per_unit = ? WHERE id = ?",
		reqBody.NumOfUnits, reqBody.QuantityPerUnit, oldEntry.QuantityId); err != nil {
		slog.Error("failed to update quantity", "quantity_id", oldEntry.QuantityId, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.UPDATE_ENTRY_ERR)
		return
	}

	if _, err = tx.Exec(
		`UPDATE entry 
		SET type = ?, compound_id = ?, date = ?, remark = ?, voucher_no = ?, quantity_id = ?, net_stock = ? 
		WHERE id = ?`,
		reqBody.Type, reqBody.CompoundId, entryDate,
		reqBody.Remark, reqBody.VoucherNo,
		oldEntry.QuantityId, currTxQuantity,
		reqBody.Id); err != nil {
		slog.Error("failed to update entry", "entry_id", reqBody.Id, "error", err)
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
			slog.Error("failed to update net stock during entry update", "entry_id", reqBody.Id, "error", errStr)
			utils.RespWithError(w, http.StatusInternalServerError, errStr)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "entry_id", reqBody.Id, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMMIT_TRANSACTION_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"entry_id": reqBody.Id,
	})
}

func validateUpdateEntryReq(reqBody *UpdateEntryReq) utils.ErrorMessage {
	if reqBody.Id == "" {
		slog.Warn("missing required field", "field", "id")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING {
		slog.Warn("invalid entry type", "received", reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	if reqBody.NumOfUnits <= 0 || reqBody.QuantityPerUnit <= 0 || reqBody.CompoundId == "" {
		slog.Warn("missing required numeric fields or compound ID", "num_of_units", reqBody.NumOfUnits, "quantity_per_unit", reqBody.QuantityPerUnit, "compound_id", reqBody.CompoundId)
		return utils.MISSING_REQUIRED_FIELDS
	}

	if _, err := time.Parse("2006-01-02", reqBody.Date); err != nil {
		slog.Warn("invalid date format", "input_date", reqBody.Date, "error", err)
		return utils.INVALID_DATE_FORMAT
	}

	var entryExists bool
	if err := db.Conn.QueryRow("SELECT EXISTS(SELECT 1 FROM entry WHERE id = ?)", reqBody.Id).Scan(&entryExists); err != nil {
		slog.Error("error checking entry existence", "entry_id", reqBody.Id, "error", err)
		return utils.ENTRY_RETRIEVAL_ERR
	}

	if !entryExists {
		slog.Warn("entry not found", "entry_id", reqBody.Id)
		return utils.INVALID_ENTRY_ID
	}

	return utils.NO_ERR
}
