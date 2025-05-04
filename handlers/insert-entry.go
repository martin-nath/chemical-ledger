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
	/* This part of the code is to prevent the trial period from exceeding the limit */
	const TRIAL_PERIOD_ENTRY_LIMIT = 20
	
	var totalEntries int
	if err := db.Conn.QueryRow("SELECT COUNT(*) FROM entry").Scan(&totalEntries); err != nil {
		slog.Error("error getting total entries", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.ENTRY_RETRIEVAL_ERR)
		return
	}
	if totalEntries >= TRIAL_PERIOD_ENTRY_LIMIT {
		slog.Error("trial period limit exceeded", "total_entries", totalEntries)
		utils.RespWithError(w, http.StatusBadRequest, utils.TRIAL_PERIOD_LIMIT_EXCEEDED)
		return
	}
	/* Trial Period code ends here */

	reqBody := &InsertEntryReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		slog.Error("failed to decode JSON request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateInsertEntryReq(reqBody); errStr != utils.NO_ERR {
		slog.Error("invalid insert entry request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateDate(reqBody.Date); errStr != utils.NO_ERR {
		slog.Error("invalid date format", "date", reqBody.Date, "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundExists, err := utils.CheckIfCompoundExists(reqBody.CompoundId)
	if err != nil {
		slog.Error("error checking if compound exists", "compound_id", reqBody.CompoundId, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}
	if !compoundExists {
		slog.Error("compound not found", "compound_id", reqBody.CompoundId)
		utils.RespWithError(w, http.StatusNotFound, utils.INVALID_COMPOUND_ID)
		return
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		slog.Error("error starting transaction", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.TX_START_ERR)
		return
	}
	defer tx.Rollback()

	quantityId := generateQuantityId()
	if _, err := tx.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)", quantityId, reqBody.NumOfUnits, reqBody.QuantityPerUnit); err != nil {
		slog.Error("error inserting quantity", "quantity_id", quantityId, "num_of_units", reqBody.NumOfUnits, "quantity_per_unit", reqBody.QuantityPerUnit, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_QUANTITY_ERR)
		return
	}

	entryDate := utils.GetDateUnix(reqBody.Date)
	currentTxQuantity := reqBody.NumOfUnits * reqBody.QuantityPerUnit
	entryId := generateEntryId()

	if _, err := tx.Exec(
		"INSERT INTO entry (id, type, compound_id, date, remark, voucher_no, quantity_id, net_stock) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		entryId, reqBody.Type, reqBody.CompoundId, entryDate, reqBody.Remark, reqBody.VoucherNo, quantityId, currentTxQuantity,
	); err != nil {
		slog.Error("error inserting entry",
			"entry_id", entryId,
			"compound_id", reqBody.CompoundId,
			"quantity_id", quantityId,
			"date", reqBody.Date,
			"error", err,
		)
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_ENTRY_ERR)
		return
	}

	if errStr := utils.UpdateNetStockFromTodayOnwards(tx, reqBody.CompoundId, entryDate); errStr != utils.NO_ERR {
		slog.Error("error updating net stock", "compound_id", reqBody.CompoundId, "date", reqBody.Date, "error", errStr)
		utils.RespWithError(w, http.StatusInternalServerError, errStr)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("error committing transaction", "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMMIT_TRANSACTION_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"entry_id": entryId,
	})
}

func validateInsertEntryReq(reqBody *InsertEntryReq) utils.ErrorMessage {
	if reqBody.Type == "" || reqBody.CompoundId == "" || reqBody.Date == "" || reqBody.NumOfUnits == 0 || reqBody.QuantityPerUnit == 0 {
		slog.Error("missing required fields in entry request", "request", reqBody)
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Type != utils.ENTRY_TYPE_INCOMING && reqBody.Type != utils.ENTRY_TYPE_OUTGOING {
		slog.Error("invalid entry type", "received_type", reqBody.Type)
		return utils.INVALID_ENTRY_TYPE
	}

	return utils.NO_ERR
}

func validateDate(date string) utils.ErrorMessage {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		slog.Error("date parsing failed", "date", date, "error", err)
		return utils.INVALID_DATE_FORMAT
	}

	if parsed.Unix() > time.Now().Unix() {
		slog.Error("future date provided", "date", date)
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
