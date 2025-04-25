package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type InsertEntryReq struct {
	Type            string `json:"type"`
	CompoundId      string `json:"compound_id"`
	Date            string `json:"date"`
	Remark          string `json:"remark"`
	VoucherNo       string `json:"voucher_no"`
	NumOfUnits      int    `json:"num_of_units"`
	QuantityPerUnit int    `json:"quantity_per_unit"`
}

func InsertEntryHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &InsertEntryReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateInsertEntryReq(reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateDate(reqBody.Date); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundExists, err := utils.CheckIfCompoundExists(reqBody.CompoundId)
	if err != nil {
		slog.Error("compound check error: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	if !compoundExists {
		slog.Error("compound not found")
		utils.RespWithError(w, http.StatusNotFound, utils.INVALID_COMPOUND_ID)
		return
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		slog.Error("Error starting transaction: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.TX_START_ERR)
		return
	}
	defer tx.Rollback()

	quantityId := generateQuantityId()

	if _, err := tx.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)", quantityId, reqBody.NumOfUnits, reqBody.QuantityPerUnit); err != nil {
		slog.Error("Error inserting quantity: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_QUANTITY_ERR)
		return
	}

	entryDate := utils.GetDateUnix(reqBody.Date)
	currentTxQuantity := reqBody.NumOfUnits * reqBody.QuantityPerUnit
	entryId := generateEntryId()

	if _, err := tx.Exec("INSERT INTO entry (id, type, compound_id, date, remark, voucher_no, quantity_id, net_stock) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", entryId, reqBody.Type, reqBody.CompoundId, entryDate, reqBody.Remark, reqBody.VoucherNo, quantityId, currentTxQuantity); err != nil {
		slog.Error("Error inserting entry: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_ENTRY_ERR)
		return
	}

	if errStr := utils.UpdateNetStockFromTodayOnwards(tx, reqBody.CompoundId, entryDate); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusInternalServerError, errStr)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Error committing transaction: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMMIT_TRANSACTION_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"entry_id": entryId,
	})
}

func validateInsertEntryReq(reqBody *InsertEntryReq) utils.ErrorMessage {
	if reqBody.Type == "" || reqBody.CompoundId == "" || reqBody.Date == "" || reqBody.NumOfUnits == 0 || reqBody.QuantityPerUnit == 0 {
		slog.Error("missing required fields")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING {
		slog.Error("invalid entry type, received: " + reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	return utils.NO_ERR
}

func validateDate(date string) utils.ErrorMessage {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		slog.Error(err.Error())
		return utils.INVALID_DATE_FORMAT
	}

	if utils.GetDateUnix(date) > time.Now().Unix() {
		slog.Error("future date provided")
		return utils.FUTURE_DATE_ERR
	}

	return utils.NO_ERR
}

func generateQuantityId() string {
	return fmt.Sprintf("Q_%d", time.Now().Unix())
}

func generateEntryId() string {
	return fmt.Sprintf("E_%d", time.Now().Unix())
}
