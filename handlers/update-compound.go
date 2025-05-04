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
		slog.Error("failed to decode JSON request", "error", errStr)
		utils.RespWithError(w, http.StatusBadRequest, errStr)
		return
	}

	if errMsg := validateUpdateCompoundReq(reqBody); errMsg != utils.NO_ERR {
		slog.Error("invalid compound update request", "compound_id", reqBody.ID, "error", errMsg)
		utils.RespWithError(w, http.StatusBadRequest, errMsg)
		return
	}

	lowerCasedName := utils.GetLowerCasedCompoundName(reqBody.Name)

	lowerCaseCompoundExists, err := utils.CheckIfLowerCaseCompoundExists(lowerCasedName)
	if err != nil {
		slog.Error("failed to check lowercased compound existence", "compound_name", reqBody.Name, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_ID_CHECK_ERR)
		return
	}

	scale, err := getCompoundScale(reqBody.ID)
	if err != nil {
		slog.Error("failed to get compound scale", "compound_id", reqBody.ID, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_SCALE_ERR)
		return
	}

	compoundName, err := getCompoundName(reqBody.ID)
	if err != nil {
		slog.Error("failed to get compound name", "compound_id", reqBody.ID, "error", err)
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_SCALE_ERR)
		return
	}

	if scale != reqBody.Scale && reqBody.Scale != "" && compoundName == reqBody.Name {
		if _, err := db.Conn.Exec(`
			UPDATE compound
			SET scale = ?
			WHERE id = ?`,
			reqBody.Scale, reqBody.ID,
		); err != nil {
			slog.Error("failed to update compound scale", "compound_id", reqBody.ID, "scale", reqBody.Scale, "error", err)
			utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_UPDATE_ERR)
			return
		}
	}

	if reqBody.Name != compoundName && reqBody.Name != "" {
		if lowerCaseCompoundExists {
			slog.Warn("compound name already exists", "name", reqBody.Name)
			utils.RespWithError(w, http.StatusNotAcceptable, utils.COMPOUND_ALREADY_EXISTS)
			return
		}

		if _, err := db.Conn.Exec(`
			UPDATE compound
			SET
				name = CASE WHEN ? != '' THEN ? ELSE name END,
				lower_case_name = CASE WHEN ? != '' THEN ? ELSE lower_case_name END
			WHERE id = ?`,
			reqBody.Name, reqBody.Name,
			lowerCasedName, lowerCasedName,
			reqBody.ID,
		); err != nil {
			slog.Error("failed to update compound name", "compound_id", reqBody.ID, "name", reqBody.Name, "error", err)
			utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_UPDATE_ERR)
			return
		}
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"compound_id": reqBody.ID,
	})
}

func validateUpdateCompoundReq(reqBody *UpdateCompoundReq) utils.ErrorMessage {
	if reqBody.ID == "" {
		slog.Warn("missing required field", "field", "id")
		return utils.MISSING_REQUIRED_FIELDS
	}

	compoundExists, err := utils.CheckIfCompoundExists(reqBody.ID)
	if err != nil {
		slog.Error("failed to check compound existence", "compound_id", reqBody.ID, "error", err)
		return utils.COMPOUND_ID_CHECK_ERR
	}

	if !compoundExists {
		slog.Warn("compound does not exist", "compound_id", reqBody.ID)
		return utils.INVALID_COMPOUND_ID
	}

	return utils.NO_ERR
}

func getCompoundScale(compoundId string) (string, error) {
	var scale string
	err := utils.IfErrRetry(func() error {
		return db.Conn.QueryRow("SELECT scale FROM compound WHERE id = ?", compoundId).Scan(&scale)
	})
	if err != nil {
		return "", err
	}
	return scale, nil
}

func getCompoundName(compoundId string) (string, error) {
	var name string
	err := utils.IfErrRetry(func() error {
		return db.Conn.QueryRow("SELECT name FROM compound WHERE id = ?", compoundId).Scan(&name)
	})
	if err != nil {
		return "", err
	}
	return name, nil
}
