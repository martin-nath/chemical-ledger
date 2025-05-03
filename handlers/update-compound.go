package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"log/slog"
	"net/http"
)

type UpdateCompoundReq struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scale string `json:"scale"`
}

func UpdateCompoundHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &UpdateCompoundReq{}
	if errStr := utils.DecodeJsonReq(r, reqBody); errStr != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errMsg := validateUpdateCompoundReq(reqBody); errMsg != utils.NO_ERR {
		utils.RespWithError(w, http.StatusBadRequest, errMsg)
		return
	}

	lowerCasedName := utils.GetLowerCasedCompoundName(reqBody.Name)

	lowerCaseCompoundExists, err := utils.CheckIfLowerCaseCompoundExists(lowerCasedName)
	if err != nil {
		slog.Error("Error checking if compound exists: ", "error", err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	if lowerCaseCompoundExists {
		slog.Warn("Compound with name: ", reqBody.Name, " already exists")
		utils.RespWithError(w, http.StatusNotAcceptable, utils.COMPOUND_ALREADY_EXISTS)
		return
	}

	if _, err := db.Conn.Exec(`
	UPDATE compound
	SET
		name = CASE
			WHEN ? != '' THEN ?
			ELSE name
		END,
		scale = CASE
			WHEN ? != '' THEN ?
			ELSE scale
		END,
		lower_case_name = CASE
			WHEN ? != '' THEN ?
			ELSE lower_case_name
		END
	WHERE id = ?`,
		reqBody.Name, reqBody.Name,
		reqBody.Scale, reqBody.Scale,
		lowerCasedName, lowerCasedName,
		reqBody.ID,
	); err != nil {
		slog.Error("Error updating compound", "error", err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_UPDATE_ERR)
		return
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"compound_id": reqBody.ID,
	})
}

func validateUpdateCompoundReq(reqBody *UpdateCompoundReq) utils.ErrorMessage {
	if reqBody.ID == "" {
		slog.Warn("Missing required field: id")
		return utils.MISSING_REQUIRED_FIELDS
	}

	compoundExists, err := utils.CheckIfCompoundExists(reqBody.ID)
	if err != nil {
		slog.Error("Error checking if compound exists: ", "error", err.Error())
		return utils.COMPOUND_ID_CHECK_ERR
	}

	if !compoundExists {
		slog.Warn("Compound with id: ", reqBody.ID, " does not exist")
		return utils.INVALID_COMPOUND_ID
	}

	return utils.NO_ERR
}
