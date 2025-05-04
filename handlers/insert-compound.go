package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type InsertCompoundReq struct {
	Name  string `json:"name"`
	Scale string `json:"scale"`
}

func InsertCompoundHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &InsertCompoundReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		slog.Error("failed to decode JSON request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateCompoundReq(reqBody); errStr != utils.NO_ERR {
		slog.Error("invalid compound request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundId := generateCompoundId()
	lowerCasedName := utils.GetLowerCasedCompoundName(reqBody.Name)

	var compoundExists bool
	err := db.Conn.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM compound WHERE lower_case_name = ?)",
		lowerCasedName,
	).Scan(&compoundExists)

	if err != nil {
		slog.Error("error checking if compound exists", "compound_name", reqBody.Name, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	if compoundExists {
		slog.Error("compound already exists", "compound_name", reqBody.Name)
		utils.RespWithError(w, http.StatusNotAcceptable, utils.COMPOUND_ALREADY_EXISTS)
		return
	}

	_, err = db.Conn.Exec(
		"INSERT INTO compound (id, lower_case_name, name, scale) VALUES (?, ?, ?, ?)",
		compoundId, lowerCasedName, reqBody.Name, reqBody.Scale,
	)
	if err != nil {
		slog.Error("error inserting compound", "compound_id", compoundId, "compound_name", reqBody.Name, "scale", reqBody.Scale, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_COMPOUND_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"compound_id": compoundId,
	})
}

func validateCompoundReq(reqBody *InsertCompoundReq) utils.ErrorMessage {
	if reqBody.Name == "" || reqBody.Scale == "" {
		slog.Error("missing required fields", "name", reqBody.Name, "scale", reqBody.Scale)
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Scale != utils.SCALE_G && reqBody.Scale != utils.SCALE_ML {
		slog.Error("invalid scale", "scale", reqBody.Scale)
		return utils.INVALID_SCALE_ERR
	}

	return utils.NO_ERR
}

func generateCompoundId() string {
	return fmt.Sprintf("C_%d", time.Now().Unix())
}
