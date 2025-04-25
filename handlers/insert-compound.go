package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type InsertCompoundReq struct {
	Name  string `json:"name"`
	Scale string `json:"scale"`
}

func InsertCompoundHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &InsertCompoundReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errStr := validateCompoundReq(reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	compoundId := generateCompoundId(reqBody.Name)
	lowerCasedName := getLowerCasedCompoundName(reqBody.Name)

	compoundExists := false
	err := db.Conn.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE lower_case_name = ?)", lowerCasedName).Scan(&compoundExists)
	if err != nil {
		slog.Error("Error checking compound: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	if compoundExists {
		slog.Error("compound already exists, " + reqBody.Name)
		utils.RespWithError(w, http.StatusNotAcceptable, utils.COMPOUND_ALREADY_EXISTS)
		return
	}

	if _, err := db.Conn.Exec("INSERT INTO compound (id, lower_case_name, name, scale) VALUES (?, ?, ?, ?)", compoundId, lowerCasedName, reqBody.Name, reqBody.Scale); err != nil {
		slog.Error("Error inserting compound: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.INSERT_COMPOUND_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"compound_id": compoundId,
	})
}

func validateCompoundReq(reqBody *InsertCompoundReq) utils.ErrorMessage {
	if reqBody.Name == "" || reqBody.Scale == "" {
		slog.Error("missing required fields")
		return utils.MISSING_REQUIRED_FIELDS
	}

	if reqBody.Scale != utils.SCALE_G && reqBody.Scale != utils.SCALE_ML {
		slog.Error("invalid scale, received: " + reqBody.Scale)
		return utils.INVALID_SCALE_ERR
	}

	return utils.NO_ERR
}

func generateCompoundId(compoundName string) string {
	return fmt.Sprintf("C_%d", time.Now().Unix())
}

func getLowerCasedCompoundName(compoundName string) string {
	subStrs := strings.Split(compoundName, " ")
	for i, subStr := range subStrs {
		subStrs[i] = strings.ToLower(subStr)
	}
	return strings.Join(subStrs, "-")
}
